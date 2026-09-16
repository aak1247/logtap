package query

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/gin-gonic/gin"
)

type logTrendPayload struct {
	ProjectID int              `json:"project_id"`
	Level     string           `json:"level"`
	Bucket    string           `json:"bucket"`
	Labels    []string         `json:"labels"`
	Points    []int64          `json:"points"`
	Top       []map[string]any `json:"top"`
}

func fetchLogTrend(t *testing.T, r http.Handler, query string) logTrendPayload {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/api/1/logs/trend"+query, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var env apiEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Code != 0 {
		t.Fatalf("code=%d err=%s", env.Code, env.Err)
	}
	var out logTrendPayload
	if err := json.Unmarshal(env.Data, &out); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return out
}

func TestLogTrendHandler(t *testing.T) {
	db := openTopEventsHandlerTestDB(t)

	base := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	logs := []model.Log{
		{ProjectID: 1, Timestamp: base, Level: "error", Message: "boom a"},
		{ProjectID: 1, Timestamp: base.Add(30 * time.Minute), Level: "error", Message: "boom a"},
		{ProjectID: 1, Timestamp: base.Add(90 * time.Minute), Level: "error", Message: "boom b"},
		{ProjectID: 1, Timestamp: base.Add(10 * time.Minute), Level: "info", Message: "boom a"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/:projectId/logs/trend", LogTrendHandler(db))

	q := fmt.Sprintf(
		"?level=error&start=%s&end=%s",
		url.QueryEscape(base.Format(time.RFC3339)),
		url.QueryEscape(base.Add(2*time.Hour).Format(time.RFC3339)),
	)
	got := fetchLogTrend(t, r, q)

	if len(got.Points) != 2 || got.Points[0] != 2 || got.Points[1] != 1 {
		t.Fatalf("unexpected points: %+v (labels=%v)", got.Points, got.Labels)
	}
	if len(got.Labels) != 2 || got.Labels[0] != "2026-05-01T10:00:00Z" {
		t.Fatalf("unexpected labels: %v", got.Labels)
	}
	if len(got.Top) != 2 {
		t.Fatalf("unexpected top rows: %+v", got.Top)
	}
	if got.Top[0]["message"] != "boom a" || got.Top[0]["count"] != float64(2) {
		t.Fatalf("unexpected top[0]: %+v", got.Top[0])
	}
	if got.Top[1]["message"] != "boom b" || got.Top[1]["count"] != float64(1) {
		t.Fatalf("unexpected top[1]: %+v", got.Top[1])
	}
}

func TestLogTrendHandler_DayBucketZeroFill(t *testing.T) {
	db := openTopEventsHandlerTestDB(t)

	day1 := time.Date(2026, 5, 1, 8, 15, 0, 0, time.UTC)
	logs := []model.Log{
		{ProjectID: 1, Timestamp: day1, Level: "error", Message: "boom"},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/:projectId/logs/trend", LogTrendHandler(db))

	q := fmt.Sprintf(
		"?level=error&bucket=day&start=%s&end=%s",
		url.QueryEscape(day1.Format(time.RFC3339)),
		url.QueryEscape(day1.Add(72*time.Hour).Format(time.RFC3339)),
	)
	got := fetchLogTrend(t, r, q)

	// The window [day1, day1+72h) spans four UTC day buckets (05-01..05-04).
	if len(got.Points) != 4 {
		t.Fatalf("expected 4 day buckets, got %d (%v)", len(got.Points), got.Points)
	}
	if got.Points[0] != 1 || got.Points[1] != 0 || got.Points[2] != 0 || got.Points[3] != 0 {
		t.Fatalf("unexpected zero-filled points: %v", got.Points)
	}
}
