package logtap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	defer r.Body.Close()

	var rd io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatalf("gzip.NewReader: %v", err)
		}
		defer zr.Close()
		rd = zr
	}

	b, err := io.ReadAll(rd)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	return b
}

func TestClient_Flush_SendsLogsAndTrack_GzipAndHeaders(t *testing.T) {
	t.Parallel()

	type received struct {
		Path    string
		Headers http.Header
		Body    []byte
	}

	var (
		mu    sync.Mutex
		calls []received
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, received{
			Path:    r.URL.Path,
			Headers: r.Header.Clone(),
			Body:    readBody(t, r),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	client, err := NewClient(ClientOptions{
		BaseURL:       srv.URL,
		ProjectID:     1,
		ProjectKey:    "pk_test",
		Gzip:          true,
		FlushInterval: -1,
		Now: func() time.Time {
			return now
		},
		GlobalTags: map[string]string{"env": "test"},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	client.Info("hello", map[string]any{"k": "v"}, &LogOptions{Tags: map[string]string{"req": "1"}})
	client.Track("signup", map[string]any{"plan": "pro"}, &TrackOptions{Tags: map[string]string{"req": "1"}})

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mu.Lock()
	got := append([]received(nil), calls...)
	mu.Unlock()

	if len(got) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(got))
	}

	checkBatch := func(t *testing.T, call received, kind string) {
		t.Helper()
		if call.Headers.Get("X-Project-Key") != "pk_test" {
			t.Fatalf("%s: missing X-Project-Key header", kind)
		}
		if call.Headers.Get("Content-Type") != "application/json" {
			t.Fatalf("%s: unexpected Content-Type: %q", kind, call.Headers.Get("Content-Type"))
		}
		if call.Headers.Get("Content-Encoding") != "gzip" {
			t.Fatalf("%s: expected gzip encoding", kind)
		}

		var items []map[string]any
		if err := json.Unmarshal(call.Body, &items); err != nil {
			t.Fatalf("%s: json.Unmarshal: %v", kind, err)
		}
		if len(items) != 1 {
			t.Fatalf("%s: expected 1 item, got %d", kind, len(items))
		}

		it := items[0]
		if it["device_id"] == "" {
			t.Fatalf("%s: expected device_id", kind)
		}
		if it["timestamp"] == "" {
			t.Fatalf("%s: expected timestamp", kind)
		}
		sdk, _ := it["sdk"].(map[string]any)
		if sdk["name"] != SDKName {
			t.Fatalf("%s: expected sdk.name=%q, got %v", kind, SDKName, sdk["name"])
		}
	}

	for _, call := range got {
		switch call.Path {
		case "/api/1/logs/":
			checkBatch(t, call, "logs")
			var items []map[string]any
			_ = json.Unmarshal(call.Body, &items)
			if items[0]["message"] != "hello" || items[0]["level"] != "info" {
				t.Fatalf("logs: unexpected payload: %v", items[0])
			}
			fields, _ := items[0]["fields"].(map[string]any)
			if fields["k"] != "v" {
				t.Fatalf("logs: expected fields.k=v, got %v", fields["k"])
			}
			tags, _ := items[0]["tags"].(map[string]any)
			if tags["env"] != "test" || tags["req"] != "1" {
				t.Fatalf("logs: expected merged tags, got %v", tags)
			}
		case "/api/1/track/":
			checkBatch(t, call, "track")
			var items []map[string]any
			_ = json.Unmarshal(call.Body, &items)
			if items[0]["name"] != "signup" {
				t.Fatalf("track: expected name=signup, got %v", items[0]["name"])
			}
			props, _ := items[0]["properties"].(map[string]any)
			if props["plan"] != "pro" {
				t.Fatalf("track: expected properties.plan=pro, got %v", props["plan"])
			}
		default:
			t.Fatalf("unexpected path: %s", call.Path)
		}
	}
}

func TestClient_Flush_RespectsMaxBatchSize(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		logCalls  int
		lastBatch []map[string]any
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/logs/" {
			w.WriteHeader(http.StatusAccepted)
			return
		}

		b := readBody(t, r)
		var items []map[string]any
		if err := json.Unmarshal(b, &items); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		mu.Lock()
		logCalls++
		lastBatch = items
		mu.Unlock()

		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(ClientOptions{
		BaseURL:       srv.URL,
		ProjectID:     1,
		FlushInterval: -1,
		MaxBatchSize:  2,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	for i := 0; i < 5; i++ {
		client.Info("m"+strconv.Itoa(i), nil, nil)
	}

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if logCalls != 3 {
		t.Fatalf("expected 3 log batches (2+2+1), got %d", logCalls)
	}
	if len(lastBatch) != 1 {
		t.Fatalf("expected last batch size 1, got %d", len(lastBatch))
	}
}

func TestClient_Queue_DropsOldestBeyondMaxQueueSize(t *testing.T) {
	t.Parallel()

	var (
		mu       sync.Mutex
		lastBody []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/1/logs/" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		mu.Lock()
		lastBody = readBody(t, r)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(ClientOptions{
		BaseURL:       srv.URL,
		ProjectID:     1,
		FlushInterval: -1,
		MaxQueueSize:  2,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	client.Info("m0", nil, nil)
	client.Info("m1", nil, nil)
	client.Info("m2", nil, nil)

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mu.Lock()
	b := append([]byte(nil), lastBody...)
	mu.Unlock()

	if len(b) == 0 {
		t.Fatalf("expected request body")
	}

	var items []map[string]any
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0]["message"] != "m1" || items[1]["message"] != "m2" {
		t.Fatalf("expected last 2 messages (m1,m2), got %v", []any{items[0]["message"], items[1]["message"]})
	}
}

func TestClient_PostBody_IsValidJSONArray(t *testing.T) {
	t.Parallel()

	ch := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ch <- readBody(t, r)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(ClientOptions{
		BaseURL:       srv.URL,
		ProjectID:     1,
		FlushInterval: -1,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client.Info("hello", nil, nil)

	if err := client.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	body := <-ch

	// Ensure it's a JSON array (batch endpoint shape).
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '[' {
		t.Fatalf("expected JSON array body, got: %s", string(body))
	}
}
