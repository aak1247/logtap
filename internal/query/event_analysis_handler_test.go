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
	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type apiEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Err  string          `json:"err"`
}

type topEventsPayload struct {
	Items []TopEventRow `json:"items"`
}

func openTopEventsHandlerTestDB(t testing.TB) *gorm.DB {
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

	if err := gdb.AutoMigrate(&model.Log{}, &model.TrackEvent{}, &model.TrackEventDaily{}); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return gdb
}

func fetchTopEvents(t *testing.T, r http.Handler, start, end time.Time) topEventsPayload {
	t.Helper()

	req := httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf(
			"/api/1/analytics/events/top?start=%s&end=%s&limit=20",
			url.QueryEscape(start.UTC().Format(time.RFC3339Nano)),
			url.QueryEscape(end.UTC().Format(time.RFC3339Nano)),
		),
		nil,
	)
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
		t.Fatalf("envelope code=%d err=%s", env.Code, env.Err)
	}
	var payload topEventsPayload
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return payload
}

func TestTopEventsHandler_FallbacksToLogsWhenDailyAndTrackEventsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTopEventsHandlerTestDB(t)

	now := time.Now().UTC()
	logRow := model.Log{
		ProjectID:  1,
		Timestamp:  now,
		Level:      "event",
		DistinctID: "u1",
		Message:    "signup",
		Fields:     datatypes.JSON([]byte("{}")),
	}
	if err := db.Create(&logRow).Error; err != nil {
		t.Fatalf("insert log: %v", err)
	}

	r := gin.New()
	r.GET("/api/:projectId/analytics/events/top", TopEventsHandler(db))

	payload := fetchTopEvents(t, r, now.Add(-time.Hour), now.Add(time.Hour))
	if len(payload.Items) == 0 {
		t.Fatalf("expected non-empty items")
	}
	var signup *TopEventRow
	for i := range payload.Items {
		if payload.Items[i].Name == "signup" {
			signup = &payload.Items[i]
			break
		}
	}
	if signup == nil {
		t.Fatalf("expected signup in items: %+v", payload.Items)
	}
	if signup.Events != 1 || signup.Users != 1 {
		t.Fatalf("unexpected signup counts: events=%d users=%d", signup.Events, signup.Users)
	}
}

func TestTopEventsHandler_PrefersDailyRollupWhenAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTopEventsHandlerTestDB(t)

	now := time.Now().UTC()
	logRow := model.Log{
		ProjectID:  1,
		Timestamp:  now,
		Level:      "event",
		DistinctID: "u1",
		Message:    "signup",
		Fields:     datatypes.JSON([]byte("{}")),
	}
	if err := db.Create(&logRow).Error; err != nil {
		t.Fatalf("insert log: %v", err)
	}
	daily := model.TrackEventDaily{
		ProjectID:  1,
		Day:        now.Format("2006-01-02"),
		Name:       "signup",
		DistinctID: "u1",
		Events:     3,
	}
	if err := db.Create(&daily).Error; err != nil {
		t.Fatalf("insert daily: %v", err)
	}

	r := gin.New()
	r.GET("/api/:projectId/analytics/events/top", TopEventsHandler(db))

	payload := fetchTopEvents(t, r, now.Add(-time.Hour), now.Add(time.Hour))
	if len(payload.Items) == 0 {
		t.Fatalf("expected non-empty items")
	}
	if payload.Items[0].Name != "signup" || payload.Items[0].Events != 3 || payload.Items[0].Users != 1 {
		t.Fatalf("unexpected first item: %+v", payload.Items[0])
	}
}
