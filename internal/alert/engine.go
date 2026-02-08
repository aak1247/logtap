package alert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aak1247/logtap/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Engine struct {
	DB  *gorm.DB
	Now func() time.Time
}

func NewEngine(db *gorm.DB) *Engine {
	return &Engine{DB: db, Now: time.Now}
}

func (e *Engine) Evaluate(ctx context.Context, in Input) error {
	if e == nil || e.DB == nil {
		return nil
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
		return err
	}

	for _, r := range rules {
		rm := RuleMatch{}
		_ = json.Unmarshal(r.Match, &rm)
		if !matchRule(rm, in) {
			continue
		}

		rep := RuleRepeat{}
		_ = json.Unmarshal(r.Repeat, &rep)
		applyRepeatDefaults(&rep)

		targets := RuleTargets{}
		_ = json.Unmarshal(r.Targets, &targets)

		keyHash := computeDedupeKeyHash(r.ID, in, rep)

		shouldSend, err := e.updateStateAndDecide(ctx, r.ID, keyHash, now, rep)
		if err != nil {
			return err
		}
		if !shouldSend {
			continue
		}

		title, content := formatNotification(r, in)
		if err := e.enqueueDeliveries(ctx, r, targets, title, content, now); err != nil {
			return err
		}
	}

	return nil
}

func applyRepeatDefaults(r *RuleRepeat) {
	if r.WindowSec <= 0 {
		r.WindowSec = 60
	}
	if r.Threshold <= 0 {
		r.Threshold = 1
	}
	if r.DedupeByMessage == nil {
		v := true
		r.DedupeByMessage = &v
	}
	if r.BaseBackoffSec <= 0 {
		r.BaseBackoffSec = 60
	}
	if r.MaxBackoffSec <= 0 {
		r.MaxBackoffSec = 3600
	}
	if r.MaxBackoffSec < r.BaseBackoffSec {
		r.MaxBackoffSec = r.BaseBackoffSec
	}
}

