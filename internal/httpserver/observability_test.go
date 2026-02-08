package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/config"
	"github.com/aak1247/logtap/internal/obs"
	"github.com/gin-gonic/gin"
)

func TestDebugMetricsEndpoint(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	stats := obs.New()

	cfg := config.Config{
		HTTPAddr:             "127.0.0.1:0",
		AuthSecret:           []byte("01234567890123456789012345678901"),
		AuthTokenTTL:         time.Hour,
		EnableDebugEndpoints: true,
	}

	srv := New(cfg, nil, nil, nil, stats)
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)

	// Seed one request so /debug/metrics snapshot includes it.
	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = res.Body.Close()

	res2, err := http.Get(ts.URL + "/debug/metrics")
	if err != nil {
		t.Fatalf("GET /debug/metrics: %v", err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res2.StatusCode)
	}
	var body struct {
		Code int `json:"code"`
		Data struct {
			HTTP struct {
				Requests int64 `json:"requests"`
			} `json:"http"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Code != 0 {
		t.Fatalf("expected code=0, got %d", body.Code)
	}
	if body.Data.HTTP.Requests < 1 {
		t.Fatalf("expected http.requests >= 1, got %d", body.Data.HTTP.Requests)
	}
}
