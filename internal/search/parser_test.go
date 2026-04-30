package search

import (
	"testing"
)

func TestParseEmpty(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Filters) != 0 || len(result.Keywords) != 0 {
		t.Fatalf("expected empty, got filters=%v keywords=%v", result.Filters, result.Keywords)
	}
}

func TestParseKeyValues(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse("level:error tag:api")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(result.Filters))
	}
	if result.Filters[0].Field != "level" || result.Filters[0].Value != "error" {
		t.Errorf("first filter: got %+v", result.Filters[0])
	}
	if result.Filters[1].Field != "tag" || result.Filters[1].Value != "api" {
		t.Errorf("second filter: got %+v", result.Filters[1])
	}
}

func TestParseKeywords(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse("timeout connection_reset")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d: %v", len(result.Keywords), result.Keywords)
	}
	if result.Keywords[0] != "timeout" || result.Keywords[1] != "connection_reset" {
		t.Errorf("got keywords %v", result.Keywords)
	}
}

func TestParseQuotedValue(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse(`level:error message:"connection timeout"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(result.Filters))
	}
	if result.Filters[1].Value != "connection timeout" {
		t.Errorf("expected 'connection timeout', got '%s'", result.Filters[1].Value)
	}
}

func TestParseMixed(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse(`level:error timeout tag:"user service" connection_reset`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(result.Filters))
	}
	if len(result.Keywords) != 2 {
		t.Fatalf("expected 2 keywords, got %d", len(result.Keywords))
	}
}

func TestParseWhitespace(t *testing.T) {
	p := NewQueryParser()
	result, err := p.Parse("  level:error   timeout  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(result.Filters))
	}
	if len(result.Keywords) != 1 {
		t.Fatalf("expected 1 keyword, got %d", len(result.Keywords))
	}
}
