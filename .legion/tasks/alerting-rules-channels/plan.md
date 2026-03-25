# Alert Rules + Alert Channels (logtap)

## Goals

- Provide a configurable alerting system for each project.
- Support rules triggered by:
  - event name (for track events)
  - message keywords (for logs/events)
  - field conditions (JSON path)
- Support repeat-alert controls:
  - repeat threshold within a time window
  - dedupe key controls: by message and/or by selected fields
  - exponential backoff for repeated alerts for the same dedupe key
- Support alert channels:
  - email contacts / groups (contact book maintained separately; rules select groups/contacts)
  - SMS contacts / groups (contact book maintained separately; rules select groups/contacts)
  - WeCom bot webhooks (rules select bots)
  - generic webhook endpoints (rules select endpoints)
- Support retry / re-delivery:
  - durable outbox table
  - worker performs delivery with retry backoff

## Non-goals (for this iteration)

- UI pages for managing rules/channels (API-first).
- Complex boolean logic (OR groups / nested expressions) in rule match conditions.
- Persistent client-side queueing beyond DB outbox (e.g. Kafka).
- Multi-tenant admin controls beyond existing project-owner auth.

## Current Status (already implemented in codebase)

- DB tables/models (current draft implementation; will be adjusted per updated requirements below):
  - `alert_recipient_groups`, `alert_recipients`, `alert_wecom_bots`
  - `alert_rules` (jsonb: `match`, `repeat`, `targets`)
  - `alert_states` (dedupe/backoff state)
  - `alert_deliveries` (outbox, retry)
- Rule matching + dedupe/backoff decision engine in `internal/alert/engine.go` + `match.go`.
- Delivery worker in `internal/alert/worker.go` (WeCom webhook, SMS via webhook, SMTP email) but **needs config fields**.

## Proposed Technical Design

### Data Model (already present)

- `alert_rules.source`: `logs|events|both`
- `alert_rules.match` JSON:
  - `levels?: string[]`
  - `eventNames?: string[]` (only applies to `level == "event"` logs)
  - `messageKeywords?: string[]`
  - `fieldsAll?: FieldMatch[]`
- `alert_rules.repeat` JSON:
  - `windowSec: number` (default 60)
  - `threshold: number` (default 1)
  - `dedupeByMessage: boolean` (default true)
  - `dedupeFields: string[]` (default empty)
  - `baseBackoffSec: number` (default 60)
  - `maxBackoffSec: number` (default 3600)
- `alert_rules.targets` JSON:
  - `emailGroupIds?: number[]`
  - `emailContactIds?: number[]`
  - `smsGroupIds?: number[]`
  - `smsContactIds?: number[]`
  - `wecomBotIds?: number[]`
  - `webhookEndpointIds?: number[]`

### Contacts + Groups (updated requirement)

Replace the “group contains raw values” model with a centralized contact book:

- `alert_contacts`:
  - `id`, `project_id`, `type=email|sms`, `name`, `value` (email/phone), timestamps
  - sms `value` format: E.164 (e.g. `+8613800138000`)
  - uniqueness: `(project_id, type, value)`
- `alert_contact_groups`:
  - `id`, `project_id`, `type=email|sms`, `name`, timestamps
  - uniqueness: `(project_id, type, name)`
- `alert_contact_group_members`:
  - `group_id`, `contact_id`
  - uniqueness: `(group_id, contact_id)`

Rules select groups and/or individual contacts; the engine expands to outbox rows.

### Webhook endpoints (updated requirement)

Webhook is a distinct alert channel (different from WeCom bot webhook):

- `alert_webhook_endpoints`:
  - `id`, `project_id`, `name`, `url`, timestamps
  - uniqueness: `(project_id, name)`

Delivery payload is a fixed JSON envelope:
- `title`, `content`, `projectId`, `ruleId`, `createdAt`, plus original matched input (`source/level/message/fields`).

No signing in this iteration (URL-only).

### JSON path support

- Path syntax: dot-separated segments, plus optional array index segment:
  - `user.id`
  - `exception.values.0.type`
- Implementation change: extend `getByPath()` to support numeric segments for arrays (`[]any`).

### Repeat / Dedup / Backoff

- Dedupe key hash = rule + source + level + (message if enabled) + (selected fields if configured).
- For each `rule_id + key_hash`, maintain state:
  - `occurrences` within a rolling window (reset if silence > windowSec).
  - `next_allowed_at` gates sending; after each send, exponential delay grows up to max.
- Decision:
  - only enqueue deliveries when `occurrences >= threshold` and `now >= next_allowed_at`.

### Delivery Outbox Worker

- Poll `alert_deliveries` where `status=pending` and `next_attempt_at <= now`.
- Delivery types:
  - `wecom`: POST to WeCom bot `webhook_url` (managed resources).
  - `webhook`: POST to generic webhook endpoint `url` (managed resources; optional signing).
  - `sms`: send via provider (Aliyun/Tencent) using configured credentials + template.
  - `email`: SMTP via `SMTP_HOST/PORT` + optional auth.
- Reliability:
  - transient failures retry with exponential backoff.
  - config-missing errors treated as permanent (mark `failed` immediately) to avoid noisy retry loops.

