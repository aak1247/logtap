package clickhouse

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/tenant"
	"github.com/google/uuid"
)

// TestCursorRoundTrip is pure unit coverage (no ClickHouse needed).
func TestCursorRoundTrip(t *testing.T) {
	c := SearchCursor{Timestamp: time.Now().UTC().Truncate(time.Millisecond), IngestID: uuid.NewString()}
	enc := EncodeCursor(c)
	got, err := DecodeCursor(enc)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}
	if !got.Timestamp.Equal(c.Timestamp) || got.IngestID != c.IngestID {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, c)
	}
	if _, err := DecodeCursor("not-a-cursor"); err == nil {
		t.Fatalf("expected error for invalid cursor")
	}
}

func TestKeywordExpr(t *testing.T) {
	if expr, args := KeywordExpr("", "fts"); expr != "" || args != nil {
		t.Fatalf("empty keyword must be empty expr")
	}
	expr, args := KeywordExpr("timeout", "")
	if expr != "hasToken(message, ?)" || len(args) != 1 || args[0] != "timeout" {
		t.Fatalf("single token: %q %v", expr, args)
	}
	expr, args = KeywordExpr("conn reset", "")
	if expr != "hasAllTokens(message, ?)" || len(args) != 2 {
		t.Fatalf("multi token: %q %v", expr, args)
	}
	expr, _ = KeywordExpr("time out", "contains")
	if expr != "positionCaseInsensitive(message, ?) > 0" {
		t.Fatalf("contains mode: %q", expr)
	}
}

// TestWhereAlwaysTenantScoped pins the isolation contract: every generated
// WHERE clause starts with the tenant predicate (design §7.1).
func TestWhereAlwaysTenantScoped(t *testing.T) {
	f := LogsFilter{ProjectID: 7, Start: time.Now().Add(-time.Hour), End: time.Now(), Level: "error"}
	where, args := f.where()
	if len(where) < len("tenant_id = ? AND project_id = ?") || where[:len("tenant_id = ?")] != "tenant_id = ?" {
		t.Fatalf("where must be tenant-scoped first: %q", where)
	}
	if len(args) < 2 || args[0] != tenant.DefaultTenantID {
		t.Fatalf("first bind must be the tenant id, got %v", args)
	}
}

