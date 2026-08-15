# Payments MCP Sandbox Implementation Plan

## 1. Objective and constraints

Implement `docs/requirements.md` as a small, production-quality Go MCP service
that is easy to run locally and safe against duplicate or partial synthetic money
operations.

The implementation deliberately models only one USD funding balance, recipients
identified by email, deposits, and recorded transfers. It does not model
recipient accounts, ledger entries, configurable currencies, metrics, or traces.

This document is a plan. The planning pull request must not contain production
code or create implementation issues.

## 2. Architecture

Use a compact layered service with dependencies directed toward the domain:

```text
MCP stdio / Streamable HTTP       diagnostics HTTP
              |                 /healthz /readyz
              v                         |
       transport handlers               |
              |                         |
              v                         v
       application services <------ health state
              |
              v
       domain types and errors
              |
              v
       narrow store interfaces
              |
              v
       SQLite adapter + migrations

       Zap logging spans process boundaries
```

Handlers decode MCP input, invoke application services, and encode results. They
contain no business rules or SQL. Services own validation orchestration and
transaction boundaries. Domain types own email, money, and error invariants.
SQLite sits behind small behavior-oriented interfaces.

### Proposed layout

```text
cmd/payments-mcp-sandbox/main.go       process assembly and lifecycle
internal/app/                          recipient, deposit, transfer use cases
internal/config/                       environment/flag loading and validation
internal/domain/                       email, money, operation types, errors
internal/mcpserver/                    schemas, handlers, result/error mapping
internal/store/                        narrow storage interfaces
internal/store/sqlite/                 SQLite queries and transactions
internal/logging/                      Zap construction and shared fields
internal/health/                       readiness state and HTTP handlers
internal/platform/                     clock/ID seams only where needed
migrations/                            embedded versioned SQL
docs/                                  protocol and operating documentation
test/contract/                         black-box MCP contract tests
```

Avoid a general account model, recipient balances, ledger abstractions, ORMs,
dependency-injection frameworks, event buses, background workers, and custom
observability infrastructure.

## 3. Technology decisions

Pin stable dependency versions in `go.mod` when implementation begins. Upgrades
that affect MCP or storage behavior require explicit review and contract tests.

### TD-1: MCP SDK and transports

- **Decision:** Use official `github.com/modelcontextprotocol/go-sdk/mcp` v1.
- **Recommendation:** Use `StreamableHTTPHandler` for deployment and
  `StdioTransport` locally. Use stateless Streamable HTTP because all application
  state is in SQLite and no server-to-client MCP requests are needed.
- **Reasoning:** The official SDK tracks the protocol, provides schemas and
  lifecycle support, and implements both required transports.
- **Alternatives:** `mark3labs/mcp-go` is established and ergonomic, but the
  official SDK reduces protocol drift. Hand-written JSON-RPC is rejected.
- **Review point:** Pin and document accepted MCP protocol versions.

### TD-2: Structured logging

- **Decision:** Use `go.uber.org/zap` for structured logging.
- **Recommendation:** Emit production JSON logs by default and a development
  encoder locally; direct stdio-mode logs to stderr. Create one process logger
  and pass scoped loggers with bounded, non-sensitive fields.
- **Reasoning:** Zap is mature, fast, strongly typed, and well suited to
  production structured logs without adding a separate metrics/tracing stack.
- **Alternatives:** Standard-library `log/slog` would reduce dependencies, but Zap
  is the explicit project choice. Zerolog is also mature but offers no needed
  advantage here.

### TD-3: SQLite persistence

- **Decision:** Use `database/sql` with `modernc.org/sqlite`.
- **Recommendation:** Use a file-backed database with foreign keys, busy timeout,
  WAL where supported, bounded connections, and short explicit write
  transactions.
- **Reasoning:** SQLite fits a self-contained sandbox; the pure-Go driver avoids
  CGO/cross-build complexity. One process and one funding-balance row fit its
  concurrency model.
- **Alternatives:** `mattn/go-sqlite3` is mature and potentially faster but needs
  CGO. PostgreSQL adds an external runtime that this sandbox does not need.
- **Review point:** Test the chosen pool and WAL settings under concurrent
  deposit/transfer operations.

### TD-4: Schema migrations

- **Decision:** Use `github.com/pressly/goose/v3` with SQL migrations embedded by
  `embed.FS`.
- **Recommendation:** Apply upward migrations before readiness; test up/down
  paths, but never auto-downgrade on production startup.
