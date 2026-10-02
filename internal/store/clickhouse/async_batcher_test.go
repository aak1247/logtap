package clickhouse

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeCtl struct {
	finish    atomic.Int32
	requeue   atomic.Int32
	touch     atomic.Int32
	lastDelay atomic.Int64
}

func (f *fakeCtl) Finish()                 { f.finish.Add(1) }
func (f *fakeCtl) Requeue(d time.Duration) { f.requeue.Add(1); f.lastDelay.Store(int64(d)) }
func (f *fakeCtl) Touch()                  { f.touch.Add(1) }

type flushRecorder struct {
	mu    sync.Mutex
	sizes []int
	items []Item[int]
	calls int
	fail  atomic.Bool
}

func (r *flushRecorder) record(ctx context.Context, items []Item[int]) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.fail.Load() {
		return errors.New("injected flush failure")
	}
	r.sizes = append(r.sizes, len(items))
	r.items = append(r.items, items...)
	return nil
}

func (r *flushRecorder) flushedRows() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.sizes {
		n += s
	}
	return n
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

func newTestBatcher(t *testing.T, batchSize int, flush func(ctx context.Context, items []Item[int]) error, after func(items []Item[int])) *Batcher[int] {
	t.Helper()
	b := NewBatcher[int](BatcherOptions{
		BatchSize:     batchSize,
		FlushInterval: 30 * time.Millisecond,
		FlushTimeout:  time.Second,
		DrainTimeout:  time.Second,
		QueueSize:     100,
		Shards:        1,
		TouchInterval: 10 * time.Millisecond,
		TouchAfter:    40 * time.Millisecond,
	}, flush, after)
	return b
}

func makeItems(n int) []Item[int] {
	items := make([]Item[int], n)
	now := time.Now()
	for i := range items {
		items[i] = Item[int]{
			Ctl:        &fakeCtl{},
			Row:        i,
			TenantID:   "00000000-0000-0000-0000-000000000001",
			DedupKey:   "dedup:t:1:x",
			ClientIP:   "127.0.0.1",
			EnqueuedAt: now,
			Attempts:   1,
		}
	}
	return items
}

func TestBatcher_FlushesWhenBatchSizeReached(t *testing.T) {
	rec := &flushRecorder{}
	b := newTestBatcher(t, 5, rec.record, nil)
	items := makeItems(5)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return rec.flushedRows() == 5 }, "batch-size flush")
	ctl := items[0].Ctl.(*fakeCtl)
	if ctl.finish.Load() != 1 {
		t.Fatalf("expected item finished once, got %d", ctl.finish.Load())
	}
	if ctl.requeue.Load() != 0 {
		t.Fatalf("expected no requeues, got %d", ctl.requeue.Load())
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.sizes) != 1 || rec.sizes[0] != 5 {
		t.Fatalf("expected one batch of 5, got %v", rec.sizes)
	}
}

func TestBatcher_FlushesPartialBatchOnInterval(t *testing.T) {
	rec := &flushRecorder{}
	b := newTestBatcher(t, 100, rec.record, nil)
	items := makeItems(3)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return rec.flushedRows() == 3 }, "interval flush")
	b.Stop()
}

func TestBatcher_RequeuesOnFlushFailure(t *testing.T) {
	rec := &flushRecorder{}
	rec.fail.Store(true)
	b := newTestBatcher(t, 2, rec.record, nil)
	items := makeItems(2)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool {
		ctl := items[0].Ctl.(*fakeCtl)
		ctl2 := items[1].Ctl.(*fakeCtl)
		return ctl.requeue.Load() >= 1 && ctl2.requeue.Load() >= 1
	}, "requeue after flush failure")
	if items[0].Ctl.(*fakeCtl).finish.Load() != 0 {
		t.Fatalf("failed items must not be finished")
	}
	b.Stop()
}

