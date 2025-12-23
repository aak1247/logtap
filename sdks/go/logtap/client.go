package logtap

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SDKName    = "logtap-go"
	SDKVersion = "0.1.0"
)

type BeforeSendFunc func(payload any) any

type ClientOptions struct {
	BaseURL   string
	ProjectID int64

	ProjectKey string

	FlushInterval time.Duration
	MaxBatchSize  int
	MaxQueueSize  int
	Timeout       time.Duration

	Gzip bool

	DeviceID string
	User     *User

	GlobalTags       map[string]string
	GlobalFields     map[string]any
	GlobalProperties map[string]any
	GlobalContexts   map[string]any

	BeforeSend BeforeSendFunc
	HTTPClient *http.Client

	Now func() time.Time
}

type Client struct {
	baseURL    string
	projectID  string
	projectKey string

	flushInterval time.Duration
	maxBatchSize  int
	maxQueueSize  int
	timeout       time.Duration

	gzip bool

	httpClient *http.Client
	now        func() time.Time

	mu sync.Mutex

	deviceID         string
	user             map[string]any
	globalTags       map[string]string
	globalFields     map[string]any
	globalProperties map[string]any
	globalContexts   map[string]any
	beforeSend       BeforeSendFunc

	logQueue   []LogPayload
	trackQueue []TrackEventPayload

	backoff time.Duration

	flushMu sync.Mutex

	ticker *time.Ticker
	done   chan struct{}
	wg     sync.WaitGroup
	closed bool
}

func NewClient(options ClientOptions) (*Client, error) {
	baseURL, err := normalizeBaseURL(options.BaseURL)
	if err != nil {
		return nil, err
	}
	if options.ProjectID <= 0 {
		return nil, errors.New("projectID must be > 0")
	}

	flushInterval := options.FlushInterval
	if flushInterval == 0 {
		flushInterval = 2 * time.Second
	}

	maxBatchSize := options.MaxBatchSize
	if maxBatchSize <= 0 {
		maxBatchSize = 50
	}

	maxQueueSize := options.MaxQueueSize
	if maxQueueSize <= 0 {
		maxQueueSize = 1000
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	nowFn := options.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	deviceID := strings.TrimSpace(options.DeviceID)
	if deviceID == "" {
		deviceID = newDeviceID()
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	c := &Client{
		baseURL:    baseURL,
		projectID:  strconv.FormatInt(options.ProjectID, 10),
		projectKey: strings.TrimSpace(options.ProjectKey),

		flushInterval: flushInterval,
		maxBatchSize:  maxBatchSize,
		maxQueueSize:  maxQueueSize,
		timeout:       timeout,
		gzip:          options.Gzip,

		httpClient: httpClient,
		now:        nowFn,

		deviceID:         deviceID,
		user:             userToMap(options.User),
		globalTags:       cloneStringMap(options.GlobalTags),
		globalFields:     jsonSafeAnyMap(options.GlobalFields),
		globalProperties: jsonSafeAnyMap(options.GlobalProperties),
		globalContexts:   jsonSafeAnyMap(options.GlobalContexts),
		beforeSend:       options.BeforeSend,

		done: make(chan struct{}),
	}

	if c.flushInterval > 0 {
		c.ticker = time.NewTicker(c.flushInterval)
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case <-c.ticker.C:
					_ = c.Flush(context.Background())
				case <-c.done:
					return
				}
			}
		}()
	}

	return c, nil
}

func userToMap(u *User) map[string]any {
	if u == nil {
		return nil
	}
	out := map[string]any{}
	if strings.TrimSpace(u.ID) != "" {
		out["id"] = strings.TrimSpace(u.ID)
	}
	if strings.TrimSpace(u.Email) != "" {
		out["email"] = strings.TrimSpace(u.Email)
	}
	if strings.TrimSpace(u.Username) != "" {
		out["username"] = strings.TrimSpace(u.Username)
	}
	if len(u.Traits) > 0 {
		out["traits"] = jsonSafeAnyMap(u.Traits)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (c *Client) SetUser(user *User) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.user = userToMap(user)
}

func (c *Client) Identify(userID string, traits map[string]any) {
	id := strings.TrimSpace(userID)
	if id == "" {
		return
	}
	u := &User{ID: id}
	if len(traits) > 0 {
		u.Traits = traits
	}
	c.SetUser(u)
}

func (c *Client) ClearUser() {
	c.SetUser(nil)
}

func (c *Client) SetDeviceID(deviceID string) {
	id := strings.TrimSpace(deviceID)
	if id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deviceID = id
}

func (c *Client) Log(level Level, message string, fields map[string]any, opts *LogOptions) {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return
	}
	if strings.TrimSpace(string(level)) == "" {
		level = LevelInfo
	}

	now := c.now().UTC()
	ts := now

	var traceID, spanID string
	var tags map[string]string
	var deviceID string
	var user map[string]any
	var contexts map[string]any
	var extra map[string]any

	if opts != nil {
		if !opts.Timestamp.IsZero() {
			ts = opts.Timestamp.UTC()
		}
		traceID = strings.TrimSpace(opts.TraceID)
		spanID = strings.TrimSpace(opts.SpanID)
		tags = cloneStringMap(opts.Tags)
		deviceID = strings.TrimSpace(opts.DeviceID)
		user = userToMap(opts.User)
		contexts = jsonSafeAnyMap(opts.Contexts)
		extra = jsonSafeAnyMap(opts.Extra)
	}

	c.mu.Lock()
	if deviceID == "" {
		deviceID = c.deviceID
	}
	if user == nil {
		user = cloneAnyMap(c.user)
	}
	globalTags := cloneStringMap(c.globalTags)
	globalFields := cloneAnyMap(c.globalFields)
	globalContexts := cloneAnyMap(c.globalContexts)
	beforeSend := c.beforeSend
	c.mu.Unlock()

	payload := LogPayload{
		Level:     level,
		Message:   msg,
		Timestamp: &ts,
		DeviceID:  deviceID,
		TraceID:   traceID,
		SpanID:    spanID,
		Fields:    mergeAnyMap(globalFields, jsonSafeAnyMap(fields)),
		Tags:      mergeStringMap(globalTags, tags),
		User:      user,
		Contexts:  mergeAnyMap(globalContexts, contexts),
		Extra:     extra,
		SDK:       sdkInfo(),
	}

	if beforeSend != nil {
		if p := applyBeforeSend[LogPayload](beforeSend, &payload); p == nil {
			return
		} else {
			payload = *p
		}
	}

	c.enqueueLog(payload)
}

