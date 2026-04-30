package detector

import (
	"testing"
)

func TestAggregateToPGInterval(t *testing.T) {
	tests := []struct {
		input AggregateInterval
		want  string
	}{
		{IntervalMinute, "1 minute"},
		{IntervalHour, "1 hour"},
		{IntervalDay, "1 day"},
		{AggregateInterval("unknown"), "1 hour"},
	}
	for _, tt := range tests {
		got := aggregateToPGInterval(tt.input)
		if got != tt.want {
			t.Errorf("aggregateToPGInterval(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResultStore_NilDB(t *testing.T) {
	s := NewResultStore(nil)
	if err := s.Store(nil, nil); err != nil {
		t.Errorf("Store with nil db should be nil, got %v", err)
	}
	results, err := s.Query(nil, "test", ResultQuery{})
	if err != nil {
		t.Errorf("Query with nil db should be nil, got %v", err)
	}
	if results != nil {
		t.Errorf("Query with nil db should return nil results")
	}
}
