package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/search"
	"gorm.io/gorm"
)

// PostgresAdapter implements search.SearchAdapter by querying existing
// logs and events tables directly. No storage changes in v1.
type PostgresAdapter struct {
	db *gorm.DB
}

func NewAdapter(db *gorm.DB) *PostgresAdapter {
	return &PostgresAdapter{db: db}
}

func (a *PostgresAdapter) Type() string { return "postgres" }

// Ping checks database connectivity.
func (a *PostgresAdapter) Ping(ctx context.Context) error {
	sqlDB, err := a.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// EnsureExtensions creates pg_trgm extension if not present (idempotent).
func (a *PostgresAdapter) EnsureExtensions(ctx context.Context) error {
	return a.db.WithContext(ctx).Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error
}

// Index is a no-op for v1: data is already in PG via existing ingest.
func (a *PostgresAdapter) Index(_ context.Context, _ int, _ []search.SearchHit) error {
	return nil
}

// DeleteBefore delegates to existing cleanup; kept for interface compliance.
func (a *PostgresAdapter) DeleteBefore(ctx context.Context, projectID int, before time.Time) error {
	return a.db.WithContext(ctx).
		Where("project_id = ? AND timestamp < ?", projectID, before).
		Delete(nil).Error // caller should use table-specific logic
}

// Search translates a SearchQuery into SQL against the logs table (v1).
func (a *PostgresAdapter) Search(ctx context.Context, q search.SearchQuery) (*search.SearchResult, error) {
	// Build base query on logs table
	qdb := a.db.WithContext(ctx).Table("logs").
		Where("project_id = ?", q.ProjectID)

	// Time range
	if !q.TimeRange.Start.IsZero() {
		qdb = qdb.Where("timestamp >= ?", q.TimeRange.Start)
	}
	if !q.TimeRange.End.IsZero() {
		qdb = qdb.Where("timestamp <= ?", q.TimeRange.End)
	}

	// Filters
	for _, f := range q.Filters {
		qdb = applyFilter(qdb, f)
	}

	// Keywords → ILIKE on message
	for _, kw := range q.Keywords {
		qdb = qdb.Where("message ILIKE ?", "%"+kw+"%")
	}

	// Count total
	var total int64
	if err := qdb.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("search count: %w", err)
	}

	// Order
	order := "timestamp DESC"
	if q.Sort.Field != "" {
		dir := "DESC"
		if strings.EqualFold(q.Sort.Order, "asc") {
			dir = "ASC"
		}
		order = q.Sort.Field + " " + dir
	}

	// Fetch rows
	type hitRow struct {
		ID        int64
		Timestamp time.Time
		Level     string
		TraceID   string
		SpanID    string
		Message   string
		Fields    string // jsonb as string
	}
	var rows []hitRow
	selectCols := "id, timestamp, level, trace_id, span_id, message, fields::text AS fields"
	if err := qdb.Select(selectCols).
		Order(order).
		Offset(q.Pagination.Offset).
		Limit(q.Pagination.Limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}

	// Convert to SearchHit
	hits := make([]search.SearchHit, 0, len(rows))
	for _, r := range rows {
		hit := search.SearchHit{
			ID:        r.ID,
			Type:      "log",
			Timestamp: r.Timestamp,
			Level:     r.Level,
			Message:   r.Message,
			Fields:    map[string]any{},
		}
		// Highlight: simple substring match for keywords
		if len(q.Keywords) > 0 {
			hit.Highlight = highlightFields(r.Message, q.Keywords)
		}
		hits = append(hits, hit)
	}

	// Facets: level distribution
	facets := make(map[string]search.Facet)
	if len(q.Filters) > 0 || len(q.Keywords) > 0 || !q.TimeRange.Start.IsZero() {
		// Rebuild filtered query for facets
		fdb := a.db.WithContext(ctx).Table("logs").
			Where("project_id = ?", q.ProjectID)
		if !q.TimeRange.Start.IsZero() {
			fdb = fdb.Where("timestamp >= ?", q.TimeRange.Start)
		}
		if !q.TimeRange.End.IsZero() {
			fdb = fdb.Where("timestamp <= ?", q.TimeRange.End)
		}
		for _, f := range q.Filters {
			fdb = applyFilter(fdb, f)
		}
		for _, kw := range q.Keywords {
			fdb = fdb.Where("message ILIKE ?", "%"+kw+"%")
		}

		type levelBucket struct {
			Level string
			Count int64
		}
		var buckets []levelBucket
		if err := fdb.Select("level, COUNT(*) as count").
			Group("level").
			Find(&buckets).Error; err == nil {
			fb := make([]search.FacetBucket, 0, len(buckets))
			for _, b := range buckets {
				fb = append(fb, search.FacetBucket{Key: b.Level, Count: b.Count})
			}
			facets["level"] = search.Facet{Field: "level", Buckets: fb}
		}
	}

	return &search.SearchResult{
		Total:  total,
		Hits:   hits,
		Facets: facets,
	}, nil
}

// applyFilter translates a Filter to a GORM where clause.
func applyFilter(qdb *gorm.DB, f search.Filter) *gorm.DB {
	col := f.Field
	// Map common field names to DB columns
	col = mapColumn(col)

	switch strings.ToLower(f.Operator) {
	case "eq", "":
		return qdb.Where(col+" = ?", f.Value)
	case "neq":
		return qdb.Where(col+" != ?", f.Value)
	case "contains":
		return qdb.Where(col+" ILIKE ?", "%"+fmt.Sprint(f.Value)+"%")
	case "in":
		return qdb.Where(col+" IN ?", f.Value)
	case "exists":
		return qdb.Where(col+" IS NOT NULL AND " + col + " != ''")
	case "not_exists":
		return qdb.Where(col + " IS NULL OR " + col + " = ''")
	default:
		return qdb.Where(col+" = ?", f.Value)
	}
}

// mapColumn maps DSL field names to actual DB columns.
func mapColumn(field string) string {
	m := map[string]string{
		"level":       "level",
		"trace_id":    "trace_id",
		"traceid":     "trace_id",
		"span_id":     "span_id",
		"spanid":      "span_id",
		"message":     "message",
		"environment": "fields->>'environment'",
		"service":     "fields->>'service'",
		"tag":         "fields->>'tag'",
	}
	if mapped, ok := m[strings.ToLower(field)]; ok {
		return mapped
	}
	// Default: try as a direct column name
	return field
}

// highlightFields returns simple highlight snippets for keywords found in message.
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
			snippet := message[start:end]
			snippets = append(snippets, snippet)
		}
	}
	if len(snippets) == 0 {
		return nil
	}
	return map[string][]string{
		"message": snippets,
	}
}
