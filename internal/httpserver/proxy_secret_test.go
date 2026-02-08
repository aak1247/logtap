package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/httpserver"
	"github.com/aak1247/logtap/internal/testkit"
	"github.com/gin-gonic/gin"
)

func TestProxySecret_ProtectsProjectRoutesAndBypassesAuth(t *testing.T) {
	t.Parallel()

	ts := newProxySecretServer(t, "proxy_secret")
	baseURL := ts.URL
	client := ts.Client()

	br := testkit.Bootstrap(t, client, baseURL)
	if br.Token == "" || br.ProjectID <= 0 {
		t.Fatalf("expected bootstrap ok: token=%q projectID=%d", br.Token, br.ProjectID)
	}

	// Ingest without proxy secret should be rejected.
	status, _ := testkit.DoJSON(t, client, http.MethodPost, baseURL+"/api/1/logs/", map[string]any{
		"level": "info", "message": "x",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 without proxy secret, got %d", status)
	}

	// Ingest with proxy secret should succeed without X-Project-Key.
	headers := map[string]string{"X-Logtap-Proxy-Secret": "proxy_secret"}
	status, body := testkit.DoJSON(t, client, http.MethodPost, baseURL+"/api/1/logs/", map[string]any{
		"level": "info", "message": "hello",
	}, headers)
	if status != http.StatusAccepted {
		t.Fatalf("expected 202 with proxy secret, got %d body=%s", status, string(body))
	}

	// Query with proxy secret should succeed without Authorization.
	status, body = testkit.DoJSON(t, client, http.MethodGet, baseURL+"/api/1/logs/search?limit=10", nil, headers)
	if status != http.StatusOK {
		t.Fatalf("expected 200 query with proxy secret, got %d body=%s", status, string(body))
	}
}

func TestInternalCreateProject_RequiresProxySecret(t *testing.T) {
	t.Parallel()

	ts := newProxySecretServer(t, "proxy_secret")
	baseURL := ts.URL
	client := ts.Client()

	// Missing secret => 401
	status, body := testkit.DoJSON(t, client, http.MethodPost, baseURL+"/api/internal/projects", map[string]any{"name": "app"}, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", status, string(body))
	}

	headers := map[string]string{"X-Logtap-Proxy-Secret": "proxy_secret"}
	status, body = testkit.DoJSON(t, client, http.MethodPost, baseURL+"/api/internal/projects", map[string]any{"name": "app"}, headers)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", status, string(body))
	}
	env := testkit.DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("expected code=0, got %d err=%s", env.Code, env.Err)
	}
	var data struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if data.ID <= 0 {
		t.Fatalf("expected id > 0, got %d", data.ID)
	}
}

func newProxySecretServer(t *testing.T, secret string) *httptest.Server {
	t.Helper()

	gin.SetMode(gin.TestMode)

	db := testkit.OpenTestDB(t)
	publisher := &testkit.InlinePublisher{DB: db}

	cfg := config.Config{
		HTTPAddr:          "127.0.0.1:0",
		AuthSecret:        []byte(testkit.TestAuthSecret),
		AuthTokenTTL:      time.Hour,
		LogtapProxySecret: secret,
	}

	srv := httpserver.New(cfg, publisher, db, nil, nil)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}