- **Reasoning:** Goose is mature, supports embedded SQLite migrations, and avoids
  custom version management.
- **Alternatives:** `golang-migrate/migrate/v4` is also mature. Goose offers the
  simpler embedded-library workflow for this service.

### TD-5: SQL and transactions

- **Decision:** Use explicit parameterized SQL rather than an ORM.
- **Recommendation:** Keep persistence records private to the SQLite package and
  expose purpose-built atomic deposit/transfer methods.
- **Reasoning:** The four-table schema is small and exact balance/idempotency
  behavior should be visible in code review.
- **Alternatives:** `sqlc` can be reconsidered if the query surface grows. GORM or
  Ent adds abstraction without enough benefit for this scope.

### TD-6: Validation and email normalization

- **Decision:** Combine MCP schemas, explicit domain constructors, and
  `github.com/go-playground/validator/v10` for structural input validation.
- **Recommendation:** Normalize recipient emails by trimming and lowercasing,
  then validate a bounded email format before lookup or persistence. Keep money,
  idempotency, and storage-dependent rules in domain/application code.
- **Reasoning:** This gives helpful tool schemas and consistent field errors while
  preserving business rules outside transport handlers.
- **Alternatives:** Fully manual validation saves one dependency but tends to
  produce inconsistent errors. No email-delivery verification is needed because
  recipients are synthetic.

### TD-7: IDs, time, and money

- **Decision:** Use `github.com/google/uuid`, UTC timestamps, injected clock/ID
  functions, and `int64` cents.
- **Recommendation:** Use UUIDv7 if supported by the pinned library, otherwise
  UUIDv4. Check addition/subtraction before balance changes. Return currency as
  constant `USD` rather than storing/configuring it.
- **Reasoning:** These choices are portable and deterministic in tests while
  avoiding floating-point errors.
- **Alternatives:** Decimal libraries and multi-currency money types are
  unnecessary for one fixed two-decimal currency.

### TD-8: Configuration

- **Decision:** Use `github.com/caarlos0/env/v11` plus a small standard-library
  flag layer for non-secret overrides.
- **Recommendation:** Parse once into an immutable config struct, validate it,
  and document `.env.example` without loading it implicitly in production.
- **Reasoning:** The library is focused and mature; typed configuration is simpler
  than a broad file/global configuration framework.
- **Alternatives:** Koanf is appropriate if multiple config formats become a real
  requirement. Raw `os.LookupEnv` is verbose and inconsistent.

### TD-9: Tests and quality tools

- **Decision:** Use Go `testing`, optional `testify` assertions, temporary SQLite
  files, and the SDK in-memory transport.
- **Recommendation:** CI runs formatting checks, `go test ./...`, race tests,
  `go vet`, Staticcheck, `govulncheck`, and migration verification.
- **Reasoning:** This validates the real embedded database and MCP contracts
  without a network or external service.
- **Alternatives:** SQL mocks validate expected calls rather than SQLite
  semantics and are not suitable for balance transaction tests.

## 4. Data design

The first migration defines only the state required by the reviewed scope:

- `funding_balance`: exactly one row (`id = 1`) containing non-negative
  `balance_minor` and `updated_at`; first-run value is zero.
- `recipients`: server-generated ID, unique normalized `email`, and timestamps.
- `deposits`: ID, positive amount, unique idempotency key, normalized-request
  fingerprint, balance after the operation, and creation timestamp.
- `transfers`: ID, recipient foreign key, positive amount, `completed` status,
  unique idempotency key, normalized-request fingerprint, optional note, balance
  after the operation, and creation timestamp.

The schema has no general `accounts`, recipient-balance, or ledger tables and no
currency column. Application responses add constant `USD`.

Use `CHECK`, `UNIQUE`, and foreign-key constraints as a second line of defense.
Store timestamps in a canonical UTC representation.

### Atomic deposit

Within one write transaction:

1. Look up the deposit idempotency key.
2. If found, compare its normalized-input fingerprint and return the original
   deposit on a match or an idempotency conflict on a mismatch.
3. Increase the singleton balance with overflow protection.
4. Insert the deposit record, including the resulting balance.
5. Commit.

### Atomic transfer

Within one write transaction:

1. Look up the transfer idempotency key and apply the same replay/conflict rule.
2. Resolve the normalized recipient email.
3. Decrement the singleton balance with a conditional update requiring
   `balance_minor >= amount_minor` and exactly one changed row.
4. Insert one completed transfer record with the resulting balance.
5. Commit.