// TestClickHouseBackendLifecycle is the live contract suite. It runs only
// when CLICKHOUSE_TEST_DSN is set (e.g.
// CLICKHOUSE_TEST_DSN='clickhouse://default@127.0.0.1:19000/logtap'):
//
//	CLICKHOUSE_TEST_DSN=... go test ./internal/store/clickhouse/ -run TestClickHouseBackendLifecycle
func TestClickHouseBackendLifecycle(t *testing.T) {
	dsn := testDSN()
	if dsn == "" {
		t.Skip("CLICKHOUSE_TEST_DSN not set; skipping live ClickHouse contract test")
	}
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	clients, err := WaitForClickHouse(connectCtx, dsn, "")
	connectCancel()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer clients.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := Migrate(ctx, clients.Write, MigrateOptions{LogTTLDays: 30}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Fresh slate for deterministic assertions on our own data.
	for _, table := range []string{"logs_daily_mv", "track_events_daily_mv", "log_daily_stats", "track_event_daily", "logs", "events", "track_events"} {
		_ = clients.Write.Exec(ctx, "DROP TABLE IF EXISTS logtap."+table)
	}
	if err := Migrate(ctx, clients.Write, MigrateOptions{LogTTLDays: 30}); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	ingest := func(id string) *uuid.UUID { u := uuid.MustParse(id); return &u }
	logs := []model.Log{
		{ProjectID: 42, Timestamp: now.Add(-3 * time.Hour), IngestID: ingest("00000000-0000-0000-0000-0000000000a1"), Level: "error", Message: "connection timeout to db", TraceID: "tr-1", DistinctID: "u1", DeviceID: "d1", Fields: []byte(`{"environment":"prod"}`)},
		{ProjectID: 42, Timestamp: now.Add(-2 * time.Hour), IngestID: ingest("00000000-0000-0000-0000-0000000000a2"), Level: "error", Message: "connection timeout to cache", TraceID: "tr-2", DistinctID: "u2", Fields: []byte(`{"environment":"prod"}`)},
		{ProjectID: 42, Timestamp: now.Add(-2 * time.Hour), IngestID: ingest("00000000-0000-0000-0000-0000000000a3"), Level: "info", Message: "service started", DistinctID: "u1"},
		{ProjectID: 42, Timestamp: now.Add(-1 * time.Hour), IngestID: ingest("00000000-0000-0000-0000-0000000000a4"), Level: "info", Message: "health check ok", DistinctID: "u2"},
		{ProjectID: 42, Timestamp: now.Add(-30 * time.Minute), IngestID: ingest("00000000-0000-0000-0000-0000000000a5"), Level: "event", Message: "login", DistinctID: "u1", DeviceID: "d1"},
		{ProjectID: 42, Timestamp: now.Add(-29 * time.Minute), IngestID: ingest("00000000-0000-0000-0000-0000000000a6"), Level: "event", Message: "login", DistinctID: "u2"},
	}
	rows := LogRowsFromModel(logs)
	for i := range rows {
		rows[i].WithTenant(tenant.DefaultTenantID)
	}
	if err := InsertLogs(ctx, clients.Write, rows); err != nil {
		t.Fatalf("InsertLogs: %v", err)
	}
	teRows := TrackEventRowsFromModel([]model.TrackEvent{
		{ProjectID: 42, Timestamp: now.Add(-30 * time.Minute), IngestID: ingest("00000000-0000-0000-0000-0000000000a5"), Name: "login", DistinctID: "u1", DeviceID: "d1"},
		{ProjectID: 42, Timestamp: now.Add(-29 * time.Minute), IngestID: ingest("00000000-0000-0000-0000-0000000000a6"), Name: "login", DistinctID: "u2"},
	})
	for i := range teRows {
		teRows[i].WithTenant(tenant.DefaultTenantID)
	}
	if err := InsertTrackEvents(ctx, clients.Write, teRows); err != nil {
		t.Fatalf("InsertTrackEvents: %v", err)
	}
	events := []model.Event{
		{ID: uuid.MustParse("10000000-0000-0000-0000-0000000000e1"), ProjectID: 42, Timestamp: now.Add(-time.Hour), Level: "error", Title: "TypeError", DistinctID: "u1", Data: []byte(`{"event_id":"10000000-0000-0000-0000-0000000000e1","title":"TypeError"}`)},
	}
	eRows := EventRowsFromModel(events)
	for i := range eRows {
		eRows[i].WithTenant(tenant.DefaultTenantID)
	}
	if err := InsertEvents(ctx, clients.Write, eRows); err != nil {
		t.Fatalf("InsertEvents: %v", err)
	}

	// A second tenant with the SAME project id must be fully invisible.
	other := tenant.ID("99999999-9999-9999-9999-999999999999")
	otherRows := LogRowsFromModel(logs[:1])
	otherRows[0].WithTenant(other)
	if err := InsertLogs(ctx, clients.Write, otherRows); err != nil {
		t.Fatalf("InsertLogs(other tenant): %v", err)
	}

	t.Run("countAndSearch", func(t *testing.T) {
		f := LogsFilter{TenantID: tenant.DefaultTenantID, ProjectID: 42, Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour), Level: "error"}
		n, err := CountLogs(ctx, clients.Read, f)
		if err != nil || n != 2 {
			t.Fatalf("CountLogs error=2: n=%d err=%v", n, err)
		}
		f.Keyword = "timeout"
		f.Mode = "contains"
		n, err = CountLogs(ctx, clients.Read, f)
		if err != nil || n != 2 {
			t.Fatalf("contains search: n=%d err=%v", n, err)
		}
		entries, err := SearchLogs(ctx, clients.Read, f, 10, nil)
		if err != nil || len(entries) != 2 {
			t.Fatalf("SearchLogs: %d entries err=%v", len(entries), err)
		}
		if entries[0].Timestamp.Before(entries[1].Timestamp) {
			t.Fatalf("entries must be timestamp DESC")
		}
		if entries[0].Fields["environment"] != "prod" {
			t.Fatalf("fields map not round-tripped: %v", entries[0].Fields)
		}
		// Cross-tenant isolation: the other tenant sees exactly its own row,
		// and the counts above prove the default tenant never sees it.
		fOther := f
		fOther.TenantID = other
		nOther, err := CountLogs(ctx, clients.Read, fOther)
		if err != nil || nOther != 1 {
			t.Fatalf("other tenant must see exactly its own row: nOther=%d err=%v", nOther, err)
		}
	})

	t.Run("keysetPagination", func(t *testing.T) {
		f := LogsFilter{TenantID: tenant.DefaultTenantID, ProjectID: 42, Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)}
		seen := map[string]bool{}
		var cursor *SearchCursor
		pages := 0
		for {
			entries, err := SearchLogs(ctx, clients.Read, f, 2, cursor)
			if err != nil {
				t.Fatalf("page %d: %v", pages, err)
			}
			for _, e := range entries {
				if seen[e.IngestID] {
					t.Fatalf("duplicate ingest_id across pages: %s", e.IngestID)
				}
				seen[e.IngestID] = true
			}
			pages++
			if len(entries) < 2 {
				break
			}
			last := entries[len(entries)-1]
			cursor = &SearchCursor{Timestamp: last.Timestamp, IngestID: last.IngestID}
			if pages > 10 {
				t.Fatalf("pagination did not terminate")
			}
		}
		if len(seen) != len(logs) {
			t.Fatalf("pagination lost rows: seen=%d want=%d", len(seen), len(logs))
		}
	})

	t.Run("facetAndTrend", func(t *testing.T) {
		f := LogsFilter{TenantID: tenant.DefaultTenantID, ProjectID: 42, Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)}
		facet, err := LevelFacet(ctx, clients.Read, f)
		if err != nil {
			t.Fatalf("LevelFacet: %v", err)
		}
		if facet["error"] != 2 || facet["info"] != 2 || facet["event"] != 2 {
			t.Fatalf("unexpected facet: %v", facet)
		}
		points, tops, err := LogTrend(ctx, clients.Read, f, "hour", 10)
		if err != nil {
			t.Fatalf("LogTrend: %v", err)
		}
		if len(points) == 0 {
			t.Fatalf("no trend points")
		}
		total := int64(0)
		for _, p := range points {
			total += p.Count
		}
		if total != 6 {
			t.Fatalf("trend total=%d want 6", total)
		}
		if len(tops) == 0 || tops[0].Count < 2 {
			t.Fatalf("unexpected tops: %+v", tops)
		}
	})

	t.Run("metricsRollup", func(t *testing.T) {
		// MVs fire synchronously on insert, so the rollup is already there.
		day := now
		stats, err := MetricsByLevel(ctx, clients.Read, 42, &day, &day)
		if err != nil {
			t.Fatalf("MetricsByLevel: %v", err)
		}
		var logsN, eventsN, errorsN int64
		for _, s := range stats {
			logsN += s.Count
			if s.Level == "event" {
				eventsN = s.Count
			}
			if s.Level == "error" || s.Level == "fatal" {
				errorsN += s.Count
			}
		}
		usersN, err := MetricsUsers(ctx, clients.Read, 42, &day, &day)
		if err != nil {
			t.Fatalf("MetricsUsers: %v", err)
		}
		if logsN != 6 || eventsN != 2 || errorsN != 2 || usersN != 2 {
			t.Fatalf("rollup mismatch: logs=%d events=%d errors=%d users=%d", logsN, eventsN, errorsN, usersN)
		}
		exactLogs, exactUsers, err := MetricsExact(ctx, clients.Read, 42, now.Add(-24*time.Hour), now.Add(time.Hour))
		if err != nil || exactLogs != 6 || exactUsers != 2 {
			t.Fatalf("MetricsExact: logs=%d users=%d err=%v", exactLogs, exactUsers, err)
		}
	})

	t.Run("events", func(t *testing.T) {
		recent, err := RecentEvents(ctx, clients.Read, 42, 10)
		if err != nil || len(recent) != 1 {
			t.Fatalf("RecentEvents: %d err=%v", len(recent), err)
		}
		data, ok, err := GetEventData(ctx, clients.Read, 42, uuid.MustParse("10000000-0000-0000-0000-0000000000e1"))
		if err != nil || !ok || !strings.Contains(data, "TypeError") {
			t.Fatalf("GetEventData: ok=%v data=%q err=%v", ok, data, err)
		}
	})

	t.Run("topEventsAndFunnel", func(t *testing.T) {
		top, err := TopEvents(ctx, clients.Read, 42, now.Add(-24*time.Hour), now.Add(time.Hour), 10, "")
		if err != nil {
			t.Fatalf("TopEvents: %v", err)
		}
		if len(top) == 0 || top[0].Name != "login" || top[0].Events != 2 {
			t.Fatalf("unexpected top: %+v", top)
		}
		counts, used, err := FunnelCounts(ctx, clients.Read, 42, []string{"login", "logout"}, now.Add(-24*time.Hour), now.Add(time.Hour), 3600, "")
		if err != nil {
			t.Fatalf("FunnelCounts: %v", err)
		}
		if used != "track_events" || counts[0] != 2 || counts[1] != 0 {
			t.Fatalf("funnel counts=%v used=%s", counts, used)
		}
	})

	t.Run("customAnalytics", func(t *testing.T) {
		stats, err := CustomAnalytics(ctx, clients.Read, 42, "day", now.Add(-24*time.Hour), now.Add(time.Hour), true, false, "", nil, true)
		if err != nil {
			t.Fatalf("CustomAnalytics: %v", err)
		}
		total := int64(0)
		for _, s := range stats {
			total += s.Events
		}
		if total != 2 {
			t.Fatalf("custom analytics events=%d want 2", total)
		}
	})

	t.Run("storageEstimate", func(t *testing.T) {
		logsEst, eventsEst, err := StorageEstimate(ctx, clients.Read, 42)
		if err != nil {
			t.Fatalf("StorageEstimate: %v", err)
		}
		if logsEst.Count != 6 || eventsEst.Count != 1 {
			t.Fatalf("estimate counts: logs=%d events=%d", logsEst.Count, eventsEst.Count)
		}
		if logsEst.EstBytes <= 0 {
			t.Fatalf("estimate bytes must be positive")
		}
	})
}

func testDSN() string {
	return strings.TrimSpace(os.Getenv("CLICKHOUSE_TEST_DSN"))
}
