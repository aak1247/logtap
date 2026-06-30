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
	Total  int64            `json:"total"`
	Hits   []SearchHit      `json:"hits"`
	Facets map[string]Facet `json:"facets,omitempty"`
}

type SearchHit struct {
	ID        any                 `json:"id"`
	Type      string              `json:"type"` // "log" | "event" | "error"
	Timestamp time.Time           `json:"timestamp"`
	Level     string              `json:"level,omitempty"`
	Message   string              `json:"message"`
	Fields    map[string]any      `json:"fields,omitempty"`
	Highlight map[string][]string `json:"highlight,omitempty"`
}

type Facet struct {
	Field   string        `json:"field"`
	Buckets []FacetBucket `json:"buckets"`
}

type FacetBucket struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
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
