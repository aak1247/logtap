package search

import "context"

// SearchEngine is the unified search entry point. It parses raw query strings,
// delegates to a SearchAdapter, and returns structured results.
type SearchEngine struct {
	adapter SearchAdapter
	parser  QueryParser
}

// NewEngine creates a SearchEngine with the given adapter and built-in parser.
func NewEngine(adapter SearchAdapter) *SearchEngine {
	return &SearchEngine{
		adapter: adapter,
		parser:  NewQueryParser(),
	}
}

// NewEngineWithParser creates a SearchEngine with a custom parser.
func NewEngineWithParser(adapter SearchAdapter, parser QueryParser) *SearchEngine {
	return &SearchEngine{
		adapter: adapter,
		parser:  parser,
	}
}

// Search is the unified search entry point. It parses rawQuery, builds a
// SearchQuery, and delegates to the adapter.
func (e *SearchEngine) Search(ctx context.Context, rawQuery string, projectID int, timeRange TimeRange, page, pageSize int) (*SearchResult, error) {
	parsed, err := e.parser.Parse(rawQuery)
	if err != nil {
		return nil, err
	}

	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 500 {
		pageSize = 500
	}
	if page <= 0 {
		page = 1
	}

	sq := SearchQuery{
		ProjectID: projectID,
		TimeRange: timeRange,
		Filters:   parsed.Filters,
		Keywords:  parsed.Keywords,
		Sort: SortSpec{
			Field: "timestamp",
			Order: "desc",
		},
		Pagination: Pagination{
			Offset: (page - 1) * pageSize,
			Limit:  pageSize,
		},
		RawQuery: rawQuery,
	}

	return e.adapter.Search(ctx, sq)
}

// Index delegates to the adapter's Index method.
func (e *SearchEngine) Index(ctx context.Context, projectID int, hits []SearchHit) error {
	return e.adapter.Index(ctx, projectID, hits)
}
