package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Envelope is a minimal parser for Sentry Envelope.
// Format (lines):
//  - envelope header JSON
//  - item header JSON
//  - item payload (may contain newlines, but most SDK payloads are single JSON line)
// For MVP we parse the first "event" item with JSON payload.
type Envelope struct {
	Header map[string]any
	Items  []EnvelopeItem
}

type EnvelopeItem struct {
	Header  map[string]any
	Payload []byte
}

func ParseEnvelope(body []byte) (Envelope, error) {
	r := bytes.NewReader(body)

	headerLine, err := readLine(r)
	if err != nil {
		return Envelope{}, err
	}
	headerLine = bytes.TrimSpace(headerLine)
	if len(headerLine) == 0 {
		return Envelope{}, errors.New("invalid envelope: empty header")
	}

	var header map[string]any
	if err := json.Unmarshal(headerLine, &header); err != nil {
		return Envelope{}, err
	}

	var items []EnvelopeItem
	for {
		itemHeaderLine, err := readLine(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return Envelope{}, err
		}
		itemHeaderLine = bytes.TrimSpace(itemHeaderLine)
		if len(itemHeaderLine) == 0 {
			continue
		}

		var itemHeader map[string]any
		if err := json.Unmarshal(itemHeaderLine, &itemHeader); err != nil {
			return Envelope{}, err
		}

		payload, err := readItemPayload(r, itemHeader)
		if err != nil {
			return Envelope{}, err
		}
		items = append(items, EnvelopeItem{Header: itemHeader, Payload: payload})
	}

	if len(items) == 0 {
		return Envelope{}, errors.New("invalid envelope: no items")
	}
	return Envelope{Header: header, Items: items}, nil
}

func (e Envelope) FirstEventJSON() (map[string]any, bool) {
	for _, it := range e.Items {
		typ, _ := it.Header["type"].(string)
		if typ != "event" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(it.Payload, &event); err != nil {
			continue
		}
		return event, true
	}
	return nil, false
}

func readLine(r *bytes.Reader) ([]byte, error) {
	var buf []byte
	for {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) && len(buf) > 0 {
				return buf, nil
			}
			return nil, err
		}
		if b == '\n' {
			return buf, nil
		}
		if b != '\r' {
			buf = append(buf, b)
		}
	}
}

func readItemPayload(r *bytes.Reader, itemHeader map[string]any) ([]byte, error) {
	length, hasLength := itemHeader["length"]
	if !hasLength {
		// Fallback: treat the next line as payload.
		line, err := readLine(r)
		if err != nil {
			return nil, err
		}
		return bytes.TrimSpace(line), nil
	}

	n, err := asInt64(length)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("invalid item length: %v", length)
	}
	if n == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	// Consume optional trailing newline(s).
	for {
		b, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return payload, nil
			}
			return nil, err
		}
		if b != '\n' && b != '\r' {
			_ = r.UnreadByte()
			return payload, nil
		}
	}
}

func asInt64(v any) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case json.Number:
		return x.Int64()
	default:
		return 0, fmt.Errorf("unsupported length type %T", v)
	}
}
