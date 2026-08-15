# Payments MCP Sandbox Requirements

## 1. Purpose

`payments-mcp-sandbox` is a synthetic, enterprise-style Model Context Protocol
(MCP) service for demonstrating peer-to-peer payment workflows. It should behave
like a small production service while remaining completely isolated from real
money, payment rails, bank accounts, and external financial systems.

This document defines what the service must do. Implementation details and
technology choices are in `docs/implementation-plan.md`.

## 2. Scope

### In scope

- Manage synthetic recipients identified by unique email addresses.
- Maintain one implicit synthetic funding account with a single USD balance.
- Deposit synthetic funds into that balance.
- Record synthetic transfers from the funding balance to recipients.
- Persist recipients, deposits, transfers, and the current balance locally.
- Expose capabilities through MCP over deployable and local transports.
- Provide production-quality configuration, structured logging, validation,
  error behavior, health/readiness, graceful shutdown, and tests.

### Out of scope

- Real money, payment processors, banking networks, financial APIs, or real
  payment credentials.
- Recipient accounts or balances, multiple funding accounts, configurable
  currencies, foreign exchange, and double-entry accounting.
- End-user authentication, authorization, approval policy, or tenancy. Per the
  repository README, access governance belongs to an external system such as
  AGP.
- Reversals, refunds, recurring transfers, recipient updates/deletion, fees, or
  interest in the initial release.
- Horizontal multi-writer scaling; v1 is one process with an embedded database.
- A REST business API, graphical interface, metrics, or distributed tracing.

## 3. Domain model and invariants

### Funding balance

- The service has exactly one implicit synthetic funding account represented by
  one persisted balance, not a general account model.
- The currency is always USD and is not configurable.
- The balance starts at zero on first database initialization. Restarts must not
  reset it.
- `deposit_money` increases the balance and `transfer_money` decreases it.
- The balance must never be negative and all changes are transactional.

### Recipient

- A recipient contains only a server-generated opaque ID, a unique normalized
  email address, and creation/update timestamps.
- Email is the caller-facing recipient identifier for transfer and filtering
  operations; callers do not need to retain the internal ID.
- Email normalization trims surrounding whitespace and lowercases the address.
- Invalid or duplicate normalized email addresses return stable validation or
  conflict errors.
- V1 has no external reference, display name, payment handle, status, account, or
  recipient balance.

### Money operations

- Monetary values are signed 64-bit integers in USD minor units (cents); binary
  floating point is forbidden.
- Deposit and transfer amounts must be positive, bounded by configured operation
  limits, and checked for overflow.
- A transfer must not reduce the funding balance below zero.
- A completed transfer records the recipient email and amount but does not credit
  a recipient account or create ledger entries.
- Deposits and transfers require caller-provided idempotency keys. Reusing a key
  for the same operation with identical normalized input returns the original
  result; reusing it with different input returns an idempotency conflict.
- Valid operations complete synchronously. Rejected operations do not create a
  deposit/transfer record or modify the balance.

## 4. MCP tool requirements

Tool names are stable snake_case identifiers. Descriptions and schemas must state
that all entities and funds are synthetic.

### `add_recipient`

Input: required `email` string.

- Validate and normalize the email.
- Create one recipient when the normalized email is unique.
- Return the server-generated ID, normalized email, and timestamps.
- Return a duplicate error when the email already exists; the existing unique
  email provides safe retry semantics without another idempotency key.

### `list_recipients`

Input: optional non-negative `offset` and bounded `limit`, with documented
defaults.

- Return recipients in deterministic creation order.
- Return an empty list, not an error, when none exist.
- Return the next offset only when another page exists.

### `deposit_money`

Input: required integer `amount_minor` and bounded `idempotency_key`.

- Atomically record the deposit and increase the funding balance.
- Return deposit ID, amount, fixed `USD` currency, resulting balance, and
  timestamp.
- An idempotent retry must not increase the balance twice.

### `get_current_account_balance`

Input: none.

- Return the current synthetic funding balance and fixed `USD` currency.
- Be read-only and safe to retry.

### `transfer_money`

Input: required `recipient_email`, integer `amount_minor`, and bounded
`idempotency_key`; optional bounded `note`.

- Normalize and resolve the recipient by email.
- Reject unknown recipients, non-positive/oversized amounts, and insufficient
  funds.
- Atomically record the transfer and decrease the funding balance.
- Return transfer ID, recipient ID/email, amount, fixed `USD` currency,
  `completed` status, resulting balance, and timestamp.
- Never log idempotency keys or notes.

### `list_transactions`

Input: optional non-negative `offset`, bounded `limit`, and normalized
`recipient_email` filter.

- Return transfer summaries in deterministic newest-first order.
- Apply the recipient filter before pagination.
- Return recorded transfers only; there are no ledger rows or recipient balances.
- Return the next offset only when another page exists.

### `check_transfer_status`

Input: required `transfer_id`.

- Return the recorded transfer summary and its `completed` status.
- Return a stable not-found error for an unknown ID.
- Be read-only and safe to retry.

## 5. Common interface behavior

### Validation

- MCP schemas reject incorrect primitive types and missing required fields.
- Application code independently enforces email normalization/format, string
  lengths, amount ranges, offsets, limits, and cross-field rules.
- Unknown input fields should be rejected where supported.
- Limits and defaults are documented in tool schemas and boundary-tested.