func computeDedupeKeyHash(ruleID int, in Input, rep RuleRepeat) string {
	parts := []string{
		fmt.Sprintf("rule=%d", ruleID),
		fmt.Sprintf("src=%s", in.Source),
		fmt.Sprintf("lvl=%s", strings.TrimSpace(in.Level)),
	}
	if rep.DedupeByMessage == nil || *rep.DedupeByMessage {
		parts = append(parts, "msg="+normalizeLower(in.Message))
	}
	if len(rep.DedupeFields) > 0 {
		fields := append([]string(nil), rep.DedupeFields...)
		sort.Strings(fields)
		for _, p := range fields {
			v, ok := getByPath(in.Fields, p)
			if !ok {
				parts = append(parts, "f."+p+"=<missing>")
				continue
			}
			parts = append(parts, "f."+p+"="+normalizeLower(toString(v)))
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func (e *Engine) updateStateAndDecide(ctx context.Context, ruleID int, keyHash string, now time.Time, rep RuleRepeat) (bool, error) {
	window := time.Duration(rep.WindowSec) * time.Second

	state := model.AlertState{
		RuleID:        ruleID,
		KeyHash:       keyHash,
		Occurrences:   0,
		BackoffExp:    0,
		LastSeenAt:    now,
		LastSentAt:    time.Unix(0, 0).UTC(),
		NextAllowedAt: time.Unix(0, 0).UTC(),
	}

	if err := e.DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return false, err
	}

	var cur model.AlertState
	if err := e.DB.WithContext(ctx).
		Where("rule_id = ? AND key_hash = ?", ruleID, keyHash).
		First(&cur).Error; err != nil {
		return false, err
	}

	if !cur.LastSeenAt.IsZero() && now.Sub(cur.LastSeenAt) > window {
		cur.Occurrences = 0
		cur.BackoffExp = 0
	}
	cur.Occurrences++
	cur.LastSeenAt = now

	shouldSend := cur.Occurrences >= rep.Threshold && (cur.NextAllowedAt.IsZero() || !now.Before(cur.NextAllowedAt))
	if shouldSend {
		delay := time.Duration(rep.BaseBackoffSec) * time.Second
		if cur.BackoffExp > 0 {
			for i := 0; i < cur.BackoffExp; i++ {
				delay *= 2
				max := time.Duration(rep.MaxBackoffSec) * time.Second
				if delay >= max {
					delay = max
					break
				}
			}
		}
		cur.BackoffExp++
		cur.LastSentAt = now
		cur.NextAllowedAt = now.Add(delay)
	}

	if err := e.DB.WithContext(ctx).
		Model(&model.AlertState{}).
		Where("id = ?", cur.ID).
		Updates(map[string]any{
			"occurrences":     cur.Occurrences,
			"backoff_exp":     cur.BackoffExp,
			"last_seen_at":    cur.LastSeenAt,
			"last_sent_at":    cur.LastSentAt,
			"next_allowed_at": cur.NextAllowedAt,
		}).Error; err != nil {
		return false, err
	}

	return shouldSend, nil
}

func formatNotification(rule model.AlertRule, in Input) (title string, content string) {
	title = fmt.Sprintf("[logtap] Rule %s triggered", rule.Name)
	content = fmt.Sprintf(
		"project_id=%d source=%s level=%s message=%s",
		in.ProjectID,
		in.Source,
		strings.TrimSpace(in.Level),
		strings.TrimSpace(in.Message),
	)
	if len(in.Fields) > 0 {
		if b, err := json.Marshal(in.Fields); err == nil {
			content += "\nfields=" + string(b)
		}
	}
	return title, content
}

func (e *Engine) enqueueDeliveries(ctx context.Context, rule model.AlertRule, targets RuleTargets, title, content string, now time.Time) error {
	deliveries, err := buildDeliveries(ctx, e.DB.WithContext(ctx), rule.ProjectID, rule.ID, targets, title, content, now)
	if err != nil {
		return err
	}
	if len(deliveries) == 0 {
		return nil
	}
	return e.DB.WithContext(ctx).Create(&deliveries).Error
}

func buildDeliveries(ctx context.Context, db *gorm.DB, projectID int, ruleID int, targets RuleTargets, title, content string, now time.Time) ([]model.AlertDelivery, error) {
	out := make([]model.AlertDelivery, 0, 8)

	if len(targets.WecomBotIDs) > 0 {
		var bots []model.AlertWecomBot
		if err := db.WithContext(ctx).Where("project_id = ? AND id IN ?", projectID, targets.WecomBotIDs).Find(&bots).Error; err != nil {
			return nil, err
		}
		for _, b := range bots {
			out = append(out, model.AlertDelivery{
				ProjectID:     projectID,
				RuleID:        ruleID,
				ChannelType:   "wecom",
				Target:        b.WebhookURL,
				Title:         title,
				Content:       content,
				Status:        "pending",
				Attempts:      0,
				NextAttemptAt: now,
			})
		}
	}

	if len(targets.WebhookEndpointIDs) > 0 {
		var eps []model.AlertWebhookEndpoint
		if err := db.WithContext(ctx).Where("project_id = ? AND id IN ?", projectID, targets.WebhookEndpointIDs).Find(&eps).Error; err != nil {
			return nil, err
		}
		for _, ep := range eps {
			out = append(out, model.AlertDelivery{
				ProjectID:     projectID,
				RuleID:        ruleID,
				ChannelType:   "webhook",
				Target:        ep.URL,
				Title:         title,
				Content:       content,
				Status:        "pending",
				Attempts:      0,
				NextAttemptAt: now,
			})
		}
	}

	emailContacts, err := expandContacts(ctx, db, projectID, "email", targets.EmailContactIDs, targets.EmailGroupIDs)
	if err != nil {
		return nil, err
	}
	for _, c := range emailContacts {
		out = append(out, model.AlertDelivery{
			ProjectID:     projectID,
			RuleID:        ruleID,
			ChannelType:   "email",
			Target:        c.Value,
			Title:         title,
			Content:       content,
			Status:        "pending",
			Attempts:      0,
			NextAttemptAt: now,
		})
	}

	smsContacts, err := expandContacts(ctx, db, projectID, "sms", targets.SMSContactIDs, targets.SMSGroupIDs)
	if err != nil {
		return nil, err
	}
	for _, c := range smsContacts {
		out = append(out, model.AlertDelivery{
			ProjectID:     projectID,
			RuleID:        ruleID,
			ChannelType:   "sms",
			Target:        c.Value,
			Title:         title,
			Content:       content,
			Status:        "pending",
			Attempts:      0,
			NextAttemptAt: now,
		})
	}

	return out, nil
}

func expandContacts(ctx context.Context, db *gorm.DB, projectID int, typ string, contactIDs []int, groupIDs []int) ([]model.AlertContact, error) {
	ids := make(map[int]struct{}, len(contactIDs))
	for _, id := range contactIDs {
		if id > 0 {
			ids[id] = struct{}{}
		}
	}

	if len(groupIDs) > 0 {
		var groups []model.AlertContactGroup
		if err := db.WithContext(ctx).Where("project_id = ? AND type = ? AND id IN ?", projectID, typ, groupIDs).Find(&groups).Error; err != nil {
			return nil, err
		}
		validGroupIDs := make([]int, 0, len(groups))
		for _, g := range groups {
			validGroupIDs = append(validGroupIDs, g.ID)
		}
		if len(validGroupIDs) > 0 {
			var memberIDs []int
			if err := db.WithContext(ctx).
				Model(&model.AlertContactGroupMember{}).
				Select("distinct contact_id").
				Where("group_id IN ?", validGroupIDs).
				Scan(&memberIDs).Error; err != nil {
				return nil, err
			}
			for _, id := range memberIDs {
				if id > 0 {
					ids[id] = struct{}{}
				}
			}
		}
	}

	if len(ids) == 0 {
		return nil, nil
	}
	flat := make([]int, 0, len(ids))
	for id := range ids {
		flat = append(flat, id)
	}

	var contacts []model.AlertContact
	if err := db.WithContext(ctx).Where("project_id = ? AND type = ? AND id IN ?", projectID, typ, flat).Find(&contacts).Error; err != nil {
		return nil, err
	}
	return contacts, nil
}
