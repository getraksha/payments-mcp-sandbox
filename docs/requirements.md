# Payments MCP Sandbox Requirements

## 1. Purpose

`payments-mcp-sandbox` is a synthetic, enterprise-style Model Context Protocol
(MCP) service for demonstrating peer-to-peer payment workflows. It must behave
like a small production service while remaining completely isolated from real
money, payment rails, bank accounts, and external financial systems.

This document defines what the service must do. Implementation details and
technology choices are in `docs/implementation-plan.md`.

## 2. Scope

### In scope

- Manage synthetic payment recipients.
- Move synthetic funds from a sandbox funding account to recipient accounts.
- Persist recipients, transfers, balances, and ledger entries locally.
- Expose capabilities through MCP over deployable and local transports.
- Provide production-quality configuration, diagnostics, validation, error
  behavior, graceful shutdown, and tests.

### Out of scope

- Real money, payment processors, banking networks, financial APIs, or real
  payment credentials.
- End-user authentication, authorization, approval policy, or tenancy. Per the
  repository README, access governance belongs to an external system such as
  AGP.
- Reversals, refunds, recurring transfers, recipient deletion, foreign exchange,
  fees, or interest in the initial release.
- Horizontal multi-writer scaling; v1 is one process with an embedded database.
- A REST business API or graphical user interface.

## 3. Domain model and invariants

### Sandbox funding account

- The service owns one synthetic funding account seeded only during initial
  database creation.
- Currency and initial balance are configured before initialization; restarts
  must not reset existing state.
- Changing configured currency after initialization fails startup with a clear
  configuration/data mismatch.

### Recipient

A recipient has a server-generated opaque ID, caller-supplied unique external
reference, display name, synthetic payment handle, active status, timestamps,
and one synthetic account in the service currency. External references and
handles are unique after documented normalization. Duplicates return a stable
conflict instead of creating another recipient.

### Money

- Values are signed 64-bit integers in minor units; binary floating point is
  forbidden.
- V1 supports exactly one configured ISO 4217 currency.
- Transfer amounts are positive, bounded by configuration, checked for overflow,
  and cannot overdraw the funding account.

### Transfer and ledger

- A transfer moves funds from the sandbox funding account to one active
  recipient account.
- A successful transfer and balanced debit/credit ledger entries commit
  atomically; their sum is zero.
- Completed transfers are immutable. Rejections never partially alter balances
  or ledger data.
- Every transfer has a required caller idempotency key. Reuse with identical
  normalized input returns the original result; reuse with different input
  returns an idempotency conflict.
- V1 completes valid transfers synchronously. Status supports `completed` and
  `failed`; the schema may reserve `pending` without adding a worker in v1.
- A failed transfer record is persisted only after a syntactically valid,
  idempotent request reaches domain processing. Validation and lookup errors do
  not invent ledger activity.

## 4. MCP tools

Names are stable snake_case identifiers. Descriptions and schemas state that all
data and funds are synthetic.

### `add_recipient`

Input: required, bounded `external_reference`, `display_name`, and
`payment_handle` strings.

- Validate and normalize before persistence.
- Atomically create a recipient and zero-balance synthetic account.
- Return ID, normalized fields, status, currency, and timestamps.
- Return stable conflicts for duplicate references or handles.

### `list_recipients`

Input: optional opaque `cursor` and bounded `limit` with documented defaults.

- Return a deterministic cursor-paginated list, or an empty list when none exist.
- Include a next cursor only when another page exists.
- Reject malformed or unsupported cursors without exposing their representation.

### `transfer_money`

Input: required `recipient_id`, integer `amount_minor`, `currency`, and bounded
`idempotency_key`; optional bounded `note`.

- Enforce all money, recipient, balance, and idempotency invariants.
- Commit balance movement and double-entry ledger rows atomically.
- Return transfer/recipient IDs, amount, currency, status, timestamps, and a
  caller-safe failure object when applicable.
- Never echo idempotency keys or notes in logs or metrics.

### `list_transactions`

Input: optional opaque `cursor`, bounded `limit`, `recipient_id` filter, and
`status` filter.

- Return transfer summaries in deterministic newest-first order.
- Apply filters before pagination and return caller-facing transfer data, not
  internal ledger rows.
- Preserve stable boundaries when transfers share a timestamp.

### `check_transfer_status`

Input: required `transfer_id`.

- Return the current transfer summary and safe failure code when present.
- Return stable not-found for an unknown ID.
- Remain read-only and retry-safe.

## 5. Common interface behavior

### Validation

- MCP schemas reject incorrect primitive types and missing fields.
- The application independently enforces normalization, length, character,
  range, currency, and cross-field rules.
- Unknown fields are rejected where supported.
- Limits/defaults are documented in schemas and boundary-tested.

### Errors

- Expected failures use a consistent object with `code`, safe `message`,
  `retryable`, and optional field-level `details`.
- Stable codes cover invalid input, not found, duplicate, idempotency conflict,
  insufficient funds, currency mismatch, unavailable, and internal failure.