### Errors

- Expected failures use a consistent structured payload with `code`, a safe
  `message`, `retryable`, and optional field-level `details`.
- Stable initial codes cover invalid input, not found, duplicate email,
  idempotency conflict, insufficient funds, unavailable storage, and internal
  failure.
- Expected domain failures are unsuccessful MCP tool results; protocol-level
  failures are reserved for malformed MCP/JSON-RPC or inability to produce a
  tool result.
- Callers never receive database errors, SQL, stack traces, secrets, or paths.

### Pagination

- List tools use straightforward `limit`/`offset` pagination.
- Limits and offsets are bounded and validated.
- Ordering uses creation timestamp plus unique ID as a deterministic tie-breaker.

## 6. Configuration

- Environment variables provide configuration with documented defaults and an
  example file; flags may override non-secret operational settings.
- Settings include transport mode, MCP and diagnostics bind addresses, database
  path, maximum deposit/transfer amount, pagination limits, Zap log level/format,
  request timeout, and shutdown timeout.
- Currency is fixed to USD and initial balance is fixed to zero; neither is a
  configuration setting.
- Invalid, conflicting, missing, or unsafe settings fail before requests are
  accepted. Secrets are never logged.
- HTTP binds to loopback by default; non-loopback requires explicit opt-in because
  the service intentionally has no identity layer.

## 7. Transport and lifecycle

- Streamable HTTP is the primary deployable MCP transport at `/mcp`; stdio is
  supported for local clients.
- In stdio mode stdout is protocol-only and Zap logs use stderr.
- HTTP applies header/read/write/idle/body/concurrency limits and allowed origin/
  host checks where relevant.
- Cancellation and deadlines propagate through service and database layers.
- Startup validates configuration, opens and migrates SQLite, and initializes
  tools before readiness.
- On termination, the process stops accepting work, drains active requests,
  closes SQLite, syncs logs, and exits within a configured deadline.

## 8. Persistence and reliability

- Use local SQLite with foreign keys, a busy timeout, and WAL where supported.
- Versioned migrations are embedded and applied transactionally; migration
  failure prevents readiness.
- Database constraints enforce unique recipient email, unique operation-scoped
  idempotency keys, valid positive amounts, and a non-negative funding balance.
- Balance changes and their deposit/transfer records use explicit transactions.
- Lock, corruption, permission, and disk failures produce actionable safe errors
  without silently losing state.
- Operating documentation explains safe backup/restore; operators own backups.

## 9. Structured logging and operations

- Use Zap structured logging.
- Logs include timestamp, level, service/version, request ID, MCP tool name,
  duration, outcome, and stable error code when applicable.
- Logs omit email addresses, notes, idempotency keys, request bodies, and other
  caller-provided content by default.
- HTTP mode exposes basic liveness and readiness endpoints on a separately
  configurable diagnostics listener. Readiness remains false until startup
  completes and fails when SQLite is unusable.
- Build version, commit, and Go version are present in safe startup logs and a
  non-sensitive version response or endpoint.
- V1 does not include metrics, tracing, OpenTelemetry, or telemetry exporters.

## 10. Security

- The no-authentication boundary is explicit; network defaults are local-only.
- Remote deployment requires an external trusted access-control and TLS layer.
- Inputs, request bodies, pages, timeouts, and concurrent requests are bounded.
- Newly created database files use owner-only permissions where supported.
- Dependencies are pinned; vulnerability/license scanning is CI policy.
- Synthetic-only warnings appear in the README, tool descriptions, and startup
  logs.

## 11. Testing and quality

- Unit tests cover email normalization/validation, money arithmetic,
  idempotency, error mapping, pagination, and configuration validation.
- SQLite integration tests cover migrations, uniqueness, rollback, concurrent
  deposits/transfers, insufficient funds, restart persistence, and retries.
- MCP contract tests cover discovery, schemas, all seven tools, errors,
  cancellation, and both transports where practical.
- Tests use temporary databases and deterministic clocks/IDs; they require no
  network or external financial system.
- Race detection, static analysis, formatting, tests, and migration checks run in
  CI.

## 12. Acceptance criteria

1. All seven MCP tools are discoverable and conform to this document.
2. A unique-email recipient can be added/listed, the funding balance can be
   deposited/read, and a transfer can be recorded/listed/queried across restart.
3. Concurrent idempotent retries cannot deposit or transfer twice.
4. Invalid or rejected operations leave the balance and records unchanged.
5. A transfer decreases only the single funding balance; no recipient account or
   ledger data exists.
6. HTTP and stdio startup, cancellation, health/readiness, and graceful shutdown
   are verified.
7. Zap logs show operational outcomes without caller data, metrics, or traces.
8. Automated checks pass without real network or financial dependencies.

## 13. Assumptions and resolved planning decisions

- There is one implicit funding account represented by one persisted balance.
- The currency is fixed to USD and the first-run balance is zero.
- A recipient has a server-generated ID and a unique normalized email only; MCP
  operations identify recipients by email.
- Transfers complete synchronously and record operations rather than movement
  between sender/recipient accounts.
- Simple limit/offset pagination is sufficient for the sandbox.
- Structured Zap logging plus health/readiness is sufficient; metrics, tracing,
  and exporters are intentionally excluded.
- External systems protect deployments beyond localhost.
