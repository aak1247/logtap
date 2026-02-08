package obs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStartNSQDepthPoller_PollOnce(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "topics": [
    {"topic_name":"logs","channels":[{"channel_name":"c1","depth":10,"in_flight_count":2,"deferred_count":1}]},
    {"topic_name":"events","channels":[{"channel_name":"c1","depth":3,"in_flight_count":0,"deferred_count":0}]}
  ]
}`))
	}))
	t.Cleanup(ts.Close)

	stats := New()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go StartNSQDepthPoller(ctx, stats, ts.URL, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	snap := stats.Snapshot()
	if snap.NSQ.DepthLogs != 13 {
		t.Fatalf("expected logs depth=13, got %d", snap.NSQ.DepthLogs)
	}
	if snap.NSQ.DepthEvents != 3 {
		t.Fatalf("expected events depth=3, got %d", snap.NSQ.DepthEvents)
	}
}