- Domain failures are unsuccessful MCP tool results; protocol-level failures are
  reserved for malformed MCP/JSON-RPC or inability to produce a tool result.
- Callers never receive raw database errors, SQL, stack traces, secrets, or paths.

### Pagination

- Cursors are versioned, opaque, URL-safe, integrity-checked, and bound to the
  originating filters.
- Ordering uses timestamp plus unique ID as a tie-breaker.

## 6. Configuration

- Environment variables provide configuration with documented defaults and an
  example file; flags may override non-secret operational settings.
- Settings include transport, MCP and diagnostics binds, database path, currency,
  initial balance, maximum transfer, page limits, log level/format, request and
  shutdown timeouts, cursor-signing secret, and telemetry exporters.
- Invalid, conflicting, missing, or unsafe values fail before requests are
  accepted. Secrets are never logged.
- HTTP binds to loopback by default; non-loopback requires explicit opt-in because
  the service intentionally has no identity layer.
- Persistent domain settings are verified against initialized database metadata.

## 7. Transport and lifecycle

- Streamable HTTP is the primary deployable transport at `/mcp`; stdio is
  supported for local clients.
- In stdio mode stdout is protocol-only and logs use stderr.
- HTTP applies header/read/write/idle/body/concurrency limits and allowed origin/
  host checks where relevant.
- Cancellation and deadlines propagate through service and database layers.
- Startup validates config, opens/migrates/verifies the database, and initializes
  telemetry before readiness.
- On termination, stop accepting work, drain requests, flush telemetry, close the
  database, and exit within a configured deadline.

## 8. Persistence and reliability

- Use local SQLite with foreign keys, a busy timeout, and WAL where supported.
- Versioned migrations are embedded and applied transactionally; migration
  failure prevents readiness.
- Constraints enforce uniqueness, references, statuses, amounts, and integrity
  in addition to application validation.
- Writes use explicit transactions and bounded contexts.
- Lock, corruption, permissions, and disk failures produce actionable safe errors
  without silent loss.
- Operating docs explain safe backup/restore; operators own backups.

## 9. Logging, observability, and operations

- Structured logs contain timestamp, level, service/version, request ID, tool,
  duration, outcome, and stable error code.
- Logs omit names, handles, notes, idempotency keys, cursor contents, and request
  bodies by default.
- Bounded-cardinality metrics cover request counts/duration, active requests,
  domain failures, database latency, transfers, and shutdown outcomes; entity IDs
  never become labels.
- Optional distributed traces cover transport, tool, service, and database spans
  without payloads.
- A separate diagnostics listener provides liveness, readiness, and optional
  metrics. Readiness remains false until startup and fails if storage is unusable.
- Telemetry failures cannot corrupt payment state. Build version/commit/Go
  version are visible through safe startup/version diagnostics.

## 10. Security

- The no-authentication boundary is explicit; network defaults are local-only.
- Remote deployment requires an external trusted access-control and TLS layer.
- Inputs, cursors, request bodies, pages, timeouts, and concurrency are bounded.
- New database files use owner-only permissions where supported.
- Dependencies are pinned; vulnerability/license scanning is CI policy.
- Synthetic-only warnings appear in README, tool descriptions, and startup logs.

## 11. Testing and quality

- Unit tests cover invariants, normalization, arithmetic, idempotency, errors,
  pagination, and config.
- SQLite integration tests cover migrations, constraints, rollback, concurrent
  transfers, restart persistence, and retries.
- MCP contract tests cover discovery, schemas, calls, errors, cancellation, and
  both transports where practical.
- Tests use temporary databases and deterministic clocks/IDs; no network or
  financial dependency is required.
- Race detection, static analysis, formatting, tests, and migration checks run in
  CI. Fuzz tests target cursor decoding and risky normalization/parsing.

## 12. Acceptance criteria

1. All five tools are discoverable and conform to this document.
2. A recipient can be created, listed, paid, found in history, and queried by
   transfer ID across a restart.
3. Duplicate retries cannot move funds twice, including concurrently.
4. Invalid/failed requests leave no partial balances or ledger rows.
5. HTTP and stdio startup, cancellation, and graceful shutdown are verified.
6. Logs, metrics, traces, and health signals show outcomes without payload leaks
   or unbounded identifiers.
7. Configuration/persistent-state mismatches fail before readiness.
8. All checks pass without real network or financial systems.

## 13. Assumptions and open questions

### Assumptions

- Transfers use one service-owned funding account; callers do not select a source.
- One process owns one database file and one currency.
- Valid transfers complete synchronously; status supports retry recovery.
- External systems protect deployments beyond localhost.

### Open questions

1. Should v1 expose the funding account's remaining synthetic balance?
2. Should `add_recipient` require a separate idempotency key, or are its unique
   fields sufficient for retries?
3. What are the default currency, initial balance, and maximum transfer amount?
4. Is synchronous completion preferred, or should a later phase model durable
   pending settlement?
5. Which MCP protocol versions must be supported at launch?