Unique idempotency constraints and conditional balance updates make concurrent
retries safe. On a uniqueness race, re-read and compare the fingerprint. No
recipient balance or ledger row is created.

## 5. MCP interface design

### Tool registration

- Register seven strongly typed handlers: `add_recipient`, `list_recipients`,
  `deposit_money`, `get_current_account_balance`, `transfer_money`,
  `list_transactions`, and `check_transfer_status`.
- Include synthetic-only warnings and field constraints in relevant schemas.
- Map transport DTOs to domain commands; never expose SQLite records.
- Centralize typed domain-error to MCP-result mapping.
- Return structured content as authoritative, with concise text only when client
  compatibility requires it.

Expected errors use one documented object shape:

```json
{
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "The sandbox funding balance has insufficient synthetic funds.",
    "retryable": false,
    "details": []
  },
  "request_id": "..."
}
```

Clients branch on `code`; request IDs are correlation-only.

### Pagination

Use validated integer `limit` and `offset` fields. Repositories apply a stable
timestamp-and-ID order, fetch `limit + 1`, and return `next_offset` only when
another row exists. This deliberately favors simplicity over snapshot-stable
pagination while concurrent inserts occur, which is acceptable for this sandbox.

## 6. Process lifecycle

1. Parse and validate configuration.
2. Initialize Zap and log safe build metadata.
3. Open SQLite with explicit settings; verify permissions and ping.
4. Apply embedded migrations and seed the singleton zero balance if absent.
5. Assemble repositories, application services, health state, and MCP tools.
6. Start the selected MCP transport and optional diagnostics listener.
7. Mark ready only after mandatory startup succeeds.
8. On SIGINT/SIGTERM, mark unready, stop accepting work, cancel the root context,
   drain HTTP requests within the shutdown deadline, close SQLite, sync Zap, and
   report the final outcome.

Stdio always reserves stdout for MCP. Its diagnostics listener is opt-in.

## 7. Implementation work breakdown

Each task should be independently reviewable. Dependencies are explicit.

### Task 1: Establish the Go module and quality baseline

**Depends on:** approved planning PR.

- Add module metadata, pinned dependencies, build metadata, Make targets, CI
  checks, and a minimal executable skeleton.
- Document supported Go/platform versions and reproducible commands.

**Acceptance:** A clean checkout formats, vets, builds, and tests in CI; no tool
serves requests yet.

### Task 2: Add configuration, Zap, and domain types

**Depends on:** Task 1.

- Implement immutable configuration, safe defaults, `.env.example`, Zap setup,
  email/money types, stable errors, checked arithmetic, and clock/ID seams.
- Test invalid config, remote-bind opt-in, redaction, normalization, and amount
  boundaries.

**Acceptance:** Bad configuration fails before listeners/database use; domain
tests need neither MCP nor SQLite; logs contain no caller data.

### Task 3: Add SQLite schema and storage

**Depends on:** Task 2.

- Add embedded Goose migrations, singleton zero-balance initialization,
  constraints, indexes, recipient queries, simple pagination, deposit/transfer
  transactions, and read queries.
- Test migration up/down, uniqueness, rollback, concurrent balance changes,
  restart persistence, lock/permission failures where portable, and idempotency.

**Acceptance:** Reopen preserves recipients, operations, and balance; concurrent
operations cannot overdraw, overflow, or apply one idempotency key twice.

### Task 4: Add recipient application services

**Depends on:** Tasks 2-3.

- Implement add/list using normalized email, server IDs, limit/offset, and stable
  duplicate/validation errors.

**Acceptance:** Email is the only recipient input attribute; normalized duplicate
emails cannot create multiple recipients; pages are deterministic.

### Task 5: Add balance, deposit, and transfer services

**Depends on:** Tasks 2-3.

- Implement balance lookup, idempotent deposits, idempotent transfers, status
  lookup, and recipient-filtered history.
- Exercise concurrent retries and competing transfers using real SQLite and the
  race detector.

**Acceptance:** Deposits increase and transfers decrease only the singleton
balance exactly once; no account/ledger model exists; rejected operations leave
state unchanged.

### Task 6: Add MCP schemas, handlers, and contracts

**Depends on:** Tasks 4-5.

- Register all seven tools with structured schemas/results and synthetic-only
  descriptions.
- Map expected domain errors to unsuccessful tool results and unexpected errors
  to safe internal failures.
