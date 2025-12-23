package ingest

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestParseEnvelope_EmptyAndHeaderValidation(t *testing.T) {
	t.Parallel()

	if _, err := ParseEnvelope(nil); err == nil {
		t.Fatalf("expected error for empty body")
	}

	if _, err := ParseEnvelope([]byte("\n")); err == nil {
		t.Fatalf("expected error for empty header line")
	}
}

func TestParseEnvelope_NoItems(t *testing.T) {
	t.Parallel()

	if _, err := ParseEnvelope([]byte("{}\n")); err == nil {
		t.Fatalf("expected error for no items")
	}
}

func TestParseEnvelope_ItemFallbackLinePayload_AndFirstEventJSON(t *testing.T) {
	t.Parallel()

	body := []byte(
		"{\"event_id\":\"e1\"}\n" +
			"{\"type\":\"event\"}\n" +
			"{\"event_id\":\"e1\",\"message\":\"boom\"}\n",
	)
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if len(env.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(env.Items))
	}
	ev, ok := env.FirstEventJSON()
	if !ok {
		t.Fatalf("expected FirstEventJSON ok")
	}
	if ev["message"] != "boom" {
		t.Fatalf("unexpected event: %v", ev)
	}
}

func TestParseEnvelope_ItemLengthPayload_AndTrailingNewlines(t *testing.T) {
	t.Parallel()

	payload := []byte("{\"event_id\":\"e1\",\"message\":\"ok\"}")
	body2 := []byte("{}\n{\"type\":\"event\",\"length\":")
	body2 = append(body2, []byte(strconv.Itoa(len(payload))+"}\n")...)
	body2 = append(body2, payload...)
	body2 = append(body2, []byte("\n\n")...)
	body2 = append(body2, []byte("{\"type\":\"event\"}\n{\"event_id\":\"e2\"}\n")...)

	env, err := ParseEnvelope(body2)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if len(env.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(env.Items))
	}
}

func TestEnvelope_FirstEventJSON_SkipsNonEventOrBadJSON(t *testing.T) {
	t.Parallel()

	body := []byte(
		"{}\n" +
			"{\"type\":\"attachment\"}\n" +
			"not-json\n" +
			"{\"type\":\"event\"}\n" +
			"not-json\n",
	)
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if _, ok := env.FirstEventJSON(); ok {
		t.Fatalf("expected no valid event json")
	}
}

func TestReadItemPayload_LengthValidation(t *testing.T) {
	t.Parallel()

	// Unsupported length type.
	if _, err := ParseEnvelope([]byte("{}\n{\"type\":\"event\",\"length\":\"bad\"}\n{}")); err == nil {
		t.Fatalf("expected error for invalid length type")
	}
	// Negative length.
	if _, err := ParseEnvelope([]byte("{}\n{\"type\":\"event\",\"length\":-1}\n{}")); err == nil {
		t.Fatalf("expected error for negative length")
	}
	// Length too large for body.
	if _, err := ParseEnvelope([]byte("{}\n{\"type\":\"event\",\"length\":10}\n{}")); err == nil {
		t.Fatalf("expected error for short read")
	}
}

func TestAsInt64(t *testing.T) {
	t.Parallel()

	if n, err := asInt64(float64(3)); err != nil || n != 3 {
		t.Fatalf("float64: n=%d err=%v", n, err)
	}
	if n, err := asInt64(int64(3)); err != nil || n != 3 {
		t.Fatalf("int64: n=%d err=%v", n, err)
	}
	if n, err := asInt64(int(3)); err != nil || n != 3 {
		t.Fatalf("int: n=%d err=%v", n, err)
	}
	if n, err := asInt64(json.Number("3")); err != nil || n != 3 {
		t.Fatalf("json.Number: n=%d err=%v", n, err)
	}
	if _, err := asInt64("x"); err == nil {
		t.Fatalf("expected error for unsupported type")
	}
}