### Ingestion Hook (engine evaluation)

- After a log/event is successfully stored, run `alert.Engine.Evaluate()` best-effort.
- Hook points:
  - `internal/consumer/consumer.go`: after `store.InsertLog` / `store.InsertEvent`.
  - `internal/testkit/publisher.go`: after direct `store.InsertLog` / `store.InsertEvent` so integration tests cover alerting even without NSQ.
- Timeouts:
  - keep alert evaluation short (e.g. `context.WithTimeout(..., 500ms)`).

### HTTP APIs (project-owner protected)

Under `/api/:projectId/alerts/...`:

**Contacts**
- `GET /contacts?type=email|sms`
- `POST /contacts` body: `{type,name,value}`
- `PUT /contacts/:contactId` body: `{name,value}`
- `DELETE /contacts/:contactId`

**Contact groups**
- `GET /contact-groups?type=email|sms`
- `POST /contact-groups` body: `{type,name,memberContactIds:[...]}`
- `PUT /contact-groups/:groupId` body: `{name,memberContactIds:[...]}`
- `DELETE /contact-groups/:groupId`

**WeCom bots**
- `GET /wecom-bots`
- `POST /wecom-bots` body: `{name,webhookUrl}`
- `PUT /wecom-bots/:botId` body: `{name,webhookUrl}`
- `DELETE /wecom-bots/:botId`

**Webhook endpoints**
- `GET /webhook-endpoints`
- `POST /webhook-endpoints` body: `{name,url,secret?}`
- `PUT /webhook-endpoints/:endpointId` body: `{name,url,secret?}`
- `DELETE /webhook-endpoints/:endpointId`

**Alert rules**
- `GET /rules`
- `POST /rules` body: `{name,enabled,source,match,repeat,targets}`
- `PUT /rules/:ruleId` body: `{name,enabled,source,match,repeat,targets}`
- `DELETE /rules/:ruleId`
- (optional) `POST /rules/test` body: `{source,level,message,fields}` → returns which rules match and whether it would enqueue

**Deliveries (ops visibility, optional)**
- `GET /deliveries?status=pending|sent|failed&limit=...`

### Configuration (env)

Add to `internal/config/config.go`:
- `SMTP_HOST`, `SMTP_PORT` (default 587), `SMTP_FROM`, `SMTP_USERNAME`, `SMTP_PASSWORD`
- `SMS_PROVIDER=aliyun|tencent` (no default; SMS channel disabled if unset)
- Aliyun SMS:
  - `ALIYUN_SMS_ACCESS_KEY_ID`, `ALIYUN_SMS_ACCESS_KEY_SECRET`
  - `ALIYUN_SMS_REGION` (default `cn-hangzhou`)
  - `ALIYUN_SMS_SIGN_NAME`, `ALIYUN_SMS_TEMPLATE_CODE`
  - Template params mapping (first iteration): send JSON params `{ "title": "...", "content": "..." }`
- Tencent SMS:
  - `TENCENT_SMS_SECRET_ID`, `TENCENT_SMS_SECRET_KEY`
  - `TENCENT_SMS_APP_ID`
  - `TENCENT_SMS_SIGN_NAME`, `TENCENT_SMS_TEMPLATE_ID`
  - Template params mapping (first iteration): send params array `["<title>", "<content>"]`

Worker runtime switch:
- `ENABLE_ALERT_WORKER=true|false` (default `false`)

### Tests

- Unit tests:
  - `internal/alert/match_test.go` (keywords, field ops, array path)
  - `internal/alert/engine_test.go` (threshold + backoff gating)
  - `internal/alert/worker_test.go` (wecom + webhook via `httptest`, permanent vs transient; SMS signing logic unit-tested without real cloud calls)
- Integration tests:
  - `task test:it` extends to include an alerting scenario:
    - create project + rule + wecom bot
    - ingest a log/event via test server
    - assert `alert_deliveries` enqueued
    - run worker once against `httptest` endpoint and assert delivery becomes `sent`

## Files to Change (planned)

- `internal/config/config.go` (add alert delivery env config)
- `internal/alert/match.go` (array index path support)
- `internal/alert/worker.go` (permanent error handling, robustness, SMS providers, webhook channel)
- `internal/consumer/consumer.go` (wire in alert engine)
- `internal/testkit/publisher.go` (wire in alert engine for tests)
- `internal/query/alerts.go` (new REST handlers)
- `internal/httpserver/httpserver.go` (route wiring)
- `internal/openapi/openapi.go` (document new endpoints)
- `internal/alert/*_test.go` + integration test adjustments
- `docs/*` (API examples / env vars)

## Design Self-check (gate)

- ✅ Requirements mapped to data model + APIs.
- ✅ Dedupe conditions cover message + selected fields.
- ✅ Exponential backoff is per-rule+dedupe-key and stateful.
- ✅ Delivery is durable (DB outbox) and retriable.
- ✅ Tests cover match/backoff/delivery and one end-to-end path.
- ⚠️ Out-of-scope: UI, complex boolean expressions, third-party SMS providers (webhook adapter used).
