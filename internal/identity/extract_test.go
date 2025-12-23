package identity

import (
	"testing"
	"time"
)

func TestExtractUserID(t *testing.T) {
	t.Parallel()

	if got := ExtractUserID(map[string]any{"user": map[string]any{"id": "  u1  "}}); got != "u1" {
		t.Fatalf("expected u1, got %q", got)
	}
	if got := ExtractUserID(map[string]any{"user": map[string]any{"username": "  alice  "}}); got != "alice" {
		t.Fatalf("expected alice, got %q", got)
	}
	if got := ExtractUserID(map[string]any{"user": map[string]any{"email": "  a@b  "}}); got != "a@b" {
		t.Fatalf("expected a@b, got %q", got)
	}
	if got := ExtractUserID(map[string]any{"user": "not-a-map"}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractDeviceID_AndDistinctID(t *testing.T) {
	t.Parallel()

	// Prefer tags.device_id
	ev := map[string]any{
		"tags": map[string]any{"device_id": " d1 "},
		"contexts": map[string]any{
			"device": map[string]any{"id": "ctx"},
		},
	}
	if got := ExtractDeviceID(ev); got != "d1" {
		t.Fatalf("expected d1, got %q", got)
	}
	if got, src := ExtractDistinctID(map[string]any{"user": map[string]any{"id": "u1"}, "tags": map[string]any{"device_id": "d1"}}); got != "u1" || src != "user.id" {
		t.Fatalf("expected distinct u1 from user.id, got %q/%q", got, src)
	}
	if got, src := ExtractDistinctID(map[string]any{"tags": map[string]any{"device_id": "d1"}}); got != "d1" || src != "device_id" {
		t.Fatalf("expected distinct d1 from device_id, got %q/%q", got, src)
	}

	// tags as array of pairs, with non-string value.
	ev2 := map[string]any{
		"tags": []any{[]any{"device_id", 123}},
	}
	if got := ExtractDeviceID(ev2); got != "123" {
		t.Fatalf("expected 123, got %q", got)
	}

	// contexts.device.id fallback.
	ev3 := map[string]any{
		"contexts": map[string]any{"device": map[string]any{"id": " ctx1 "}},
	}
	if got := ExtractDeviceID(ev3); got != "ctx1" {
		t.Fatalf("expected ctx1, got %q", got)
	}

	// extra.device_id fallback.
	ev4 := map[string]any{
		"extra": map[string]any{"device_id": " ex1 "},
	}
	if got := ExtractDeviceID(ev4); got != "ex1" {
		t.Fatalf("expected ex1, got %q", got)
	}
}

func TestExtractOS_AndBrowser(t *testing.T) {
	t.Parallel()

	ev := map[string]any{
		"contexts": map[string]any{
			"os":      map[string]any{"name": "iOS", "version": "17"},
			"browser": map[string]any{"name": "Chrome", "version": "120"},
		},
	}
	if got := ExtractOS(ev); got != "iOS 17" {
		t.Fatalf("expected iOS 17, got %q", got)
	}
	if got := ExtractBrowser(ev); got != "Chrome 120" {
		t.Fatalf("expected Chrome 120, got %q", got)
	}

	ev2 := map[string]any{"contexts": map[string]any{"os": map[string]any{"name": " Android "}}}
	if got := ExtractOS(ev2); got != "Android" {
		t.Fatalf("expected Android, got %q", got)
	}
	if got := ExtractBrowser(map[string]any{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestExtractTimestamp(t *testing.T) {
	t.Parallel()

	fallback := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t1 := "2025-01-01T00:00:00Z"
	if got := ExtractTimestamp(map[string]any{"timestamp": t1}, time.Time{}); !got.Equal(fallback) {
		t.Fatalf("expected %v, got %v", fallback, got)
	}

	t2 := "2025-01-01T00:00:00.123Z"
	got2 := ExtractTimestamp(map[string]any{"timestamp": t2}, fallback)
	if got2.Unix() != 1735689600 {
		t.Fatalf("expected unix=1735689600, got %d", got2.Unix())
	}
	if got2.Nanosecond() == 0 {
		t.Fatalf("expected non-zero nanos")
	}

	got3 := ExtractTimestamp(map[string]any{"timestamp": float64(1735689600.5)}, fallback)
	if got3.Unix() != 1735689600 || got3.Nanosecond() == 0 {
		t.Fatalf("expected fractional timestamp, got %v", got3)
	}

	got4 := ExtractTimestamp(map[string]any{"timestamp": int64(1735689600)}, fallback)
	if got4.Unix() != 1735689600 {
		t.Fatalf("expected unix=1735689600, got %d", got4.Unix())
	}

	got5 := ExtractTimestamp(map[string]any{"timestamp": "bad"}, fallback)
	if !got5.Equal(fallback) {
		t.Fatalf("expected fallback, got %v", got5)
	}
}