func TestBatcher_StopDrainsPendingItems(t *testing.T) {
	rec := &flushRecorder{}
	b := newTestBatcher(t, 1000, rec.record, nil) // never reaches batch size
	items := makeItems(4)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	b.Stop() // must flush pending items within DrainTimeout
	waitFor(t, 2*time.Second, func() bool { return rec.flushedRows() == 4 }, "drain flush")
	for i := range items {
		if items[i].Ctl.(*fakeCtl).finish.Load() != 1 {
			t.Fatalf("drained item %d not finished", i)
		}
	}
}

func TestBatcher_StopRequeuesWhenFlushNeverSucceeds(t *testing.T) {
	rec := &flushRecorder{}
	rec.fail.Store(true)
	b := NewBatcher[int](BatcherOptions{
		BatchSize:     1000,
		FlushInterval: time.Hour,
		FlushTimeout:  30 * time.Millisecond,
		DrainTimeout:  120 * time.Millisecond,
		QueueSize:     100,
		Shards:        1,
		TouchInterval: 10 * time.Millisecond,
		TouchAfter:    time.Hour,
	}, rec.record, nil)
	items := makeItems(2)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	b.Stop()
	waitFor(t, 2*time.Second, func() bool {
		return items[0].Ctl.(*fakeCtl).requeue.Load() >= 1 && items[1].Ctl.(*fakeCtl).requeue.Load() >= 1
	}, "requeue when drain cannot flush")
	if rec.flushedRows() != 0 {
		t.Fatalf("nothing should have been flushed successfully")
	}
}

func TestBatcher_TouchesLongQueuedItems(t *testing.T) {
	// First batch parks inside the flusher (blocked flush), the second waits
	// in the inflight queue — both holding spots must get Touch() after
	// TouchAfter so NSQ never times them out.
	release := make(chan struct{})
	var mu sync.Mutex
	flushed := 0
	flush := func(ctx context.Context, items []Item[int]) error {
		<-release
		mu.Lock()
		flushed += len(items)
		mu.Unlock()
		return nil
	}
	b := NewBatcher[int](BatcherOptions{
		BatchSize:     2,
		FlushInterval: time.Hour,
		FlushTimeout:  5 * time.Second,
		DrainTimeout:  2 * time.Second,
		QueueSize:     100,
		Shards:        1,
		TouchInterval: 10 * time.Millisecond,
		TouchAfter:    40 * time.Millisecond,
	}, flush, nil)
	items := makeItems(4)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool {
		return items[0].Ctl.(*fakeCtl).touch.Load() >= 1 && // parked in the flusher
			items[2].Ctl.(*fakeCtl).touch.Load() >= 1 // waiting in inflight
	}, "touch of long-queued items in flushing and inflight")
	close(release)
	b.Stop()
	mu.Lock()
	defer mu.Unlock()
	if flushed != 4 {
		t.Fatalf("expected all 4 items flushed after release, got %d", flushed)
	}
}

func TestBatcher_AfterRunsOnSuccess(t *testing.T) {
	rec := &flushRecorder{}
	var afterRows atomic.Int32
	b := newTestBatcher(t, 2, rec.record, func(items []Item[int]) {
		afterRows.Add(int32(len(items)))
	})
	items := makeItems(2)
	for i := range items {
		if err := b.Enqueue(items[i]); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return afterRows.Load() == 2 }, "after callback")
}

func TestBatcher_RequeueDelayBacksOff(t *testing.T) {
	if requeueDelay(1) != 500*time.Millisecond {
		t.Fatalf("unexpected first-attempt delay")
	}
	if requeueDelay(2) != time.Second {
		t.Fatalf("unexpected second-attempt delay")
	}
	if requeueDelay(99) != 30*time.Second {
		t.Fatalf("expected cap at 30s")
	}
}

func TestBatcher_EnqueueAfterStopReturnsErr(t *testing.T) {
	b := newTestBatcher(t, 10, func(ctx context.Context, items []Item[int]) error { return nil }, nil)
	b.Stop()
	if err := b.Enqueue(Item[int]{}); !errors.Is(err, ErrStopped) {
		t.Fatalf("expected ErrStopped, got %v", err)
	}
}
