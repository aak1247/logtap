package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/migration"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/testkit"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

func TestMigrationExportCloud_AllowsNoAuthLocalServer(t *testing.T) {
	t.Parallel()

	var receivedAuth string
	var receivedBundle migration.Bundle
	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/imports/local-logtap" {
			t.Fatalf("unexpected cloud path: %s", r.URL.Path)
		}
		receivedAuth = r.Header.Get("Authorization")
		var req struct {
			Bundle migration.Bundle `json:"bundle"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode cloud request: %v", err)
		}
		receivedBundle = req.Bundle
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"imp_test","status":"pending","status_url":"/api/imports/imp_test"}}`))
	}))
	t.Cleanup(cloud.Close)

	gin.SetMode(gin.TestMode)
	db := testkit.OpenTestDB(t)
	publisher := &testkit.InlinePublisher{DB: db}
	project := model.Project{Name: "local-prod"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := db.Create(&model.Log{
		ProjectID:  project.ID,
		Timestamp:  time.Now().UTC(),
		Level:      "info",
		Message:    "hello",
		Fields:     datatypes.JSON([]byte(`{"k":"v"}`)),
		DistinctID: "u1",
	}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	cfg := config.Config{
		HTTPAddr:          "127.0.0.1:0",
		MigrationCloudURL: cloud.URL,
	}
	srv := httpserver.New(cfg, publisher, db, nil, nil, nil, nil)
	local := httptest.NewServer(srv.Handler)
	t.Cleanup(local.Close)

	status, body := testkit.DoJSON(t, local.Client(), http.MethodPost, local.URL+"/api/migration/export/preview", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("preview code=%d err=%s", env.Code, env.Err)
	}
	var preview migration.Preview
	if err := json.Unmarshal(env.Data, &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.CloudDefaultURL != cloud.URL || len(preview.Projects) != 1 || preview.TotalLogs != 1 {
		t.Fatalf("bad preview: %+v", preview)
	}

	status, body = testkit.DoJSON(t, local.Client(), http.MethodPost, local.URL+"/api/migration/export/cloud", map[string]any{
		"cloud_token": "cloud-token",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("export status=%d body=%s", status, string(body))
	}
	env = testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("export code=%d err=%s", env.Code, env.Err)
	}
	if receivedAuth != "Bearer cloud-token" {
		t.Fatalf("unexpected cloud auth header: %q", receivedAuth)
	}
	if len(receivedBundle.Projects) != 1 || receivedBundle.Projects[0].Name != "local-prod" {
		t.Fatalf("bad bundle projects: %+v", receivedBundle.Projects)
	}
	if len(receivedBundle.Projects[0].Logs) != 1 {
		t.Fatalf("expected one exported log, got %d", len(receivedBundle.Projects[0].Logs))
	}
}

func TestMigrationExportPreview_RequiresAuthWhenConfigured(t *testing.T) {
	t.Parallel()

	ts := testkit.NewServer(t)
	status, body := testkit.DoJSON(t, ts.HTTP.Client(), http.MethodPost, ts.HTTP.URL+"/api/migration/export/preview", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without local auth token, got %d body=%s", status, string(body))
	}

	boot := testkit.Bootstrap(t, ts.HTTP.Client(), ts.HTTP.URL)
	status, body = testkit.DoJSON(t, ts.HTTP.Client(), http.MethodPost, ts.HTTP.URL+"/api/migration/export/preview", nil, map[string]string{
		"Authorization": "Bearer " + boot.Token,
	})
	if status != http.StatusOK {
		t.Fatalf("expected 200 with local auth token, got %d body=%s", status, string(body))
	}
}
