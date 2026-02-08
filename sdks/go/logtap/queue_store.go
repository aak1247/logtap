package logtap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type persistedQueueState struct {
	V             int                 `json:"v"`
	FirstQueuedAt *time.Time          `json:"first_queued_at,omitempty"`
	Logs          []LogPayload        `json:"logs,omitempty"`
	Track         []TrackEventPayload `json:"track,omitempty"`
}

type diskQueueStore struct {
	path string
}

func defaultQueueFilePath(projectID int64) string {
	base, err := os.UserCacheDir()
	if err != nil || strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "logtap")
	return filepath.Join(dir, "queue_"+strconv.FormatInt(projectID, 10)+".json")
}

func (s *diskQueueStore) Load() (*persistedQueueState, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil, nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st persistedQueueState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, err
	}
	if st.V == 0 {
		st.V = 1
	}
	return &st, nil
}

func (s *diskQueueStore) Clear() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	_ = os.Remove(s.path)
	return nil
}

func (s *diskQueueStore) Save(st *persistedQueueState) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	if st == nil || (len(st.Logs) == 0 && len(st.Track) == 0) {
		return s.Clear()
	}

	st.V = 1
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := s.path + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(s.path)
		if err2 := os.Rename(tmp, s.path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func (c *Client) loadPersistedQueue() {
	if c == nil || c.queueStore == nil {
		return
	}
	st, err := c.queueStore.Load()
	if err != nil || st == nil {
		return
	}

	c.mu.Lock()
	if len(st.Logs) > 0 {
		c.logQueue = append([]LogPayload(nil), st.Logs...)
	}
	if len(st.Track) > 0 {
		c.trackQueue = append([]TrackEventPayload(nil), st.Track...)
	}
	if len(c.logQueue) > c.maxQueueSize {
		c.logQueue = append([]LogPayload(nil), c.logQueue[len(c.logQueue)-c.maxQueueSize:]...)
	}
	if len(c.trackQueue) > c.maxQueueSize {
		c.trackQueue = append([]TrackEventPayload(nil), c.trackQueue[len(c.trackQueue)-c.maxQueueSize:]...)
	}
	if st.FirstQueuedAt != nil && !st.FirstQueuedAt.IsZero() {
		c.firstQueuedAt = st.FirstQueuedAt.UTC()
	} else if len(c.logQueue)+len(c.trackQueue) > 0 && c.firstQueuedAt.IsZero() {
		c.firstQueuedAt = c.now().UTC()
	}
	c.mu.Unlock()
}

func (c *Client) schedulePersist() {
	if c == nil || c.queueStore == nil || c.persistDebounce <= 0 {
		if c != nil && c.queueStore != nil && c.persistDebounce == 0 {
			c.persistNow()
		}
		return
	}

	c.persistMu.Lock()
	defer c.persistMu.Unlock()

	if c.persistTimer != nil {
		_ = c.persistTimer.Reset(c.persistDebounce)
		return
	}
	c.persistTimer = time.AfterFunc(c.persistDebounce, func() {
		c.persistNow()
	})
}

func (c *Client) stopPersist() {
	if c == nil || c.queueStore == nil {
		return
	}
	c.persistMu.Lock()
	if c.persistTimer != nil {
		c.persistTimer.Stop()
		c.persistTimer = nil
	}
	c.persistMu.Unlock()
	c.persistNow()
}

func (c *Client) persistNow() {
	if c == nil || c.queueStore == nil {
		return
	}

	c.persistWriteMu.Lock()
	defer c.persistWriteMu.Unlock()

	c.persistMu.Lock()
	if c.persistTimer != nil {
		c.persistTimer.Stop()
		c.persistTimer = nil
	}
	c.persistMu.Unlock()

	var st persistedQueueState
	c.mu.Lock()
	st.Logs = append([]LogPayload(nil), c.logQueue...)
	st.Track = append([]TrackEventPayload(nil), c.trackQueue...)
	if !c.firstQueuedAt.IsZero() {
		t := c.firstQueuedAt.UTC()
		st.FirstQueuedAt = &t
	}
	c.mu.Unlock()

	_ = c.queueStore.Save(&st)
}
