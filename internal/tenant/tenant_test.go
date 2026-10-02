package tenant

import (
	"context"
	"testing"
)

func TestWithAndFrom(t *testing.T) {
	ctx := context.Background()
	if _, ok := From(ctx); ok {
		t.Fatalf("expected no tenant in bare context")
	}

	custom := ID("11111111-1111-1111-1111-111111111111")
	got, ok := From(With(ctx, custom))
	if !ok || got != custom {
		t.Fatalf("expected %q, got %q ok=%v", custom, got, ok)
	}
}

func TestWithEmptyFallsBackToDefault(t *testing.T) {
	ctx := With(context.Background(), "")
	if id, ok := From(ctx); !ok || id != DefaultTenantID {
		t.Fatalf("expected empty id to fall back to %q, got %q ok=%v", DefaultTenantID, id, ok)
	}
	if got := FromOrDefault(context.Background()); got != DefaultTenantID {
		t.Fatalf("expected FromOrDefault on bare context to be %q, got %q", DefaultTenantID, got)
	}
	if got := FromOrDefault(With(context.Background(), "")); got != DefaultTenantID {
		t.Fatalf("expected FromOrDefault on empty id to be %q, got %q", DefaultTenantID, got)
	}
}

func TestMustFromPanicsWithoutTenant(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("expected MustFrom to panic without tenant")
		}
	}()
	MustFrom(context.Background())
}

func TestMustFromReturnsTenant(t *testing.T) {
	custom := ID("22222222-2222-2222-2222-222222222222")
	if got := MustFrom(With(context.Background(), custom)); got != custom {
		t.Fatalf("expected %q, got %q", custom, got)
	}
}
