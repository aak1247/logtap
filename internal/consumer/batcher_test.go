package consumer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBatcher_FlushOnMaxSize(t *testing.T) {
	t.Parallel()

	flushed := make(chan []int, 1)
	b := NewBatcher[int](2, time.Hour, time.Second, func(ctx context.Context, items []int) error {
		cp := append([]int(nil), items...)
		flushed <- cp
		return nil
	})
	t.Cleanup(b.Close)

	done1 := make(chan struct{})
	go func() {
		_ = b.Add(1)
		close(done1)
	}()

	select {
	case <-done1:
		t.Fatalf("Add returned before flush")
	case <-time.After(50 * time.Millisecond):
	}

	if err := b.Add(2); err != nil {
		t.Fatalf("Add(2): %v", err)
	}

	select {
	case <-done1:
	case <-time.After(time.Second):
		t.Fatalf("expected Add(1) to return after flush")
	}

	select {
	case got := <-flushed:
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("unexpected flushed items: %v", got)
		}
	default:
		t.Fatalf("expected flush to run")
	}
}

func TestBatcher_FlushOnInterval(t *testing.T) {
	t.Parallel()

	flushed := make(chan struct{}, 1)
	b := NewBatcher[int](10, 30*time.Millisecond, time.Second, func(ctx context.Context, items []int) error {
		flushed <- struct{}{}
		return nil
	})
	t.Cleanup(b.Close)

	start := time.Now()
	if err := b.Add(1); err != nil {
		t.Fatalf("Add: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond {
		t.Fatalf("expected Add to block until interval flush, elapsed=%s", elapsed)
	}

	select {
	case <-flushed:
	default:
		t.Fatalf("expected interval flush to run")
	}
}

func TestBatcher_FlushErrorPropagates(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	b := NewBatcher[int](1, time.Hour, time.Second, func(ctx context.Context, items []int) error {
		return want
	})
	t.Cleanup(b.Close)

	if err := b.Add(1); !errors.Is(err, want) {
		t.Fatalf("expected %v, got %v", want, err)
	}
}
