# Tasks: Alert Rules + Channels

## Phase 0 — Design Gate

- [x] Confirm existing partial implementation compiles
- [x] Confirm API surface and JSON schemas
- [x] Confirm delivery configuration via env vars
- [x] User approval to proceed with implementation

## Phase 1 — Backend APIs

- [x] Implement CRUD for contacts (email/sms)
- [x] Implement CRUD for contact groups + membership
- [x] Implement CRUD for WeCom bots
- [x] Implement CRUD for webhook endpoints
- [x] Implement CRUD for alert rules (+ validation)
- [x] Wire routes in `internal/httpserver/httpserver.go`
- [x] Extend `internal/openapi/openapi.go`

## Phase 2 — Runtime Wiring

- [x] Add missing config fields to `internal/config/config.go`
- [x] Wire alert evaluation into consumer + testkit publisher
- [x] Start delivery worker from gateway main (behind safe default)
- [x] Implement SMS providers: aliyun + tencent (worker selection by `SMS_PROVIDER`)
- [x] Implement webhook channel (distinct from WeCom)

## Phase 3 — Tests

- [x] Unit tests: matcher (keywords/fields/array path)
- [x] Unit tests: engine (threshold/backoff)
- [x] Unit tests: worker (wecom/webhook via httptest)
- [x] Integration test: enqueue + send -> sent

## Phase 4 — Docs

- [x] Document env vars and example rule payloads
