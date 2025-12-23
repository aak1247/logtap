package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/ingest"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testAuthSecret = "01234567890123456789012345678901"

type testPublisher struct {
	db *gorm.DB
}

func (p *testPublisher) Publish(_ string, body []byte) error {
	var msg ingest.NSQMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return err
	}

	switch msg.Type {
	case "log":
		var lp ingest.CustomLogPayload
		if err := json.Unmarshal(msg.Payload, &lp); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return store.InsertLog(ctx, p.db, msg.ProjectID, lp)
	case "event", "envelope":
		var ev map[string]any
		if err := json.Unmarshal(msg.Payload, &ev); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return store.InsertEvent(ctx, p.db, msg.ProjectID, ev)
	default:
		return nil
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open(sqlite): %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := gdb.AutoMigrate(&model.User{}, &model.Project{}, &model.ProjectKey{}, &model.Event{}, &model.Log{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	cfg := config.Config{
		HTTPAddr:     "127.0.0.1:0",
		AuthSecret:   []byte(testAuthSecret),
		AuthTokenTTL: time.Hour,
	}
	publisher := &testPublisher{db: db}

	srv := httpserver.New(cfg, publisher, db, nil)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

type apiEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Err  string          `json:"err"`
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) (int, []byte) {
	t.Helper()

	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		rd = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return res.StatusCode, b
}

func decodeEnvelope(t *testing.T, body []byte) apiEnvelope {
	t.Helper()

	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, string(body))
	}
	return env
}

