package clickhouse

import (
	"context"
	"errors"
	"expvar"
	"sync"
	"time"
)

// MessageCtl abstracts the NSQ acknowledgement controls the batcher needs.
// The CH consumer adapts *nsq.Message to this interface; tests use fakes.
type MessageCtl interface {
	Finish()
	Requeue(delay time.Duration)
	Touch()
}

// Item is one queued message traveling through the batcher.
type Item[T any] struct {
	// Ctl acknowledges the message: Finish after a durable flush, Requeue
	// after a failure. The batcher owns it from Enqueue to flush outcome.
	Ctl MessageCtl
	// Row is the backend row (model.Log, model.Event, ...).
	Row T
	// TenantID is the message tenant (open-source: DefaultTenantID).
	TenantID string
	// DedupKey is the Redis dedup key recorded after a successful flush
	// (design §4.7); empty disables recording.
	DedupKey string
	// ClientIP travels with the item for the sidecar geo metrics.
	ClientIP string
	// EnqueuedAt drives Touch() lease renewal for long-queued items.
	EnqueuedAt time.Time
	// Attempts is the NSQ delivery count, driving requeue backoff.
	Attempts int
}

// BatcherOptions configures one Batcher (defaults follow design §5.3).
type BatcherOptions struct {
	// BatchSize is the row count that triggers an immediate flush.
	BatchSize int
	// FlushInterval flushes partial batches when it elapses.
	FlushInterval time.Duration
	// FlushTimeout bounds one flush; must stay below the NSQ MsgTimeout.
	FlushTimeout time.Duration
	// DrainTimeout bounds Stop(): leftovers are Requeue(0)ed after it.
	DrainTimeout time.Duration
	// QueueSize is the incoming channel capacity (first-level buffer).
	QueueSize int
	// Shards is the number of flusher workers (1-2 recommended to start).
	Shards int
	// TouchInterval / TouchAfter control lease renewal (5s / 15s).
	TouchInterval time.Duration
	TouchAfter    time.Duration
}

func (o BatcherOptions) withDefaults() BatcherOptions {
	if o.BatchSize <= 0 {
		o.BatchSize = 5000
	}
	if o.FlushInterval <= 0 {
		o.FlushInterval = 500 * time.Millisecond
	}
	if o.FlushTimeout <= 0 {
		o.FlushTimeout = 10 * time.Second
	}
	if o.DrainTimeout <= 0 {
		o.DrainTimeout = 20 * time.Second
	}
	if o.QueueSize <= 0 {
		o.QueueSize = 100000
	}
	if o.Shards <= 0 {
		o.Shards = 1
	}
	if o.TouchInterval <= 0 {
		o.TouchInterval = 5 * time.Second
	}
	if o.TouchAfter <= 0 {
		o.TouchAfter = 15 * time.Second
	}
	return o
}

// metricsMapName is the shared expvar map for ClickHouse ingestion counters
// (batcher + consumer). Keys follow the design doc metric names
// (logtap_consumer.nsq_requeue_total, logtap_consumer.dedup_dropped_total, ...).
const metricsMapName = "logtap_consumer"

// Metrics returns the shared logtap_consumer expvar map. Both this package
// and the CH consumer add counters to it.
func Metrics() *expvar.Map {
	if v := expvar.Get(metricsMapName); v != nil {
		if m, ok := v.(*expvar.Map); ok {
			return m
		}
	}
	return expvar.NewMap(metricsMapName)
}

var chMetrics = Metrics()

// ErrStopped is returned by Enqueue after Stop began.
var ErrStopped = errors.New("clickhouse batcher stopped")

// Batcher asynchronously aggregates items from NSQ handlers into batches
// large enough for ClickHouse (design §5.2.2). Unlike the synchronous PG
// batcher, Enqueue never waits for a flush: handlers return immediately, and
// the batcher takes over acknowledgement (Finish/Requeue/Touch) until the
// batch is durable. A crash therefore loses nothing: unacknowledged messages
// are redelivered by NSQ after MsgTimeout.
type Batcher[T any] struct {
	opts  BatcherOptions
	flush func(ctx context.Context, items []Item[T]) error
	after func(items []Item[T])

	incoming chan Item[T]
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup

	mu       sync.Mutex
	cond     *sync.Cond
	active   []Item[T]         // waiting to reach BatchSize
	inflight [][]Item[T]       // cut, waiting for a free flusher
	flushing map[int][]Item[T] // handed to a flusher, flush in progress
	batchSeq int
	draining bool // Stop began: no more Enqueue
	aggDone  bool // aggregator exited: no more batches will be cut
	abort    bool // drain watchdog fired: drop queued batches via Requeue(0)

	drainIdle chan struct{} // closed once draining finished with nothing pending
	drainOnce sync.Once
}

