package clickhouse

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"github.com/aak1247/logtap/internal/search"
	"github.com/aak1247/logtap/internal/store/clickhouse"
	"github.com/aak1247/logtap/internal/tenant"
	"github.com/google/uuid"
)

// TestAdapterSearch runs only when CLICKHOUSE_TEST_DSN is set; it exercises
// the SearchAdapter end to end against a real ClickHouse (schema, keyword
// contains, count, facet, highlight, cursor pagination and the TTL-managed
// DeleteBefore refusal).
func TestAdapterSearch(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("CLICKHOUSE_TEST_DSN"))
	if dsn == "" {
		t.Skip("CLICKHOUSE_TEST_DSN not set; skipping live adapter test")
	}
	connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
	clients, err := clickhouse.WaitForClickHouse(connectCtx, dsn, "")
	connectCancel()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer clients.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := clickhouse.Migrate(ctx, clients.Write, clickhouse.MigrateOptions{LogTTLDays: 30}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	id := uuid.MustParse("00000000-0000-0000-0000-0000000000b1")
	rows := clickhouse.LogRowsFromModel([]model.Log{
		{ProjectID: 77, Timestamp: now.Add(-time.Hour), IngestID: &id, Level: "error", Message: "payment gateway timeout", DistinctID: "u9", Fields: []byte(`{"environment":"prod"}`)},
	})
	rows[0].WithTenant(tenant.DefaultTenantID)
	if err := clickhouse.InsertLogs(ctx, clients.Write, rows); err != nil {
		t.Fatalf("InsertLogs: %v", err)
	}

	adapter := NewAdapter(clients.Read)
	if adapter.Type() != "clickhouse" {
		t.Fatalf("unexpected adapter type %q", adapter.Type())
	}
	res, err := adapter.Search(ctx, search.SearchQuery{
		ProjectID:  77,
		TimeRange:  search.TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)},
		Keywords:   []string{"timeout"},
		Pagination: search.Pagination{Limit: 10},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.Total != 1 || len(res.Hits) != 1 {
		t.Fatalf("result: total=%d hits=%d", res.Total, len(res.Hits))
	}
	if res.Hits[0].Highlight == nil || len(res.Hits[0].Highlight["message"]) == 0 {
		t.Fatalf("missing highlight")
	}
	levelFacet, hasFacet := res.Facets["level"]
	if !hasFacet || len(levelFacet.Buckets) == 0 {
		t.Fatalf("missing level facet")
	}

	// Whitelisted map filter and unknown-field rejection.
	res, err = adapter.Search(ctx, search.SearchQuery{
		ProjectID:  77,
		TimeRange:  search.TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)},
		Filters:    []search.Filter{{Field: "environment", Operator: "eq", Value: "prod"}},
		Pagination: search.Pagination{Limit: 10},
	})
	if err != nil || res.Total != 1 {
		t.Fatalf("map filter: total=%d err=%v", res.Total, err)
	}
	res, err = adapter.Search(ctx, search.SearchQuery{
		ProjectID:  77,
		TimeRange:  search.TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)},
		Filters:    []search.Filter{{Field: "dropsides", Operator: "eq", Value: "x"}},
		Pagination: search.Pagination{Limit: 10},
	})
	if err != nil || res.Total != 0 {
		t.Fatalf("unknown field must match nothing: total=%d err=%v", res.Total, err)
	}

	// Cursor pagination: keyset semantics are "strictly before the marker",
	// so a marker placed exactly on the row must exclude it.
	marker := clickhouse.EncodeCursor(clickhouse.SearchCursor{
		Timestamp: rows[0].Timestamp,
		IngestID:  id.String(),
	})
	res, err = adapter.Search(ctx, search.SearchQuery{
		ProjectID:  77,
		TimeRange:  search.TimeRange{Start: now.Add(-24 * time.Hour), End: now.Add(time.Hour)},
		Pagination: search.Pagination{Limit: 10, Cursor: marker},
	})
	if err != nil {
		t.Fatalf("cursor page: %v", err)
	}
	if res.Total != 1 || len(res.Hits) != 0 {
		t.Fatalf("cursor page: total=%d hits=%d", res.Total, len(res.Hits))
	}

	if err := adapter.DeleteBefore(ctx, 77, now); err == nil {
		t.Fatalf("DeleteBefore must refuse on ClickHouse (TTL-managed)")
	}
	if err := adapter.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
