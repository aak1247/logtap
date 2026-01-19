package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type APIEnvelope struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
	Err  string          `json:"err"`
}

func DoJSON(t testing.TB, client *http.Client, method, rawURL string, body any, headers map[string]string) (int, []byte) {
	t.Helper()

	var rd io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		rd = bytes.NewReader(buf)
	}

	req, err := http.NewRequest(method, rawURL, rd)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return res.StatusCode, b
}

func DecodeEnvelope(t testing.TB, body []byte) APIEnvelope {
	t.Helper()

	var env APIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, string(body))
	}
	return env
}

type BootstrapResult struct {
	Token      string
	ProjectID  int
	ProjectKey string
}

func Bootstrap(t testing.TB, client *http.Client, baseURL string) BootstrapResult {
	t.Helper()

	req := map[string]any{
		"email":        "owner@example.com",
		"password":     "pass123456",
		"project_name": "Default",
		"key_name":     "default",
	}
	status, body := DoJSON(t, client, http.MethodPost, baseURL+"/api/auth/bootstrap", req, nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", status, string(body))
	}
	env := DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("bootstrap code=%d err=%s", env.Code, env.Err)
	}

	var data struct {
		Token   string `json:"token"`
		Project struct {
			ID int `json:"id"`
		} `json:"project"`
		Key struct {
			Key string `json:"key"`
		} `json:"key"`
	}
	if err := json.Unmarshal(env.Data, &data); err != nil {
		t.Fatalf("bootstrap data: %v", err)
	}

	return BootstrapResult{
		Token:      data.Token,
		ProjectID:  data.Project.ID,
		ProjectKey: data.Key.Key,
	}
}

type SearchLogsParams struct {
	Q       string
	TraceID string
	Level   string
	Limit   int
}

func SearchLogs(t testing.TB, client *http.Client, baseURL, token string, projectID int, params SearchLogsParams) []map[string]any {
	t.Helper()

	usp := url.Values{}
	if strings.TrimSpace(params.Q) != "" {
		usp.Set("q", params.Q)
	}
	if strings.TrimSpace(params.TraceID) != "" {
		usp.Set("trace_id", params.TraceID)
	}
	if strings.TrimSpace(params.Level) != "" {
		usp.Set("level", params.Level)
	}
	if params.Limit > 0 {
		usp.Set("limit", fmt.Sprintf("%d", params.Limit))
	}

	rawURL := fmt.Sprintf("%s/api/%d/logs/search", baseURL, projectID)
	if qs := usp.Encode(); qs != "" {
		rawURL += "?" + qs
	}

	headers := map[string]string{"Authorization": "Bearer " + token}
	status, body := DoJSON(t, client, http.MethodGet, rawURL, nil, headers)
	if status != http.StatusOK {
		t.Fatalf("search logs status=%d body=%s", status, string(body))
	}
	env := DecodeEnvelope(t, body)
	if env.Code != 0 {
		t.Fatalf("search logs code=%d err=%s", env.Code, env.Err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("search logs data: %v", err)
	}
	return rows
}

// RepoRoot returns the repository root directory (computed from this package location).
func RepoRoot(t testing.TB) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	// file is .../internal/testkit/api.go => root is two levels up.
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
