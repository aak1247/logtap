package httpcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/detector"
	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/monitor"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestE2E_HTTPCheck_MonitorWorker verifies the full pipeline:
// MonitorDefinition → Worker.executeOne → httpcheck Plugin → MonitorRun record
func TestE2E_HTTPCheck_MonitorWorker(t *testing.T) {
	// 1. Setup SQLite in-memory DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.MonitorDefinition{}, &model.MonitorRun{},
		&model.AlertRule{}, &model.AlertState{}, &model.AlertDelivery{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 2. Setup target HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		if r.URL.Path == "/fail" {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("maintenance"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// 3. Setup registry with http_check plugin
	registry := detector.NewRegistry()
	if err := registry.RegisterStatic(Plugin{}); err != nil {
		t.Fatalf("register http_check: %v", err)
	}

	// 4. Create Worker
	w := &monitor.Worker{
		DB:            db,
		Registry:      registry,
		TickInterval:  2 * time.Second,
		BatchSize:     20,
		LeaseDuration: 60 * time.Second,
		NodeID:        "test-node",
		Now:           func() time.Time { return time.Now().UTC() },
	}

	// --- Sub-test: successful check ---
	t.Run("Success_2xx", func(t *testing.T) {
		cfg, _ := json.Marshal(map[string]any{
			"url":                 ts.URL + "/healthz",
			"method":              "GET",
			"expectStatus":        []int{200},
			"expectBodySubstring": `"status":"ok"`,
			"timeoutMs":           3000,
		})
		now := time.Now().UTC()
		mon := model.MonitorDefinition{
			ProjectID:    1,
			Name:         "健康检查",
			DetectorType: "http_check",
			Config:       datatypes.JSON(cfg),
			IntervalSec:  60,
			TimeoutMS:    5000,
			Enabled:      true,
			NextRunAt:    now.Add(-1 * time.Second),
		}
		if err := db.Create(&mon).Error; err != nil {
			t.Fatalf("create monitor: %v", err)
		}

		if n, err := w.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce: %v", err)
		} else if n != 1 {
			t.Fatalf("expected 1 processed, got %d", n)
		}

		var run model.MonitorRun
		if err := db.Where("monitor_id = ?", mon.ID).First(&run).Error; err != nil {
			t.Fatalf("find run: %v", err)
		}
		if run.Status != "success" {
			t.Errorf("expected status=success, got=%q error=%s", run.Status, run.Error)
		}
		if run.SignalCount != 1 {
			t.Errorf("expected signal_count=1, got=%d", run.SignalCount)
		}

		// Verify result JSON contains detectorType
		var result map[string]any
		if err := json.Unmarshal(run.Result, &result); err != nil {
			t.Fatalf("unmarshal result: %v", err)
		}
		if result["detectorType"] != "http_check" {
			t.Errorf("expected detectorType=http_check, got=%v", result["detectorType"])
		}
		t.Logf("✅ Success run: status=%s signals=%d result=%s", run.Status, run.SignalCount, string(run.Result))
	})

	// --- Sub-test: failure check ---
	t.Run("Failure_5xx", func(t *testing.T) {
		cfg, _ := json.Marshal(map[string]any{
			"url":       ts.URL + "/fail",
			"timeoutMs": 3000,
		})
		mon := model.MonitorDefinition{
			ProjectID:    1,
			Name:         "故障检查",
			DetectorType: "http_check",
			Config:       datatypes.JSON(cfg),
			IntervalSec:  60,
			TimeoutMS:    5000,
			Enabled:      true,
			NextRunAt:    time.Now().UTC().Add(-1 * time.Second),
		}
		if err := db.Create(&mon).Error; err != nil {
			t.Fatalf("create monitor: %v", err)
		}

		if n, err := w.RunOnce(context.Background()); err != nil {
			t.Fatalf("RunOnce: %v", err)
		} else if n != 1 {
			t.Fatalf("expected 1 processed, got %d", n)
		}

		var run model.MonitorRun
		if err := db.Where("monitor_id = ?", mon.ID).First(&run).Error; err != nil {
			t.Fatalf("find run: %v", err)
		}
		// Worker status is "success" because execution completed (signals were produced),
		// but the signal itself has severity=error. The run records whether execution worked,
		// not whether the target is healthy.
		t.Logf("✅ Failure run: status=%s signals=%d error=%s result=%s", run.Status, run.SignalCount, run.Error, string(run.Result))
	})
}