// NewBatcher starts the aggregator, Shards flushers, the toucher and the
// drain watchdog. flush must be safe for concurrent use by Shards goroutines.
// after, when non-nil, runs off the flush path after each successful flush
// (Redis dedup records, sidecar metrics, alert evaluation).
func NewBatcher[T any](opts BatcherOptions, flush func(ctx context.Context, items []Item[T]) error, after func(items []Item[T])) *Batcher[T] {
	opts = opts.withDefaults()
	b := &Batcher[T]{
		opts:      opts,
		flush:     flush,
		after:     after,
		incoming:  make(chan Item[T], opts.QueueSize),
		stopCh:    make(chan struct{}),
		flushing:  make(map[int][]Item[T]),
		drainIdle: make(chan struct{}),
	}
	b.cond = sync.NewCond(&b.mu)
	b.wg.Add(1 + opts.Shards + 2)
	go b.aggregateLoop()
	for i := 0; i < opts.Shards; i++ {
		go b.flushLoop()
	}
	go b.touchLoop()
	go b.drainWatchdog()
	return b
}

// Enqueue hands one item to the batcher. It blocks while the incoming buffer
// is full — that is the backpressure path: blocked NSQ handlers stop the
// consumer from pulling, nsqd buffers on disk. After Stop began it returns
// ErrStopped and the caller must Requeue the message itself.
func (b *Batcher[T]) Enqueue(item Item[T]) error {
	b.mu.Lock()
	draining := b.draining
	b.mu.Unlock()
	if draining {
		return ErrStopped
	}
	select {
	case b.incoming <- item:
		return nil
	case <-b.stopCh:
		return ErrStopped
	}
}

// Stop drains pending items (design §5.2.2 drain protocol): everything
// already buffered gets flushed within DrainTimeout; whatever did not make
// it is Requeue(0)ed back to NSQ. It blocks until all loops exited.
func (b *Batcher[T]) Stop() {
	b.stopOnce.Do(func() {
		b.mu.Lock()
		b.draining = true
		b.mu.Unlock()
		close(b.stopCh)
		b.cond.Broadcast()
	})
	b.wg.Wait()
}

// aggregateLoop moves items from the incoming channel into the active
// buffer, cutting a batch as soon as it reaches BatchSize or on the flush
// ticker. After stop it drains whatever is still buffered, then exits.
func (b *Batcher[T]) aggregateLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.opts.FlushInterval)
	defer ticker.Stop()
	for {
		select {
		case it := <-b.incoming:
			if b.appendActive(it) {
				b.cutBatch()
			}
		case <-ticker.C:
			b.cutBatch()
		case <-b.stopCh:
			for {
				select {
				case it := <-b.incoming:
					if b.appendActive(it) {
						b.cutBatch()
					}
				default:
					b.cutBatch()
					b.mu.Lock()
					b.aggDone = true
					b.markDrainIdleLocked()
					b.mu.Unlock()
					b.cond.Broadcast()
					return
				}
			}
		}
	}
}

// appendActive reports whether the active buffer reached BatchSize.
func (b *Batcher[T]) appendActive(it Item[T]) bool {
	b.mu.Lock()
	b.active = append(b.active, it)
	full := len(b.active) >= b.opts.BatchSize
	b.mu.Unlock()
	return full
}

// cutBatch moves the active buffer to the inflight queue and wakes a flusher.
func (b *Batcher[T]) cutBatch() {
	b.mu.Lock()
	if len(b.active) == 0 {
		b.mu.Unlock()
		return
	}
	batch := b.active
	b.active = nil
	b.inflight = append(b.inflight, batch)
	b.mu.Unlock()
	b.cond.Broadcast()
}

