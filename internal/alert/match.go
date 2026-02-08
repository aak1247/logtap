package alert

import (
	"encoding/json"
	"strconv"
	"strings"
)

func normalizeLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func containsAny(haystack string, needles []string) bool {
	h := normalizeLower(haystack)
	if h == "" {
		return false
	}
	for _, n := range needles {
		n = normalizeLower(n)
		if n == "" {
			continue
		}
		if strings.Contains(h, n) {
			return true
		}
	}
	return false
}

func stringInList(s string, list []string) bool {
	s = strings.TrimSpace(s)
	if s == "" || len(list) == 0 {
		return false
	}
	for _, it := range list {
		if strings.EqualFold(strings.TrimSpace(it), s) {
			return true
		}
	}
	return false
}

func parseJSONMap(raw []byte) map[string]any {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

func getByPath(m map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = m
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, false
		}
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[p]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func matchField(f FieldMatch, fields map[string]any) bool {
	switch f.Op {
	case OpExists:
		_, ok := getByPath(fields, f.Path)
		return ok
	case OpEquals:
		v, ok := getByPath(fields, f.Path)
		if !ok {
			return false
		}
		return normalizeLower(toString(v)) == normalizeLower(toString(f.Value))
	case OpContains:
		v, ok := getByPath(fields, f.Path)
		if !ok {
			return false
		}
		return strings.Contains(normalizeLower(toString(v)), normalizeLower(toString(f.Value)))
	case OpIn:
		v, ok := getByPath(fields, f.Path)
		if !ok {
			return false
		}
		vs := normalizeLower(toString(v))
		for _, it := range f.Values {
			if vs == normalizeLower(toString(it)) && vs != "" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func matchRule(m RuleMatch, in Input) bool {
	if len(m.Levels) > 0 && !stringInList(in.Level, m.Levels) {
		return false
	}

	// For track events we treat Input.Level == "event" and Input.Message == event name.
	if len(m.EventNames) > 0 {
		if !strings.EqualFold(strings.TrimSpace(in.Level), "event") {
			return false
		}
		if !stringInList(in.Message, m.EventNames) {
			return false
		}
	}

	if len(m.MessageKeywords) > 0 && !containsAny(in.Message, m.MessageKeywords) {
		return false
	}

	for _, f := range m.FieldsAll {
		if !matchField(f, in.Fields) {
			return false
		}
	}

	return true
}
