package alert

import "testing"

func TestGetByPath_ArrayIndex(t *testing.T) {
	m := map[string]any{
		"exception": map[string]any{
			"values": []any{
				map[string]any{"type": "TypeError", "value": "boom"},
			},
		},
	}

	v, ok := getByPath(m, "exception.values.0.type")
	if !ok || v != "TypeError" {
		t.Fatalf("getByPath array: ok=%v v=%v", ok, v)
	}
	if _, ok := getByPath(m, "exception.values.1.type"); ok {
		t.Fatalf("expected missing index to be ok=false")
	}
}

func TestMatchRule_EventNameAndFields(t *testing.T) {
	in := Input{
		Source:  SourceLogs,
		Level:   "event",
		Message: "signup",
		Fields: map[string]any{
			"user": map[string]any{"id": "u1"},
		},
	}

	m := RuleMatch{
		EventNames: []string{"signup"},
		FieldsAll: []FieldMatch{
			{Path: "user.id", Op: OpEquals, Value: "u1"},
		},
	}
	if !matchRule(m, in) {
		t.Fatalf("expected match")
	}
}
