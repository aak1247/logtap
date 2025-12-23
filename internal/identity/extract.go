package identity

import (
	"fmt"
	"strings"
	"time"
)

func ExtractUserID(event map[string]any) string {
	return extractUserIDFromAny(event["user"])
}

func ExtractDeviceID(event map[string]any) string {
	// Prefer tags.device_id (easy for SDKs).
	if v := extractTagValue(event["tags"], "device_id"); v != "" {
		return v
	}
	// Fallback to contexts.device.id
	if v := extractContextDeviceID(event["contexts"]); v != "" {
		return v
	}
	// Some SDKs might send device_id in extra/contexts directly.
	if v := extractTagValue(event["extra"], "device_id"); v != "" {
		return v
	}
	return ""
}

func ExtractDistinctID(event map[string]any) (distinctID string, source string) {
	if v := ExtractUserID(event); v != "" {
		return v, "user.id"
	}
	if v := ExtractDeviceID(event); v != "" {
		return v, "device_id"
	}
	return "", ""
}

func ExtractOS(event map[string]any) string {
	ctxs, ok := event["contexts"].(map[string]any)
	if !ok {
		return ""
	}
	osCtx, ok := ctxs["os"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := osCtx["name"].(string)
	version, _ := osCtx["version"].(string)
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	switch {
	case name != "" && version != "":
		return fmt.Sprintf("%s %s", name, version)
	case name != "":
		return name
	default:
		return ""
	}
}

func ExtractBrowser(event map[string]any) string {
	ctxs, ok := event["contexts"].(map[string]any)
	if !ok {
		return ""
	}
	br, ok := ctxs["browser"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := br["name"].(string)
	version, _ := br["version"].(string)
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	switch {
	case name != "" && version != "":
		return fmt.Sprintf("%s %s", name, version)
	case name != "":
		return name
	default:
		return ""
	}
}

func ExtractTimestamp(event map[string]any, fallback time.Time) time.Time {
	// Keep logic close to store parsing; used for metrics bucketing.
	if fallback.IsZero() {
		fallback = time.Now().UTC()
	}
	switch t := event["timestamp"].(type) {
	case string:
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts.UTC()
		}
		if ts, err := time.Parse(time.RFC3339, t); err == nil {
			return ts.UTC()
		}
	case float64:
		sec := int64(t)
		nsec := int64((t - float64(sec)) * 1e9)
		return time.Unix(sec, nsec).UTC()
	case int64:
		return time.Unix(t, 0).UTC()
	}
	return fallback.UTC()
}

func extractUserIDFromAny(v any) string {
	user, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if id, _ := user["id"].(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	if username, _ := user["username"].(string); strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}
	if email, _ := user["email"].(string); strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	return ""
}

func extractTagValue(tagsAny any, key string) string {
	key = strings.TrimSpace(key)
	if key == "" || tagsAny == nil {
		return ""
	}

	// Sentry tags can be sent as an object or as array of [k,v] pairs depending on SDK.
	if m, ok := tagsAny.(map[string]any); ok {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	if arr, ok := tagsAny.([]any); ok {
		for _, it := range arr {
			pair, ok := it.([]any)
			if !ok || len(pair) < 2 {
				continue
			}
			k, _ := pair[0].(string)
			if k != key {
				continue
			}
			switch v := pair[1].(type) {
			case string:
				return strings.TrimSpace(v)
			default:
				return strings.TrimSpace(fmt.Sprint(v))
			}
		}
	}
	return ""
}

func extractContextDeviceID(contextsAny any) string {
	ctxs, ok := contextsAny.(map[string]any)
	if !ok {
		return ""
	}
	dev, ok := ctxs["device"].(map[string]any)
	if !ok {
		return ""
	}
	if id, _ := dev["id"].(string); strings.TrimSpace(id) != "" {
		return strings.TrimSpace(id)
	}
	return ""
}
