package alert

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
)

type DeliveryPreview struct {
	ChannelType string `json:"channelType"`
	Target      string `json:"target"`
	Title       string `json:"title"`
	Content     string `json:"content"`
}

type RulePreview struct {
	RuleID   int    `json:"ruleId"`
	RuleName string `json:"ruleName"`

	Matched bool `json:"matched"`

	DedupeKeyHash string `json:"dedupeKeyHash,omitempty"`

	WindowSec   int `json:"windowSec,omitempty"`
	Threshold   int `json:"threshold,omitempty"`
	Occurrences int `json:"occurrences,omitempty"`

	OccurrencesBefore int `json:"occurrencesBefore,omitempty"`
	OccurrencesAfter  int `json:"occurrencesAfter,omitempty"`

	BackoffExpBefore int `json:"backoffExpBefore,omitempty"`
	BackoffExpAfter  int `json:"backoffExpAfter,omitempty"`

	NextAllowedAtBefore time.Time `json:"nextAllowedAtBefore,omitempty"`
	NextAllowedAtAfter  time.Time `json:"nextAllowedAtAfter,omitempty"`

	WindowExpired bool `json:"windowExpired,omitempty"`

	WillEnqueue       bool   `json:"willEnqueue,omitempty"`
	SuppressedReason  string `json:"suppressedReason,omitempty"` // threshold|backoff
	SuppressedMessage string `json:"suppressedMessage,omitempty"`

	Deliveries []DeliveryPreview `json:"deliveries,omitempty"`
}

func (e *Engine) EvaluatePreview(ctx context.Context, in Input) ([]RulePreview, error) {
	if e == nil || e.DB == nil {
		return nil, nil
	}
	now := e.Now().UTC()

	var rules []model.AlertRule
	q := e.DB.WithContext(ctx).
		Where("project_id = ? AND enabled = true", in.ProjectID)
	switch in.Source {
	case SourceLogs:
		q = q.Where("source IN ?", []string{string(SourceLogs), string(SourceBoth)})
	case SourceEvents:
		q = q.Where("source IN ?", []string{string(SourceEvents), string(SourceBoth)})
	default:
		q = q.Where("source IN ?", []string{string(SourceBoth), string(SourceLogs), string(SourceEvents)})
	}
	if err := q.Find(&rules).Error; err != nil {
		return nil, err
	}

	out := make([]RulePreview, 0, len(rules))
	for _, r := range rules {
		rm := RuleMatch{}
		_ = json.Unmarshal(r.Match, &rm)
		if !matchRule(rm, in) {
			out = append(out, RulePreview{
				RuleID:   r.ID,
				RuleName: r.Name,
				Matched:  false,
			})
			continue
		}

		rep := RuleRepeat{}
		_ = json.Unmarshal(r.Repeat, &rep)
		applyRepeatDefaults(&rep)

		targets := RuleTargets{}
		_ = json.Unmarshal(r.Targets, &targets)

		keyHash := computeDedupeKeyHash(r.ID, in, rep)
		prev, err := e.previewDecision(ctx, r, in, targets, rep, keyHash, now)
		if err != nil {
			return nil, err
		}
		out = append(out, prev)
	}

	return out, nil
}

func (e *Engine) previewDecision(ctx context.Context, rule model.AlertRule, in Input, targets RuleTargets, rep RuleRepeat, keyHash string, now time.Time) (RulePreview, error) {
	prev := RulePreview{
		RuleID:        rule.ID,
		RuleName:      rule.Name,
		Matched:       true,
		DedupeKeyHash: keyHash,
		WindowSec:     rep.WindowSec,
		Threshold:     rep.Threshold,
	}

	var cur model.AlertState
	err := e.DB.WithContext(ctx).Where("rule_id = ? AND key_hash = ?", rule.ID, keyHash).First(&cur).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return RulePreview{}, err
	}

	prev.OccurrencesBefore = cur.Occurrences
	prev.BackoffExpBefore = cur.BackoffExp
	prev.NextAllowedAtBefore = normalizeEpochZero(cur.NextAllowedAt)

	effective := cur
	if errors.Is(err, gorm.ErrRecordNotFound) {
		effective = model.AlertState{}
	}

	window := time.Duration(rep.WindowSec) * time.Second
	if !effective.LastSeenAt.IsZero() && now.Sub(effective.LastSeenAt) > window {
		prev.WindowExpired = true
		effective.Occurrences = 0
		effective.BackoffExp = 0
		effective.LastSentAt = time.Unix(0, 0).UTC()
		effective.NextAllowedAt = time.Unix(0, 0).UTC()
	}

	prev.OccurrencesAfter = effective.Occurrences + 1
	prev.Occurrences = prev.OccurrencesAfter

	willSend := prev.OccurrencesAfter >= rep.Threshold && (effective.NextAllowedAt.IsZero() || !now.Before(effective.NextAllowedAt))
	prev.WillEnqueue = willSend

	switch {
	case prev.OccurrencesAfter < rep.Threshold:
		prev.SuppressedReason = "threshold"
		prev.SuppressedMessage = "occurrences below threshold"
	case !effective.NextAllowedAt.IsZero() && now.Before(effective.NextAllowedAt):
		prev.SuppressedReason = "backoff"
		prev.SuppressedMessage = "within backoff period"
	}

	prev.BackoffExpAfter = effective.BackoffExp
	prev.NextAllowedAtAfter = normalizeEpochZero(effective.NextAllowedAt)

	if willSend {
		delay := time.Duration(rep.BaseBackoffSec) * time.Second
		if effective.BackoffExp > 0 {
			for i := 0; i < effective.BackoffExp; i++ {
				delay *= 2
				max := time.Duration(rep.MaxBackoffSec) * time.Second
				if delay >= max {
					delay = max
					break
				}
			}
		}
		prev.BackoffExpAfter = effective.BackoffExp + 1
		prev.NextAllowedAtAfter = now.Add(delay)

		title, content := formatNotification(rule, in)
		deliveries, err := buildDeliveries(ctx, e.DB.WithContext(ctx), rule.ProjectID, rule.ID, targets, title, content, now)
		if err != nil {
			return RulePreview{}, err
		}
		prev.Deliveries = make([]DeliveryPreview, 0, len(deliveries))
		for _, d := range deliveries {
			prev.Deliveries = append(prev.Deliveries, DeliveryPreview{
				ChannelType: d.ChannelType,
				Target:      d.Target,
				Title:       d.Title,
				Content:     d.Content,
			})
		}
	}

	return prev, nil
}

func normalizeEpochZero(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	epoch := time.Unix(0, 0).UTC()
	if t.UTC().Equal(epoch) {
		return time.Time{}
	}
	return t
}
