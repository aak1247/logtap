package alert

import (
	"context"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/detector"
)

func InputFromSignal(s detector.Signal) Input {
	fields := map[string]any{}
	for key, value := range s.Fields {
		fields[key] = value
	}
	if len(s.Labels) > 0 {
		labels := map[string]any{}
		for key, value := range s.Labels {
			labels[key] = value
		}
		fields["labels"] = labels
	}
	if strings.TrimSpace(s.SourceType) != "" {
		fields["source_type"] = strings.TrimSpace(s.SourceType)
	}
	if strings.TrimSpace(s.Status) != "" {
		fields["signal_status"] = strings.TrimSpace(s.Status)
	}

	ts := s.OccurredAt.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	msg := strings.TrimSpace(s.Message)
	if msg == "" {
		msg = strings.TrimSpace(s.Title)
	}

	return Input{
		ProjectID: s.ProjectID,
		Source:    mapSignalSource(s.Source),
		Timestamp: ts,
		Level:     strings.TrimSpace(s.Severity),
		Message:   msg,
		Fields:    fields,
	}
}

func (e *Engine) EvaluateSignal(ctx context.Context, s detector.Signal) error {
	return e.Evaluate(ctx, InputFromSignal(s))
}

func mapSignalSource(raw string) Source {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(SourceLogs):
		return SourceLogs
	case string(SourceEvents):
		return SourceEvents
	default:
		return SourceBoth
	}
}