func (c *Client) Debug(message string, fields map[string]any, opts *LogOptions) {
	c.Log(LevelDebug, message, fields, opts)
}

func (c *Client) Info(message string, fields map[string]any, opts *LogOptions) {
	c.Log(LevelInfo, message, fields, opts)
}

func (c *Client) Warn(message string, fields map[string]any, opts *LogOptions) {
	c.Log(LevelWarn, message, fields, opts)
}

func (c *Client) Error(message string, fields map[string]any, opts *LogOptions) {
	c.Log(LevelError, message, fields, opts)
}

func (c *Client) Fatal(message string, fields map[string]any, opts *LogOptions) {
	c.Log(LevelFatal, message, fields, opts)
}

func (c *Client) Track(name string, properties map[string]any, opts *TrackOptions) {
	n := strings.TrimSpace(name)
	if n == "" {
		return
	}

	now := c.now().UTC()
	ts := now

	var traceID, spanID string
	var tags map[string]string
	var deviceID string
	var user map[string]any
	var contexts map[string]any
	var extra map[string]any

	if opts != nil {
		if !opts.Timestamp.IsZero() {
			ts = opts.Timestamp.UTC()
		}
		traceID = strings.TrimSpace(opts.TraceID)
		spanID = strings.TrimSpace(opts.SpanID)
		tags = cloneStringMap(opts.Tags)
		deviceID = strings.TrimSpace(opts.DeviceID)
		user = userToMap(opts.User)
		contexts = jsonSafeAnyMap(opts.Contexts)
		extra = jsonSafeAnyMap(opts.Extra)
	}

	c.mu.Lock()
	if deviceID == "" {
		deviceID = c.deviceID
	}
	if user == nil {
		user = cloneAnyMap(c.user)
	}
	globalTags := cloneStringMap(c.globalTags)
	globalProperties := cloneAnyMap(c.globalProperties)
	globalContexts := cloneAnyMap(c.globalContexts)
	beforeSend := c.beforeSend
	c.mu.Unlock()

	payload := TrackEventPayload{
		Name:       n,
		Timestamp:  &ts,
		DeviceID:   deviceID,
		TraceID:    traceID,
		SpanID:     spanID,
		Properties: mergeAnyMap(globalProperties, jsonSafeAnyMap(properties)),
		Tags:       mergeStringMap(globalTags, tags),
		User:       user,
		Contexts:   mergeAnyMap(globalContexts, contexts),
		Extra:      extra,
		SDK:        sdkInfo(),
	}

	if beforeSend != nil {
		if p := applyBeforeSend[TrackEventPayload](beforeSend, &payload); p == nil {
			return
		} else {
			payload = *p
		}
	}

	c.enqueueTrack(payload)
}

func sdkInfo() map[string]any {
	return map[string]any{
		"name":    SDKName,
		"version": SDKVersion,
		"runtime": "go",
		"goos":    runtime.GOOS,
		"goarch":  runtime.GOARCH,
	}
}

func applyBeforeSend[T any](fn BeforeSendFunc, payload *T) (out *T) {
	out = payload
	defer func() {
		if recover() != nil {
			out = payload
		}
	}()

	v := fn(payload)
	if v == nil {
		return nil
	}

	if p, ok := v.(*T); ok && p != nil {
		return p
	}
	if vv, ok := v.(T); ok {
		return &vv
	}
	return payload
}

