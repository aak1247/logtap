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

	// PersistQueue enables a local persistent queue so logs/events can survive process restarts.
	// By default it stores to a JSON file in the user's cache dir (or temp dir as fallback).
	PersistQueue    bool
	QueueFilePath   string
	PersistDebounce time.Duration

	FlushInterval time.Duration
	MinBatchSize  int
	MaxBatchSize  int
	MaxQueueSize  int
	Timeout       time.Duration

	Gzip bool

	ImmediateEvents []string
	ImmediateEvent  func(name string, payload TrackEventPayload) bool

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
	minBatchSize  int
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

	logQueue      []LogPayload
	trackQueue    []TrackEventPayload
	firstQueuedAt time.Time

	immediateEvents map[string]struct{}
	immediateEvent  func(name string, payload TrackEventPayload) bool

	backoff time.Duration

	flushMu sync.Mutex

	ticker  *time.Ticker
	flushCh chan flushRequest
	done    chan struct{}
	wg      sync.WaitGroup
	closed  bool

	queueStore      *diskQueueStore
	persistDebounce time.Duration
	persistMu       sync.Mutex
	persistTimer    *time.Timer
	persistWriteMu  sync.Mutex
}

type flushRequest struct {
	force bool
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

	minBatchSize := options.MinBatchSize
	if minBatchSize <= 0 {
		minBatchSize = 1
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

	immediateEvents := map[string]struct{}{}
	for _, n := range options.ImmediateEvents {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		immediateEvents[n] = struct{}{}
	}

	c := &Client{
		baseURL:    baseURL,
		projectID:  strconv.FormatInt(options.ProjectID, 10),
		projectKey: strings.TrimSpace(options.ProjectKey),

		flushInterval: flushInterval,
		minBatchSize:  minBatchSize,
		maxBatchSize:  maxBatchSize,
		maxQueueSize:  maxQueueSize,
		timeout:       timeout,
		gzip:          options.Gzip,

		immediateEvents: immediateEvents,
		immediateEvent:  options.ImmediateEvent,

		httpClient: httpClient,
		now:        nowFn,

		deviceID:         deviceID,
		user:             userToMap(options.User),
		globalTags:       cloneStringMap(options.GlobalTags),
		globalFields:     jsonSafeAnyMap(options.GlobalFields),
		globalProperties: jsonSafeAnyMap(options.GlobalProperties),
		globalContexts:   jsonSafeAnyMap(options.GlobalContexts),
		beforeSend:       options.BeforeSend,

		flushCh: make(chan flushRequest, 1),
		done:    make(chan struct{}),
	}

	if options.PersistDebounce > 0 {
		c.persistDebounce = options.PersistDebounce
	} else {
		c.persistDebounce = 0
	}
	if options.PersistQueue {
		path := strings.TrimSpace(options.QueueFilePath)
		if path == "" {
			path = defaultQueueFilePath(options.ProjectID)
		}
		c.queueStore = &diskQueueStore{path: path}
		c.loadPersistedQueue()
	}

	if c.flushInterval > 0 {
		c.ticker = time.NewTicker(c.flushInterval)
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			var tick <-chan time.Time
			if c.ticker != nil {
				tick = c.ticker.C
			}
			select {
			case <-tick:
				_ = c.FlushIfNeeded(context.Background())
			case req := <-c.flushCh:
				if req.force {
					_ = c.Flush(context.Background())
				} else {
					_ = c.FlushIfNeeded(context.Background())
				}
			case <-c.done:
				return
			}
		}
	}()

	if c.queueStore != nil {
		c.mu.Lock()
		queued := len(c.logQueue) + len(c.trackQueue)
		c.mu.Unlock()
		if queued > 0 {
			c.signalRetry()
		}
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
	immediate := false

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
		immediate = opts.Immediate
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

	if immediate || c.isImmediateEvent(n, payload) {
		c.enqueueTrack(payload)
		c.signalRetry()
		return
	}

	c.enqueueTrack(payload)
}

func (c *Client) isImmediateEvent(name string, payload TrackEventPayload) bool {
	if c == nil {
		return false
	}
	if c.immediateEvent != nil {
		defer func() { _ = recover() }()
		return c.immediateEvent(name, payload)
	}
	if c.immediateEvents == nil {
		return false
	}
	_, ok := c.immediateEvents[name]
	return ok
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
	var shouldSignal bool
	c.mu.Lock()
	c.logQueue = append(c.logQueue, payload)
	if c.firstQueuedAt.IsZero() {
		c.firstQueuedAt = c.now().UTC()
	}
	if len(c.logQueue) > c.maxQueueSize {
		c.logQueue = append([]LogPayload(nil), c.logQueue[len(c.logQueue)-c.maxQueueSize:]...)
	}
	shouldSignal = c.minBatchSize > 1 && (len(c.logQueue)+len(c.trackQueue)) >= c.minBatchSize
	c.mu.Unlock()
	c.schedulePersist()
	if shouldSignal {
		c.signalFlush()
	}
}

func (c *Client) enqueueTrack(payload TrackEventPayload) {
	var shouldSignal bool
	c.mu.Lock()
	c.trackQueue = append(c.trackQueue, payload)
	if c.firstQueuedAt.IsZero() {
		c.firstQueuedAt = c.now().UTC()
	}
	if len(c.trackQueue) > c.maxQueueSize {
		c.trackQueue = append([]TrackEventPayload(nil), c.trackQueue[len(c.trackQueue)-c.maxQueueSize:]...)
	}
	shouldSignal = c.minBatchSize > 1 && (len(c.logQueue)+len(c.trackQueue)) >= c.minBatchSize
	c.mu.Unlock()
	c.schedulePersist()
	if shouldSignal {
		c.signalFlush()
	}
}

func (c *Client) signalFlush() {
	if c == nil {
		return
	}
	select {
	case c.flushCh <- flushRequest{force: false}:
	default:
	}
}

func (c *Client) signalRetry() {
	if c == nil {
		return
	}
	select {
	case c.flushCh <- flushRequest{force: true}:
	default:
	}
}

// FlushIfNeeded auto-flushes when either:
// - queued items reach MinBatchSize, or
// - the oldest queued item waits longer than FlushInterval.
//
// It does nothing if neither threshold is met.
func (c *Client) FlushIfNeeded(ctx context.Context) error {
	c.mu.Lock()
	queued := len(c.logQueue) + len(c.trackQueue)
	first := c.firstQueuedAt
	minBatch := c.minBatchSize
	interval := c.flushInterval
	now := c.now().UTC()
	c.mu.Unlock()

	if queued == 0 {
		return nil
	}
	if minBatch > 1 && queued >= minBatch {
		return c.Flush(ctx)
	}
	if interval > 0 && !first.IsZero() && now.Sub(first) >= interval {
		return c.Flush(ctx)
	}
	return nil
}

func (c *Client) Flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	for {
		if err := c.waitBackoff(ctx); err != nil {
			return err
		}

		sentAny := false

		if ok, err := c.flushTrackOnce(ctx); err != nil {
			return err
		} else if ok {
			sentAny = true
		}

		if ok, err := c.flushLogsOnce(ctx); err != nil {
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
		c.mu.Lock()
		if len(c.logQueue)+len(c.trackQueue) == 0 {
			c.firstQueuedAt = time.Time{}
		}
		c.mu.Unlock()
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
	err := c.Flush(ctx)
	c.stopPersist()
	return err
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
	batch := c.peekLogs()
	if len(batch) == 0 {
		return false, nil
	}
	if ok, err := c.postJSON(ctx, "/logs/", batch); err != nil || !ok {
		c.bumpBackoff()
		c.signalRetry()
		if err != nil {
			return false, err
		}
		return false, errors.New("logtap: failed to post logs batch")
	}
	c.dropLogs(len(batch))
	c.schedulePersist()
	return true, nil
}

func (c *Client) flushTrackOnce(ctx context.Context) (bool, error) {
	batch := c.peekTrack()
	if len(batch) == 0 {
		return false, nil
	}
	if ok, err := c.postJSON(ctx, "/track/", batch); err != nil || !ok {
		c.bumpBackoff()
		c.signalRetry()
		if err != nil {
			return false, err
		}
		return false, errors.New("logtap: failed to post track batch")
	}
	c.dropTrack(len(batch))
	c.schedulePersist()
	return true, nil
}

func (c *Client) peekLogs() []LogPayload {
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
	return batch
}

func (c *Client) peekTrack() []TrackEventPayload {
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
	return batch
}

func (c *Client) dropLogs(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 {
		return
	}
	if n >= len(c.logQueue) {
		c.logQueue = nil
		return
	}
	c.logQueue = c.logQueue[n:]
}

func (c *Client) dropTrack(n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if n <= 0 {
		return
	}
	if n >= len(c.trackQueue) {
		c.trackQueue = nil
		return
	}
	c.trackQueue = c.trackQueue[n:]
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
