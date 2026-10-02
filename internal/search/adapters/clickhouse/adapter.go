// Package clickhouse implements search.SearchAdapter over the ClickHouse
// logs table (design §6.1): deterministic boolean filters (jieba text index
// for keyword tokens, positionCaseInsensitive for contains), exact counts,
// level facets and keyset or offset pagination.
package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/aak1247/logtap/internal/search"
	"github.com/aak1247/logtap/internal/store/clickhouse"
	"github.com/aak1247/logtap/internal/tenant"
)

// Adapter is the ClickHouse SearchAdapter implementation.
type Adapter struct {
	conn chdriver.Conn
}

func NewAdapter(conn chdriver.Conn) *Adapter {
	return &Adapter{conn: conn}
}

func (a *Adapter) Type() string { return "clickhouse" }

func (a *Adapter) Ping(ctx context.Context) error {
	return a.conn.Ping(ctx)
}

// Index is a no-op: rows land in ClickHouse through the ingestion path, the
// same contract as the Postgres adapter.
func (a *Adapter) Index(_ context.Context, _ int, _ []search.SearchHit) error {
	return nil
}

// DeleteBefore reports that retention is TTL-managed; cleanup workers must
// skip the ClickHouse backend instead of issuing row deletions.
func (a *Adapter) DeleteBefore(_ context.Context, _ int, _ time.Time) error {
	return fmt.Errorf("clickhouse: retention is managed by table TTL (ttl_only_drop_parts); per-project delete is not supported")
}

func filterToLogsFilter(ctx context.Context, q search.SearchQuery) clickhouse.LogsFilter {
	f := clickhouse.LogsFilter{
		TenantID:  tenant.FromOrDefault(ctx),
		ProjectID: uint32(q.ProjectID),
		Start:     q.TimeRange.Start,
		End:       q.TimeRange.End,
	}
	// Keywords behave like the Postgres adapter's ILIKE: AND-ed
	// case-insensitive substring matches (contains mode).
	f.Keywords = append(f.Keywords, q.Keywords...)
	for _, fl := range q.Filters {
		col, mapKey, known := mapColumn(fl.Field)
		if !known {
			// Match the Postgres adapter: unknown fields match nothing.
			f.Filters = append(f.Filters, clickhouse.RawFilter{Expr: "1 = 0"})
			continue
		}
		expr, args, ok := filterExpr(col, mapKey, fl)
		if !ok {
			f.Filters = append(f.Filters, clickhouse.RawFilter{Expr: "1 = 0"})
			continue
		}
		f.Filters = append(f.Filters, clickhouse.RawFilter{Expr: expr, Args: args})
	}
	return f
}

// mapColumn mirrors the Postgres adapter whitelist; map fields are expressed
// as (mapKey) so the caller binds fields['key'] lookups.
func mapColumn(field string) (col string, mapKey string, known bool) {
	switch strings.ToLower(strings.TrimSpace(field)) {
	case "level":
		return "level", "", true
	case "trace_id", "traceid":
		return "trace_id", "", true
	case "span_id", "spanid":
		return "span_id", "", true
	case "message":
		return "message", "", true
	case "environment":
		return "", "environment", true
	case "service":
		return "", "service", true
	case "tag":
		return "", "tag", true
	default:
		return "", "", false
	}
}

