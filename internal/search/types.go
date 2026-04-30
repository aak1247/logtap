package search

import (
	"context"
	"time"
)

// SearchQuery is the universal query structure passed to adapters.
type SearchQuery struct {
	ProjectID  int
	TimeRange  TimeRange
	Filters    []Filter
	Keywords   []string
	Sort       SortSpec
	Pagination Pagination
	RawQuery   string
}

type Filter struct {
	Field    string
	Operator string // eq, neq, contains, in, exists, not_exists
	Value    any
}

type TimeRange struct {
	Start time.Time
	End   time.Time
}

type SortSpec struct {
	Field string
	Order string // asc, desc
}

type Pagination struct {
	Offset int
	Limit  int
}

type SearchResult struct {
	Total  int64
	Hits   []SearchHit
	Facets map[string]Facet
}

type SearchHit struct {
	ID        any
	Type      string // "log" | "event" | "error"
	Timestamp time.Time
	Level     string
	Message   string
	Fields    map[string]any
	Highlight map[string][]string
}

type Facet struct {
	Field   string
	Buckets []FacetBucket
}

type FacetBucket struct {
	Key   string
	Count int64
}

// SearchAdapter is the core adapter interface. Implementations translate
// SearchQuery into backend-specific queries (Postgres, ClickHouse, Elastic, …).
type SearchAdapter interface {
	Type() string

	// Index writes search hits to the backend.
	Index(ctx context.Context, projectID int, hits []SearchHit) error

	// Search executes a query and returns matching hits with optional facets.
	Search(ctx context.Context, query SearchQuery) (*SearchResult, error)

	// DeleteBefore removes records older than the given time.
	DeleteBefore(ctx context.Context, projectID int, before time.Time) error

	// Ping checks backend health.
	Ping(ctx context.Context) error
}
