package alert

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

const (
	asyncEvaluatorBuffer  = 2048
	asyncEvaluatorWorkers = 4
	asyncEvalTimeout      = 10 * time.Second
)

// AsyncEvaluator decouples alert evaluation from the storage flush path.
// Inputs are queued and processed by a small worker pool; when the queue is
// full the newest inputs are dropped (alerts are best-effort) instead of
// blocking ingestion. Occurrence state lives in alert_states, so dropped
// inputs only delay threshold/backoff decisions until traffic recovers.
type AsyncEvaluator struct {
	eng    *Engine
	ch     chan Input
	stopCh chan struct{}
	done   sync.WaitGroup

	dropped   atomic.Int64
	errLogged atomic.Int64
}

func NewAsyncEvaluator(eng *Engine) *AsyncEvaluator {
	a := &AsyncEvaluator{
		eng:    eng,
		ch:     make(chan Input, asyncEvaluatorBuffer),
		stopCh: make(chan struct{}),
	}
	for i := 0; i < asyncEvaluatorWorkers; i++ {
		a.done.Add(1)
		go a.run()
	}
	return a
}

// Submit queues an input for evaluation without blocking the caller.
func (a *AsyncEvaluator) Submit(in Input) {
	select {
	case a.ch <- in:
	default:
		n := a.dropped.Add(1)
		engineAsyncDroppedTotal.Add(1)
		if n == 1 || n%1000 == 0 {
			log.Printf("alert evaluation queue full; dropped=%d so far", n)
		}
	}
}

// Stop stops the workers after draining whatever is already queued.
func (a *AsyncEvaluator) Stop() {
	close(a.stopCh)
	a.done.Wait()
}

func (a *AsyncEvaluator) run() {
	defer a.done.Done()
	for {
		select {
		case <-a.stopCh:
			a.drain()
			return
		case in := <-a.ch:
			a.evaluateOne(in)
		}
	}
}

func (a *AsyncEvaluator) drain() {
	for {
		select {
		case in := <-a.ch:
			a.evaluateOne(in)
		default:
			return
		}
	}
}

func (a *AsyncEvaluator) evaluateOne(in Input) {
	ctx, cancel := context.WithTimeout(context.Background(), asyncEvalTimeout)
	defer cancel()
	if err := a.eng.Evaluate(ctx, in); err != nil {
		n := a.errLogged.Add(1)
		engineAsyncErrorTotal.Add(1)
		if n == 1 || n%100 == 0 {
			log.Printf("alert async evaluate error (count=%d): %v", n, err)
		}
	}
}