func bootstrap(t *testing.T, client *http.Client, baseURL string) (string, int, string) {
	t.Helper()

	req := map[string]any{
		"email":        "owner@example.com",
		"password":     "pass123456",
		"project_name": "Default",
		"key_name":     "default",
	}
	status, body := doJSON(t, client, http.MethodPost, baseURL+"/api/auth/bootstrap", req, nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("bootstrap code=%d err=%s", env.Code, env.Err)
	}
	var data struct {
		Token   string `json:"token"`
		Project struct {
			ID int `json:"id"`
		} `json:"project"`
		Key struct {
			Key string `json:"key"`
		} `json:"key"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("bootstrap data: %v", err)
	}
	return data.Token, data.Project.ID, data.Key.Key
}

func createProject(t *testing.T, client *http.Client, baseURL, token, name string) int {
	t.Helper()

	headers := map[string]string{"Authorization": "Bearer " + token}
	status, body := doJSON(t, client, http.MethodPost, baseURL+"/api/projects", map[string]any{"name": name}, headers)
	if status != http.StatusOK {
		t.Fatalf("create project status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create project code=%d err=%s", env.Code, env.Err)
	}
	var data struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("create project data: %v", err)
	}
	return data.ID
}

func createProjectKey(t *testing.T, client *http.Client, baseURL, token string, projectID int, name string) string {
	t.Helper()

	headers := map[string]string{"Authorization": "Bearer " + token}
	url := fmt.Sprintf("%s/api/projects/%d/keys", baseURL, projectID)
	status, body := doJSON(t, client, http.MethodPost, url, map[string]any{"name": name}, headers)
	if status != http.StatusOK {
		t.Fatalf("create key status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("create key code=%d err=%s", env.Code, env.Err)
	}
	var data struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("create key data: %v", err)
	}
	return data.Key
}

func TestIntegration_ProjectLogsCleanup(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t)
	client := ts.Client()
	baseURL := ts.URL

	token, _, _ := bootstrap(t, client, baseURL)
	projectID := createProject(t, client, baseURL, token, "app")
	projectKey := createProjectKey(t, client, baseURL, token, projectID, "default")

	logTs := time.Now().UTC().Add(-time.Minute)
	logPayload := map[string]any{
		"level":     "info",
		"message":   "hello log",
		"trace_id":  "trace-1",
		"timestamp": logTs.Format(time.RFC3339Nano),
		"user":      map[string]any{"id": "u1"},
	}
	headers := map[string]string{"X-Project-Key": projectKey}
	status, body := doJSON(t, client, http.MethodPost, fmt.Sprintf("%s/api/%d/logs/", baseURL, projectID), logPayload, headers)
	if status != http.StatusAccepted {
		t.Fatalf("ingest log status=%d body=%s", status, string(body))
	}

	searchURL := fmt.Sprintf("%s/api/%d/logs/search?level=info&limit=10", baseURL, projectID)
	headers = map[string]string{"Authorization": "Bearer " + token}
	status, body = doJSON(t, client, http.MethodGet, searchURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("search logs status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("search logs code=%d err=%s", env.Code, env.Err)
	}
	var logs []struct {
		Message string `json:"message"`
		TraceID string `json:"trace_id"`
	}
	if err := json.Unmarshal(env.Data, &logs); err != nil {
		t.Fatalf("search logs data: %v", err)
	}
	if len(logs) == 0 {
		t.Fatalf("expected logs, got empty")
	}
	found := false
	for _, entry := range logs {
		if entry.Message == "hello log" && entry.TraceID == "trace-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected log not found: %#v", logs)
	}

	before := time.Now().UTC().Add(time.Minute)
	cleanupURL := fmt.Sprintf("%s/api/%d/logs/cleanup?before=%s", baseURL, projectID, url.QueryEscape(before.Format(time.RFC3339Nano)))
	status, body = doJSON(t, client, http.MethodDelete, cleanupURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("cleanup logs status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("cleanup logs code=%d err=%s", env.Code, env.Err)
	}
	var cleanup struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal(env.Data, &cleanup); err != nil {
		t.Fatalf("cleanup logs data: %v", err)
	}
	if cleanup.Deleted < 1 {
		t.Fatalf("expected deleted>=1, got %d", cleanup.Deleted)
	}

	status, body = doJSON(t, client, http.MethodGet, searchURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("search logs after cleanup status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("search logs after cleanup code=%d err=%s", env.Code, env.Err)
	}
	var logsAfter []map[string]any
	if err := json.Unmarshal(env.Data, &logsAfter); err != nil {
		t.Fatalf("search logs after cleanup data: %v", err)
	}
	if len(logsAfter) != 0 {
		t.Fatalf("expected logs cleared, got %d", len(logsAfter))
	}
}

func TestIntegration_EventIngestCleanup(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t)
	client := ts.Client()
	baseURL := ts.URL

	token, _, _ := bootstrap(t, client, baseURL)
	projectID := createProject(t, client, baseURL, token, "events")
	projectKey := createProjectKey(t, client, baseURL, token, projectID, "default")

	eventID := "11111111-1111-1111-1111-111111111111"
	eventTs := time.Now().UTC().Add(-time.Minute)
	eventPayload := map[string]any{
		"event_id":  eventID,
		"level":     "error",
		"message":   "boom",
		"timestamp": eventTs.Format(time.RFC3339Nano),
		"user":      map[string]any{"id": "u1"},
	}
	headers := map[string]string{"X-Project-Key": projectKey}
	storeURL := fmt.Sprintf("%s/api/%d/store/", baseURL, projectID)
	status, body := doJSON(t, client, http.MethodPost, storeURL, eventPayload, headers)
	if status != http.StatusOK {
		t.Fatalf("store event status=%d body=%s", status, string(body))
	}
	var storeResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &storeResp); err != nil {
		t.Fatalf("store event response: %v", err)
	}
	if storeResp.ID == "" {
		t.Fatalf("expected event id in store response")
	}

	headers = map[string]string{"Authorization": "Bearer " + token}
	recentURL := fmt.Sprintf("%s/api/%d/events/recent?limit=10", baseURL, projectID)
	status, body = doJSON(t, client, http.MethodGet, recentURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("recent events status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("recent events code=%d err=%s", env.Code, env.Err)
	}
	var recent []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &recent); err != nil {
		t.Fatalf("recent events data: %v", err)
	}
	if len(recent) == 0 {
		t.Fatalf("unexpected recent events: %#v", recent)
	}
	found := false
	for _, item := range recent {
		if item.ID == storeResp.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected event id not found: %#v", recent)
	}

	getURL := fmt.Sprintf("%s/api/%d/events/%s", baseURL, projectID, storeResp.ID)
	status, body = doJSON(t, client, http.MethodGet, getURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("get event status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("get event code=%d err=%s", env.Code, env.Err)
	}
	var eventData map[string]any
	if err := json.Unmarshal(env.Data, &eventData); err != nil {
		t.Fatalf("get event data: %v", err)
	}
	if eventData["message"] != "boom" {
		t.Fatalf("unexpected event data: %#v", eventData)
	}

	before := time.Now().UTC().Add(time.Minute)
	cleanupURL := fmt.Sprintf("%s/api/%d/events/cleanup?before=%s", baseURL, projectID, url.QueryEscape(before.Format(time.RFC3339Nano)))
	status, body = doJSON(t, client, http.MethodDelete, cleanupURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("cleanup events status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("cleanup events code=%d err=%s", env.Code, env.Err)
	}
	var cleanup struct {
		Deleted int64 `json:"deleted"`
	}
	if err := json.Unmarshal(env.Data, &cleanup); err != nil {
		t.Fatalf("cleanup events data: %v", err)
	}
	if cleanup.Deleted < 1 {
		t.Fatalf("expected deleted>=1, got %d", cleanup.Deleted)
	}

	status, body = doJSON(t, client, http.MethodGet, recentURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("recent events after cleanup status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("recent events after cleanup code=%d err=%s", env.Code, env.Err)
	}
	var recentAfter []map[string]any
	if err := json.Unmarshal(env.Data, &recentAfter); err != nil {
		t.Fatalf("recent events after cleanup data: %v", err)
	}
	if len(recentAfter) != 0 {
		t.Fatalf("expected events cleared, got %d", len(recentAfter))
	}
}

func TestIntegration_TrackAnalytics(t *testing.T) {
	t.Parallel()

	ts := newTestServer(t)
	client := ts.Client()
	baseURL := ts.URL

	token, _, _ := bootstrap(t, client, baseURL)
	projectID := createProject(t, client, baseURL, token, "analytics")
	projectKey := createProjectKey(t, client, baseURL, token, projectID, "default")

	now := time.Now().UTC()
	events := []struct {
		User      string
		Name      string
		Timestamp time.Time
	}{
		{User: "u1", Name: "signup", Timestamp: now.Add(1 * time.Second)},
		{User: "u1", Name: "checkout", Timestamp: now.Add(2 * time.Second)},
		{User: "u1", Name: "paid", Timestamp: now.Add(3 * time.Second)},
		{User: "u2", Name: "signup", Timestamp: now.Add(1 * time.Second)},
		{User: "u2", Name: "checkout", Timestamp: now.Add(2 * time.Second)},
	}
	headers := map[string]string{"X-Project-Key": projectKey}
	for _, ev := range events {
		payload := map[string]any{
			"name":      ev.Name,
			"timestamp": ev.Timestamp.Format(time.RFC3339Nano),
			"user":      map[string]any{"id": ev.User},
			"properties": map[string]any{
				"step": ev.Name,
			},
		}
		status, body := doJSON(t, client, http.MethodPost, fmt.Sprintf("%s/api/%d/track/", baseURL, projectID), payload, headers)
		if status != http.StatusAccepted {
			t.Fatalf("track status=%d body=%s", status, string(body))
		}
	}

	headers = map[string]string{"Authorization": "Bearer " + token}
	start := url.QueryEscape(now.Add(-time.Hour).Format(time.RFC3339Nano))
	end := url.QueryEscape(now.Add(time.Hour).Format(time.RFC3339Nano))
	topURL := fmt.Sprintf("%s/api/%d/analytics/events/top?start=%s&end=%s&limit=10", baseURL, projectID, start, end)
	status, body := doJSON(t, client, http.MethodGet, topURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("top events status=%d body=%s", status, string(body))
	}
	env := decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("top events code=%d err=%s", env.Code, env.Err)
	}
	var top struct {
		Items []struct {
			Name   string `json:"name"`
			Events int64  `json:"events"`
			Users  int64  `json:"users"`
		} `json:"items"`
	}
	if err := json.Unmarshal(env.Data, &top); err != nil {
		t.Fatalf("top events data: %v", err)
	}
	counts := map[string]struct {
		Events int64
		Users  int64
	}{}
	for _, it := range top.Items {
		counts[it.Name] = struct {
			Events int64
			Users  int64
		}{Events: it.Events, Users: it.Users}
	}
	if got := counts["signup"]; got.Events != 2 || got.Users != 2 {
		t.Fatalf("unexpected signup counts: %#v", got)
	}
	if got := counts["checkout"]; got.Events != 2 || got.Users != 2 {
		t.Fatalf("unexpected checkout counts: %#v", got)
	}
	if got := counts["paid"]; got.Events != 1 || got.Users != 1 {
		t.Fatalf("unexpected paid counts: %#v", got)
	}

	funnelURL := fmt.Sprintf("%s/api/%d/analytics/funnel?steps=signup,checkout,paid&start=%s&end=%s&within=24h", baseURL, projectID, start, end)
	status, body = doJSON(t, client, http.MethodGet, funnelURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("funnel status=%d body=%s", status, string(body))
	}
	env = decodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("funnel code=%d err=%s", env.Code, env.Err)
	}
	var funnel struct {
		Steps []struct {
			Name  string `json:"name"`
			Users int64  `json:"users"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(env.Data, &funnel); err != nil {
		t.Fatalf("funnel data: %v", err)
	}
	want := map[string]int64{"signup": 2, "checkout": 2, "paid": 1}
	seen := map[string]bool{}
	for _, step := range funnel.Steps {
		exp, ok := want[step.Name]
		if !ok {
			continue
		}
		seen[step.Name] = true
		if exp != step.Users {
			t.Fatalf("funnel step %s users=%d want=%d", step.Name, step.Users, exp)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("missing funnel step %s", name)
		}
	}
}