func (c *Client) enqueueLog(payload LogPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logQueue = append(c.logQueue, payload)
	if len(c.logQueue) > c.maxQueueSize {
		c.logQueue = append([]LogPayload(nil), c.logQueue[len(c.logQueue)-c.maxQueueSize:]...)
	}
}

func (c *Client) enqueueTrack(payload TrackEventPayload) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trackQueue = append(c.trackQueue, payload)
	if len(c.trackQueue) > c.maxQueueSize {
		c.trackQueue = append([]TrackEventPayload(nil), c.trackQueue[len(c.trackQueue)-c.maxQueueSize:]...)
	}
}

func (c *Client) Flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	for {
		if err := c.waitBackoff(ctx); err != nil {
			return err
		}

		sentAny := false

		if ok, err := c.flushLogsOnce(ctx); err != nil {
			return err
		} else if ok {
			sentAny = true
		}

		if ok, err := c.flushTrackOnce(ctx); err != nil {
			return err
		} else if ok {
			sentAny = true
		}

		if sentAny {
			c.mu.Lock()
			c.backoff = 0
			c.mu.Unlock()
			continue
		}
		return nil
	}
}

func (c *Client) Close(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	if c.ticker != nil {
		c.ticker.Stop()
	}
	close(c.done)
	c.mu.Unlock()

	c.wg.Wait()
	return c.Flush(ctx)
}

func (c *Client) waitBackoff(ctx context.Context) error {
	c.mu.Lock()
	d := c.backoff
	c.mu.Unlock()
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) flushLogsOnce(ctx context.Context) (bool, error) {
	batch := c.dequeueLogs()
	if len(batch) == 0 {
		return false, nil
	}
	if ok, err := c.postJSON(ctx, "/logs/", batch); err != nil || !ok {
		c.requeueLogsFront(batch)
		c.bumpBackoff()
		if err != nil {
			return false, err
		}
		return false, errors.New("logtap: failed to post logs batch")
	}
	return true, nil
}

func (c *Client) flushTrackOnce(ctx context.Context) (bool, error) {
	batch := c.dequeueTrack()
	if len(batch) == 0 {
		return false, nil
	}
	if ok, err := c.postJSON(ctx, "/track/", batch); err != nil || !ok {
		c.requeueTrackFront(batch)
		c.bumpBackoff()
		if err != nil {
			return false, err
		}
		return false, errors.New("logtap: failed to post track batch")
	}
	return true, nil
}

func (c *Client) dequeueLogs() []LogPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.logQueue) == 0 {
		return nil
	}
	n := c.maxBatchSize
	if n > len(c.logQueue) {
		n = len(c.logQueue)
	}
	batch := append([]LogPayload(nil), c.logQueue[:n]...)
	c.logQueue = c.logQueue[n:]
	return batch
}

func (c *Client) dequeueTrack() []TrackEventPayload {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.trackQueue) == 0 {
		return nil
	}
	n := c.maxBatchSize
	if n > len(c.trackQueue) {
		n = len(c.trackQueue)
	}
	batch := append([]TrackEventPayload(nil), c.trackQueue[:n]...)
	c.trackQueue = c.trackQueue[n:]
	return batch
}

func (c *Client) requeueLogsFront(batch []LogPayload) {
	if len(batch) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.logQueue = append(append([]LogPayload(nil), batch...), c.logQueue...)
	if len(c.logQueue) > c.maxQueueSize {
		c.logQueue = append([]LogPayload(nil), c.logQueue[len(c.logQueue)-c.maxQueueSize:]...)
	}
}

func (c *Client) requeueTrackFront(batch []TrackEventPayload) {
	if len(batch) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.trackQueue = append(append([]TrackEventPayload(nil), batch...), c.trackQueue...)
	if len(c.trackQueue) > c.maxQueueSize {
		c.trackQueue = append([]TrackEventPayload(nil), c.trackQueue[len(c.trackQueue)-c.maxQueueSize:]...)
	}
}

func (c *Client) bumpBackoff() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backoff <= 0 {
		c.backoff = 500 * time.Millisecond
		return
	}
	c.backoff *= 2
	if c.backoff > 30*time.Second {
		c.backoff = 30 * time.Second
	}
}

func (c *Client) postJSON(ctx context.Context, path string, payload any) (bool, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal payload: %w", err)
	}

	var reqBody io.Reader = bytes.NewReader(body)
	var contentEncoding string
	if c.gzip {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(body); err != nil {
			_ = zw.Close()
			return false, fmt.Errorf("gzip write: %w", err)
		}
		if err := zw.Close(); err != nil {
			return false, fmt.Errorf("gzip close: %w", err)
		}
		reqBody = &buf
		contentEncoding = "gzip"
	}

	url := fmt.Sprintf("%s/api/%s%s", c.baseURL, c.projectID, path)

	reqCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, reqBody)
	if err != nil {
		return false, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", SDKName+"/"+SDKVersion)
	if c.projectKey != "" {
		req.Header.Set("X-Project-Key", c.projectKey)
	}
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64<<10))

	return res.StatusCode >= 200 && res.StatusCode < 300, nil
}