- Test discovery, validation, success, retries, insufficient funds, pagination,
  status, cancellation, and request correlation through in-memory transport.

**Acceptance:** All seven tools are discoverable and contract snapshots remain
stable; handlers cannot bypass application services.

### Task 7: Add HTTP, stdio, health, and shutdown

**Depends on:** Tasks 2 and 6.

- Wire stateless Streamable HTTP at `/mcp` with timeouts, body/concurrency limits,
  host/origin policy, request IDs, and panic recovery.
- Wire stdio with stdout isolation and Zap on stderr.
- Add `/healthz` and `/readyz`, signal handling, ordered shutdown, and transport
  smoke tests.

**Acceptance:** Both modes complete add/deposit/transfer/query workflows;
oversized, timed-out, cancelled, and incompatible requests fail safely;
termination closes cleanly within the deadline.

### Task 8: Package and document operations

**Depends on:** Tasks 3 and 7.

- Add a minimal non-root container, writable data-mount guidance, examples for
  both transports, backup/restore notes, external access-control/TLS guidance,
  fixed-USD semantics, and synthetic-only warnings.

**Acceptance:** Documented local/container runs need no external financial or
observability service; restart preserves the balance and records.

### Task 9: Final hardening and acceptance verification

**Depends on:** Tasks 1-8.

- Run race, vulnerability, static, migration, MCP contract, and end-to-end
  suites; verify every requirement and inspect logs/errors for caller data.

**Acceptance:** Each requirement acceptance criterion has automated evidence or
  a documented manual check, with no unresolved high-severity finding.

## 8. Verification matrix

| Capability | Primary evidence | Failure/edge evidence |
| --- | --- | --- |
| Add recipient | service + SQLite + MCP | invalid email, normalized duplicate, rollback |
| List recipients | repository + MCP | empty/exact page, tied times, invalid offset |
| Deposit | transaction integration + MCP | retry race, overflow, rollback |
| Current balance | service + restart E2E | initial zero, deposit/transfer persistence |
| Transfer | transaction integration + concurrency | retry race, unknown email, insufficient funds |
| List transactions | repository + MCP | email filter, tied times, page boundary |
| Check status | service + restart E2E | unknown ID, persisted completed status |
| Configuration/logging | unit + log capture | unsafe bind, invalid limits, payload redaction |
| Lifecycle | transport/E2E | migration failure, signal in-flight, timeout |

## 9. Security and reliability checklist

- Confirm no production payment endpoint, credential field, or real-money claim.
- Threat-model remote exposure, malicious MCP payloads, replay, resource
  exhaustion, and database-file access.
- Parameterize all SQL and configure explicit HTTP limits.
- Prove idempotency and conditional balance changes under concurrency/rollback.
- Keep emails, notes, idempotency keys, and raw SQL out of logs/errors.
- Document SQLite single-process and backup/restore limits.
- Verify dependency provenance, versions, licenses, and vulnerability results.

## 10. Delivery strategy

1. Land domain/storage foundations without network exposure.
2. Land services and MCP contracts through in-memory transport tests.
3. Enable stdio, then hardened Streamable HTTP and health/readiness.
4. Add packaging and run restart/idempotency/concurrency acceptance.
5. Tag only after the verification matrix passes.

After the first tagged release, every schema change requires compatibility and
backup guidance.

## 11. Resolved review decisions and residual risks

### Resolved decisions

- Recipients contain only a server ID and unique normalized email; tools identify
  recipients by email.
- The service has one implicit funding balance, fixed USD, initialized to zero.
- `deposit_money` and `get_current_account_balance` join the five original tools.
- Transfers record operations and decrease only the funding balance; there are no
  recipient accounts or ledger entries.
- Zap is the structured logger. Metrics, tracing, OpenTelemetry, and exporters are
  excluded; basic health/readiness remains.
- List operations use simple limit/offset rather than signed cursors.

### Residual risks and mitigations

- **SQLite contention:** keep writes short, use WAL/busy timeout, bound
  concurrency, and test the intended envelope.
- **Duplicate money operations:** enforce database uniqueness, request
  fingerprints, conditional updates, and concurrent retry tests.
- **Offset pagination drift:** document that concurrent inserts can shift later
  pages; acceptable for this demonstration service.
- **MCP evolution:** pin SDK/protocol support and run negotiation contracts.
- **Sandbox misuse:** repeat warnings and ship no real payment integration points.
- **Unauthenticated exposure:** loopback defaults, explicit remote opt-in, and
  external gateway/TLS guidance.
