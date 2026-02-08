package obs

import (
	"encoding/json"
	"sync/atomic"
	"time"
)

type Stats struct {
	start time.Time

	httpRequests     atomic.Int64
	httpErrors       atomic.Int64
	httpLatencyUS    atomic.Int64
	httpLatencyCount atomic.Int64

	nsqPublishTotal  atomic.Int64
	nsqPublishErrors atomic.Int64
	nsqPublishBytes  atomic.Int64
	nsqDepthLogs     atomic.Int64
	nsqDepthEvents   atomic.Int64

	consumerMessages     atomic.Int64
	consumerErrors       atomic.Int64
	consumerLatencyUS    atomic.Int64
	consumerLatencyCount atomic.Int64

	dbFlushTotal        atomic.Int64
	dbFlushErrors       atomic.Int64
	dbFlushLatencyUS    atomic.Int64
	dbFlushLatencyCount atomic.Int64
	dbFlushRows         atomic.Int64

	cleanupDeletedLogs   atomic.Int64
	cleanupDeletedEvents atomic.Int64
}

func New() *Stats {
	return &Stats{start: time.Now()}
}

func (s *Stats) ObserveHTTP(status int, dur time.Duration) {
	if s == nil {
		return
	}
	s.httpRequests.Add(1)
	if status >= 500 {
		s.httpErrors.Add(1)
	}
	s.httpLatencyUS.Add(dur.Microseconds())
	s.httpLatencyCount.Add(1)
}

func (s *Stats) ObserveNSQPublish(bytes int, err error) {
	if s == nil {
		return
	}
	s.nsqPublishTotal.Add(1)
	s.nsqPublishBytes.Add(int64(bytes))
	if err != nil {
		s.nsqPublishErrors.Add(1)
	}
}

func (s *Stats) SetNSQDepth(topic string, depth int64) {
	if s == nil {
		return
	}
	switch topic {
	case "logs":
		s.nsqDepthLogs.Store(depth)
	case "events":
		s.nsqDepthEvents.Store(depth)
	}
}

func (s *Stats) ObserveConsumerMessage(dur time.Duration, err error) {
	if s == nil {
		return
	}
	s.consumerMessages.Add(1)
	if err != nil {
		s.consumerErrors.Add(1)
	}
	s.consumerLatencyUS.Add(dur.Microseconds())
	s.consumerLatencyCount.Add(1)
}

func (s *Stats) ObserveDBFlush(rows int, dur time.Duration, err error) {
	if s == nil {
		return
	}
	s.dbFlushTotal.Add(1)
	s.dbFlushRows.Add(int64(rows))
	if err != nil {
		s.dbFlushErrors.Add(1)
	}
	s.dbFlushLatencyUS.Add(dur.Microseconds())
	s.dbFlushLatencyCount.Add(1)
}

func (s *Stats) ObserveCleanupDeleted(logs, events int64) {
	if s == nil {
		return
	}
	if logs > 0 {
		s.cleanupDeletedLogs.Add(logs)
	}
	if events > 0 {
		s.cleanupDeletedEvents.Add(events)
	}
}

type Snapshot struct {
	UptimeSeconds int64 `json:"uptime_seconds"`

	HTTP struct {
		Requests int64   `json:"requests"`
		Errors   int64   `json:"errors"`
		AvgMS    float64 `json:"avg_ms"`
	} `json:"http"`

	NSQ struct {
		PublishTotal  int64 `json:"publish_total"`
		PublishErrors int64 `json:"publish_errors"`
		PublishBytes  int64 `json:"publish_bytes"`
		DepthLogs     int64 `json:"depth_logs"`
		DepthEvents   int64 `json:"depth_events"`
	} `json:"nsq"`

	Consumer struct {
		Messages int64   `json:"messages"`
		Errors   int64   `json:"errors"`
		AvgMS    float64 `json:"avg_ms"`
	} `json:"consumer"`

	DBFlush struct {
		Flushes int64   `json:"flushes"`
		Errors  int64   `json:"errors"`
		Rows    int64   `json:"rows"`
		AvgMS   float64 `json:"avg_ms"`
	} `json:"db_flush"`

	Cleanup struct {
		DeletedLogs   int64 `json:"deleted_logs"`
		DeletedEvents int64 `json:"deleted_events"`
	} `json:"cleanup"`
}

func (s *Stats) Snapshot() Snapshot {
	var snap Snapshot
	if s == nil {
		return snap
	}
	snap.UptimeSeconds = int64(time.Since(s.start).Seconds())

	req := s.httpRequests.Load()
	errs := s.httpErrors.Load()
	latUS := s.httpLatencyUS.Load()
	latN := s.httpLatencyCount.Load()
	snap.HTTP.Requests = req
	snap.HTTP.Errors = errs
	if latN > 0 {
		snap.HTTP.AvgMS = float64(latUS) / float64(latN) / 1000.0
	}

	snap.NSQ.PublishTotal = s.nsqPublishTotal.Load()
	snap.NSQ.PublishErrors = s.nsqPublishErrors.Load()
	snap.NSQ.PublishBytes = s.nsqPublishBytes.Load()
	snap.NSQ.DepthLogs = s.nsqDepthLogs.Load()
	snap.NSQ.DepthEvents = s.nsqDepthEvents.Load()

	msg := s.consumerMessages.Load()
	cerr := s.consumerErrors.Load()
	clatUS := s.consumerLatencyUS.Load()
	clatN := s.consumerLatencyCount.Load()
	snap.Consumer.Messages = msg
	snap.Consumer.Errors = cerr
	if clatN > 0 {
		snap.Consumer.AvgMS = float64(clatUS) / float64(clatN) / 1000.0
	}

	snap.DBFlush.Flushes = s.dbFlushTotal.Load()
	snap.DBFlush.Errors = s.dbFlushErrors.Load()
	snap.DBFlush.Rows = s.dbFlushRows.Load()
	dlatUS := s.dbFlushLatencyUS.Load()
	dlatN := s.dbFlushLatencyCount.Load()
	if dlatN > 0 {
		snap.DBFlush.AvgMS = float64(dlatUS) / float64(dlatN) / 1000.0
	}

	snap.Cleanup.DeletedLogs = s.cleanupDeletedLogs.Load()
	snap.Cleanup.DeletedEvents = s.cleanupDeletedEvents.Load()
	return snap
}

func (s *Stats) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Snapshot())
}
