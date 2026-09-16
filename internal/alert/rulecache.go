package alert

import (
	"context"
	"encoding/json"
	"time"

	"github.com/aak1247/logtap/internal/channel"
	"github.com/aak1247/logtap/internal/model"
)

type ruleCacheKey struct {
	projectID int
	source    Source
}

// cachedRule holds a rule with its JSON blobs parsed once per cache load
// instead of once per evaluated message.
type cachedRule struct {
	rule     model.AlertRule
	match    RuleMatch
	repeat   RuleRepeat
	targets  RuleTargets
	channels []channel.ChannelConfig
}

type ruleSetCacheEntry struct {
	rules   []cachedRule
	expires time.Time
}

func (e *Engine) rulesFor(ctx context.Context, projectID int, source Source) ([]cachedRule, error) {
	key := ruleCacheKey{projectID: projectID, source: source}
	if e.RuleCacheTTL <= 0 {
		return e.loadRules(ctx, projectID, source)
	}
	now := e.Now()
	if v, ok := e.ruleCache.Load(key); ok {
		entry := v.(*ruleSetCacheEntry)
		if now.Before(entry.expires) {
			return entry.rules, nil
		}
	}
	rules, err := e.loadRules(ctx, projectID, source)
	if err != nil {
		// Serve stale rules rather than failing evaluation while the DB is
		// briefly unavailable; a stale set only delays rule changes.
		if v, ok := e.ruleCache.Load(key); ok {
			return v.(*ruleSetCacheEntry).rules, nil
		}
		return nil, err
	}
	e.ruleCache.Store(key, &ruleSetCacheEntry{rules: rules, expires: now.Add(e.RuleCacheTTL)})
	return rules, nil
}

func (e *Engine) loadRules(ctx context.Context, projectID int, source Source) ([]cachedRule, error) {
	var rules []model.AlertRule
	q := e.DB.WithContext(ctx).
		Where("project_id = ? AND enabled = true", projectID)
	switch source {
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

	out := make([]cachedRule, 0, len(rules))
	for _, r := range rules {
		cr := cachedRule{rule: r}
		_ = json.Unmarshal(r.Match, &cr.match)
		_ = json.Unmarshal(r.Repeat, &cr.repeat)
		applyRepeatDefaults(&cr.repeat)
		_ = json.Unmarshal(r.Targets, &cr.targets)
		cr.channels, _ = channel.ParseChannels(json.RawMessage(r.Targets))
		out = append(out, cr)
	}
	return out, nil
}
