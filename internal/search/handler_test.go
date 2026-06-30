package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type stubAdapter struct {
	result *SearchResult
}

func (a stubAdapter) Type() string                                        { return "stub" }
func (a stubAdapter) Index(_ context.Context, _ int, _ []SearchHit) error { return nil }
func (a stubAdapter) Search(_ context.Context, _ SearchQuery) (*SearchResult, error) {
	return a.result, nil
}
func (a stubAdapter) DeleteBefore(_ context.Context, _ int, _ time.Time) error { return nil }
func (a stubAdapter) Ping(_ context.Context) error                             { return nil }

func TestSearchHandlerReturnsEnvelopeWithItems(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := NewEngine(stubAdapter{result: &SearchResult{
		Total: 1,
		Hits: []SearchHit{{
			ID:        int64(1),
			Type:      "log",
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Level:     "info",
			Message:   "hello",
		}},
		Facets: map[string]Facet{
			"level": {Field: "level", Buckets: []FacetBucket{{Key: "info", Count: 1}}},
		},
	}})
	r := gin.New()
	r.GET("/api/:projectId/search", SearchHandler(engine))

	req := httptest.NewRequest(http.MethodGet, "/api/1/search?q=hello", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var env struct {
		Code int `json:"code"`
		Data struct {
			Total int64 `json:"total"`
			Items []struct {
				ID      int64  `json:"id"`
				Message string `json:"message"`
			} `json:"items"`
			Facets map[string][]FacetBucket `json:"facets"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Code != 0 || env.Data.Total != 1 || len(env.Data.Items) != 1 || env.Data.Items[0].Message != "hello" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
	if len(env.Data.Facets["level"]) != 1 {
		t.Fatalf("expected level facet: %+v", env.Data.Facets)
	}
}