// flushLoop takes batches from the inflight queue, writes them to
// ClickHouse, then acknowledges or requeues every item in the batch.
func (b *Batcher[T]) flushLoop() {
	defer b.wg.Done()
	for {
		b.mu.Lock()
		// Keep working until the aggregator finished AND nothing is queued;
		// a stop while the aggregator is still draining must not strand the
		// batches it is about to cut.
		for len(b.inflight) == 0 && !b.aggDone && !b.abort {
			b.cond.Wait()
		}
		if (len(b.inflight) == 0 && b.aggDone) || b.abort {
			b.mu.Unlock()
			return
		}
		batch := b.inflight[0]
		b.inflight = b.inflight[1:]
		b.batchSeq++
		id := b.batchSeq
		b.flushing[id] = batch
		b.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), b.opts.FlushTimeout)
		err := b.flush(ctx, batch)
		cancel()

		b.mu.Lock()
		delete(b.flushing, id)
		aborted := b.abort || (b.aggDone && len(b.inflight) == 0)
		b.markDrainIdleLocked()
		b.mu.Unlock()

		chMetrics.Add("flush_total", 1)
		if err != nil {
			chMetrics.Add("flush_errors", 1)
			for i := range batch {
				chMetrics.Add("nsq_requeue_total", 1)
				batch[i].Ctl.Requeue(requeueDelay(batch[i].Attempts))
			}
			if aborted {
				return
			}
			continue
		}
		chMetrics.Add("flushed_rows_total", int64(len(batch)))
		for i := range batch {
			batch[i].Ctl.Finish()
		}
		if b.after != nil {
			items := make([]Item[T], len(batch))
			copy(items, batch)
			go b.after(items)
		}
		if aborted {
			return
		}
	}
}

// markDrainIdleLocked closes the drainIdle signal once draining started and
// nothing is pending anymore. The caller must hold b.mu. The drain watchdog
// listens on it so a clean early drain does not have to wait out the whole
// DrainTimeout before Stop() can return.
func (b *Batcher[T]) markDrainIdleLocked() {
	if !b.draining || !b.aggDone || b.abort {
		return
	}
	if len(b.active) == 0 && len(b.inflight) == 0 && len(b.flushing) == 0 {
		b.drainOnce.Do(func() { close(b.drainIdle) })
	}
}

// touchLoop renews the NSQ lease of items that queue longer than TouchAfter
// so that slow flushes never trigger MsgTimeout redelivery (design §5.2.2).
func (b *Batcher[T]) touchLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.opts.TouchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			deadline := time.Now().Add(-b.opts.TouchAfter)
			b.mu.Lock()
			queued := len(b.active)
			for _, batch := range b.inflight {
				queued += len(batch)
			}
			for _, batch := range b.flushing {
				queued += len(batch)
			}
			// Collect the lease renewals first and run them after releasing
			// the lock: Touch() performs network I/O on the NSQ connection
			// and must never extend the batcher's critical sections.
			var ctls []MessageCtl
			for i := range b.active {
				if b.active[i].EnqueuedAt.Before(deadline) {
					ctls = append(ctls, b.active[i].Ctl)
				}
			}
			for _, batch := range b.inflight {
				for i := range batch {
					if batch[i].EnqueuedAt.Before(deadline) {
						ctls = append(ctls, batch[i].Ctl)
					}
				}
			}
			for _, batch := range b.flushing {
				for i := range batch {
					if batch[i].EnqueuedAt.Before(deadline) {
						ctls = append(ctls, batch[i].Ctl)
					}
				}
			}
			chMetrics.Set("queued_items", func() expvar.Var { i := new(expvar.Int); i.Set(int64(queued)); return i }())
			b.mu.Unlock()
			for _, ctl := range ctls {
				ctl.Touch()
			}
		case <-b.stopCh:
			return
		}
	}
}

// drainWatchdog bounds Stop(): once Stop began it gives the flushers
// DrainTimeout to catch up; whatever is still queued then is Requeue(0)ed
// back to NSQ so nothing stays parked in memory. A clean early drain closes
// drainIdle and the watchdog returns immediately instead of waiting out the
// whole timeout.
func (b *Batcher[T]) drainWatchdog() {
	defer b.wg.Done()
	select {
	case <-b.stopCh:
	}
	timer := time.NewTimer(b.opts.DrainTimeout)
	defer timer.Stop()
	select {
	case <-b.drainIdle:
		return
	case <-timer.C:
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.draining {
		return
	}
	for _, batch := range b.inflight {
		for i := range batch {
			chMetrics.Add("nsq_requeue_total", 1)
			batch[i].Ctl.Requeue(0)
		}
	}
	b.inflight = nil
	b.abort = true
	b.aggDone = true
	b.cond.Broadcast()
}

// requeueDelay grows exponentially with the NSQ delivery count, capped at 30s
// (well below the point where it would interact with redelivery windows).
func requeueDelay(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 7 {
		return 30 * time.Second
	}
	d := 500 * time.Millisecond
	for i := 1; i < attempts; i++ {
		d *= 2
	}
	if d > 30*time.Second {
		return 30 * time.Second
	}
	return d
}
