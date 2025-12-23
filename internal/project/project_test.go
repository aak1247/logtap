package project

import "testing"

func TestParseID(t *testing.T) {
	t.Parallel()

	if id, err := ParseID("1"); err != nil || id != 1 {
		t.Fatalf("ParseID: id=%d err=%v", id, err)
	}
	if _, err := ParseID("0"); err == nil {
		t.Fatalf("expected error for 0")
	}
	if _, err := ParseID("-1"); err == nil {
		t.Fatalf("expected error for negative")
	}
	if _, err := ParseID("abc"); err == nil {
		t.Fatalf("expected error for non-numeric")
	}
}

