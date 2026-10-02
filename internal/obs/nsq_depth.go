package obs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type nsqStats struct {
	Topics []struct {
		TopicName string `json:"topic_name"`
		Channels  []struct {
			Depth                int64  `json:"depth"`
			InFlightCount        int64  `json:"in_flight_count"`
			DeferredCount        int64  `json:"deferred_count"`
			MessageCount         int64  `json:"message_count"`
			RequeueCount         int64  `json:"requeue_count"`
			TimeoutCount         int64  `json:"timeout_count"`
			BackendDepth         int64  `json:"backend_depth"`
			ClientCount          int64  `json:"client_count"`
			ReadyCount           int64  `json:"ready_count"`
			FinishCount          int64  `json:"finish_count"`
			DeferredMsgs         int64  `json:"deferred_messages"`
			InFlightMsgs         int64  `json:"in_flight_messages"`
			MemoryDepth          int64  `json:"memory_depth"`
			ChannelName          string `json:"channel_name"`
			E2eProcessingLatency any    `json:"e2e_processing_latency"`
		} `json:"channels"`
	} `json:"topics"`
}

func StartNSQDepthPoller(ctx context.Context, stats *Stats, nsqdHTTPAddr string, interval time.Duration) {
	StartNSQDepthPollerFunc(ctx, stats, nsqdHTTPAddr, interval, nil)
}

// StartNSQDepthPollerFunc additionally invokes onDepth with each topic's
// total backlog (depth + in-flight + deferred); used by the ingest
// backpressure guard in cmd/gateway.
func StartNSQDepthPollerFunc(ctx context.Context, stats *Stats, nsqdHTTPAddr string, interval time.Duration, onDepth func(topic string, total int64)) {
	if stats == nil {
		return
	}
	if strings.TrimSpace(nsqdHTTPAddr) == "" {
		return
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	url := strings.TrimSpace(nsqdHTTPAddr)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	url = strings.TrimRight(url, "/") + "/stats?format=json"

	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	_ = pollOnce(ctx, client, url, stats, onDepth)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = pollOnce(ctx, client, url, stats, onDepth)
		}
	}
}

func pollOnce(ctx context.Context, client *http.Client, url string, stats *Stats, onDepth func(topic string, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("nsqd stats http status=%d", res.StatusCode)
	}
	var payload nsqStats
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return err
	}
	depths := map[string]int64{}
	for _, t := range payload.Topics {
		total := int64(0)
		for _, ch := range t.Channels {
			// backlog = depth + in-flight + deferred
			total += ch.Depth + ch.InFlightCount + ch.DeferredCount
		}
		depths[t.TopicName] = total
	}
	if v, ok := depths["logs"]; ok {
		stats.SetNSQDepth("logs", v)
		if onDepth != nil {
			onDepth("logs", v)
		}
	}
	if v, ok := depths["events"]; ok {
		stats.SetNSQDepth("events", v)
		if onDepth != nil {
			onDepth("events", v)
		}
	}
	return nil
}
