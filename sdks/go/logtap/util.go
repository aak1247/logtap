package logtap

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func normalizeBaseURL(baseURL string) (string, error) {
	s := strings.TrimSpace(baseURL)
	if s == "" {
		return "", errors.New("baseURL is required")
	}
	return strings.TrimRight(s, "/"), nil
}

func randomHex(bytes int) string {
	if bytes <= 0 {
		return ""
	}
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		// Fallback: crypto/rand should basically never fail, but avoid returning empty.
		return strings.Repeat("0", bytes*2)
	}
	return hex.EncodeToString(buf)
}

func newDeviceID() string {
	return "d_" + randomHex(16)
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func mergeStringMap(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func mergeAnyMap(a, b map[string]any) map[string]any {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func jsonSafe(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return fmt.Sprint(v)
	}
	return out
}

func jsonSafeAnyMap(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = jsonSafe(v)
	}
	return out
}
