# Context: Alert Rules + Channels

## Summary

Goal: add a configurable alerting system (rules + channels) for logtap.

Implemented:
- New DB schema/models: contacts, contact groups, webhook endpoints, rules, states, deliveries.
- Alert engine wired into ingest/consumer and testkit publisher (best-effort).
- Delivery worker supports: `wecom`, `webhook`, `email` (SMTP), `sms` (Aliyun/Tencent); worker start is gated by `ENABLE_ALERT_WORKER` (default false).
- REST APIs under `/api/:projectId/alerts/*` for managing contacts/groups/bots/endpoints/rules.
- Unit + integration tests added (integration verifies webhook delivery end-to-end).

## Decisions

- Use DB outbox (`alert_deliveries`) for retries and durability.
- Provide a centralized contact book (email/phone) + contact groups; rules reference groups/contacts.
- Treat generic webhook endpoints as a distinct channel from WeCom bot webhooks.
- Keep rule matching MVP: AND-only across conditions; extend later if needed.
- Support SMS providers by config: `aliyun`, `tencent`.

## Next Step

- Optional follow-ups: add OpenAPI entries for the new endpoints; add UI pages in console; add “rules test/dry-run” endpoint; add provider mocks for SMS.