func filterExpr(col, mapKey string, f search.Filter) (expr string, args []any, ok bool) {
	op := strings.ToLower(f.Operator)
	if op == "" {
		op = "eq"
	}
	if col != "" {
		switch op {
		case "eq":
			return col + " = ?", []any{f.Value}, true
		case "neq":
			return col + " != ?", []any{f.Value}, true
		case "contains":
			return "positionCaseInsensitive(" + col + ", ?) > 0", []any{fmt.Sprint(f.Value)}, true
		case "in":
			values, isSlice := f.Value.([]any)
			if !isSlice {
				return "", nil, false
			}
			placeholders := make([]string, len(values))
			for i := range values {
				placeholders[i] = "?"
			}
			return col + " IN (" + strings.Join(placeholders, ",") + ")", values, true
		case "exists":
			return col + " != ''", nil, true
		case "not_exists":
			return col + " = ''", nil, true
		default:
			return col + " = ?", []any{f.Value}, true
		}
	}
	ref := "fields['" + mapKey + "']"
	switch op {
	case "eq":
		return ref + " = ?", []any{fmt.Sprint(f.Value)}, true
	case "neq":
		return ref + " != ?", []any{fmt.Sprint(f.Value)}, true
	case "contains":
		return "positionCaseInsensitive(" + ref + ", ?) > 0", []any{fmt.Sprint(f.Value)}, true
	case "in":
		values, isSlice := f.Value.([]any)
		if !isSlice {
			return "", nil, false
		}
		placeholders := make([]string, len(values))
		for i := range values {
			placeholders[i] = "?"
		}
		return ref + " IN (" + strings.Join(placeholders, ",") + ")", values, true
	case "exists":
		return ref + " != ''", nil, true
	case "not_exists":
		return ref + " = ''", nil, true
	default:
		return ref + " = ?", []any{fmt.Sprint(f.Value)}, true
	}
}

// Search translates a SearchQuery into ClickHouse SQL: one exact count, one
// page of rows and one level facet.
func (a *Adapter) Search(ctx context.Context, q search.SearchQuery) (*search.SearchResult, error) {
	f := filterToLogsFilter(ctx, q)

	total, err := clickhouse.CountLogs(ctx, a.conn, f)
	if err != nil {
		return nil, fmt.Errorf("search count: %w", err)
	}

	limit := q.Pagination.Limit
	if limit <= 0 {
		limit = 50
	}
	var cursor *clickhouse.SearchCursor
	if q.Pagination.Cursor != "" {
		c, err := clickhouse.DecodeCursor(q.Pagination.Cursor)
		if err != nil {
			return nil, err
		}
		cursor = &c
	}
	rows, err := clickhouse.SearchLogs(ctx, a.conn, f, limit, cursor)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}

	hits := make([]search.SearchHit, 0, len(rows))
	for _, r := range rows {
		hit := search.SearchHit{
			ID:        r.IngestID,
			Type:      "log",
			Timestamp: r.Timestamp,
			Level:     r.Level,
			Message:   r.Message,
			Fields:    map[string]any{},
		}
		for k, v := range r.Fields {
			hit.Fields[k] = v
		}
		if len(q.Keywords) > 0 {
			hit.Highlight = highlightFields(r.Message, q.Keywords)
		}
		hits = append(hits, hit)
	}

	facets := make(map[string]search.Facet)
	if len(q.Filters) > 0 || len(q.Keywords) > 0 || !q.TimeRange.Start.IsZero() {
		buckets, err := clickhouse.LevelFacet(ctx, a.conn, f)
		if err == nil && len(buckets) > 0 {
			fb := make([]search.FacetBucket, 0, len(buckets))
			for level, count := range buckets {
				fb = append(fb, search.FacetBucket{Key: level, Count: count})
			}
			facets["level"] = search.Facet{Field: "level", Buckets: fb}
		}
	}

	return &search.SearchResult{Total: total, Hits: hits, Facets: facets}, nil
}

// highlightFields mirrors the Postgres adapter's app-layer snippet logic.
func highlightFields(message string, keywords []string) map[string][]string {
	var snippets []string
	lower := strings.ToLower(message)
	for _, kw := range keywords {
		idx := strings.Index(lower, strings.ToLower(kw))
		if idx >= 0 {
			start := idx - 30
			if start < 0 {
				start = 0
			}
			end := idx + len(kw) + 30
			if end > len(message) {
				end = len(message)
			}
			snippets = append(snippets, message[start:end])
		}
	}
	if len(snippets) == 0 {
		return nil
	}
	return map[string][]string{"message": snippets}
}
