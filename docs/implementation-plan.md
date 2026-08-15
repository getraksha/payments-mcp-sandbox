# Payments MCP Sandbox Implementation Plan

## 1. Objective and constraints

Implement `docs/requirements.md` as a small, production-quality Go service that
is easy to run locally, observable when deployed, and safe against duplicate or
partial synthetic transfers.

This plan does not authorize production code in the planning pull request. It
defines architecture, technology decisions, sequencing, and acceptance criteria
for a later implementation stage.

## 2. Architecture

Use a conventional layered service with dependency direction toward the domain:

```text
MCP stdio / Streamable HTTP        diagnostics HTTP
              |                  /healthz /readyz /metrics
              v                            |
       transport adapters                  |
              |                            |
              v                            v
       application services <------ observability
              |
              v
       domain types/rules
              |
              v
       repository interfaces
              |
              v
        SQLite adapter + migrations
```

Handlers decode MCP input, invoke application services, and encode results; they
contain no payment rules or SQL. Services own use-case orchestration and
transaction boundaries. Domain types own invariants and typed errors. SQLite
sits behind small behavior-oriented interfaces so tests can use fakes without a
generic repository framework.

### Proposed layout

```text
cmd/payments-mcp-sandbox/main.go       process assembly and lifecycle
internal/app/                          use cases and transaction orchestration
internal/config/                       environment/flag loading and validation
internal/domain/                       recipient, money, transfer, errors
internal/mcpserver/                    schemas, handlers, result/error mapping
internal/store/                        narrow storage interfaces
internal/store/sqlite/                 SQLite implementation and tx helpers
internal/observability/                slog, metrics, tracing, health
internal/platform/                     clock/ID seams only where needed
migrations/                            embedded versioned SQL
docs/                                  protocol and operating documentation
test/contract/                         black-box transport contract tests
```

Avoid `pkg/`, dependency injection frameworks, generic CRUD repositories, event
buses, and background workers until a concrete requirement needs them.

## 3. Technology decisions

Pin current stable versions in `go.mod` when implementation begins. Upgrades that
affect protocols, storage, or migrations require explicit review and tests.

### TD-1: MCP SDK and transports

- **Decision:** Use official `github.com/modelcontextprotocol/go-sdk/mcp` v1.
- **Recommendation:** Use `StreamableHTTPHandler` for deployment and
  `StdioTransport` locally. Configure HTTP statelessly because durable state is
  in SQLite and no server-to-client MCP requests are required.
- **Reasoning:** The official SDK tracks MCP, supplies schema/lifecycle support,
  and implements both transports. Stateless HTTP avoids in-memory session
  ownership for request/response-only tools.
- **Alternatives:** `mark3labs/mcp-go` is established and ergonomic, but the
  official SDK minimizes specification drift. Hand-written JSON-RPC is rejected.
- **Review point:** Pin/document accepted protocol versions; SDK upgrades must
  pass negotiation and contract tests.

### TD-2: Go baseline, HTTP, and logging

- **Decision:** Select a supported stable Go release at implementation time and
  use `context`, `errors`, `net/http`, `database/sql`, and `log/slog`.
- **Recommendation:** Prefer standard-library facilities and wrap only at domain
  or test seams.
- **Reasoning:** They provide cancellation, hardened HTTP, database abstraction,
  and structured logs with minimal dependency surface.
- **Alternatives:** Zap and Zerolog are mature but their extra performance is not
  required at this expected load; a custom logging facade adds little value.

### TD-3: Persistence

- **Decision:** Use SQLite via `database/sql` and `modernc.org/sqlite`.
- **Recommendation:** Use a file-backed database with foreign keys, busy timeout,
  WAL where supported, bounded connections, and explicit write transactions.
- **Reasoning:** SQLite fits a self-contained sandbox; the pure-Go driver avoids
  CGO and cross-build complexity. The scope matches its single-process model.
- **Alternatives:** `mattn/go-sqlite3` is mature and potentially faster but needs
  CGO. PostgreSQL fits future multi-writer deployment but adds external runtime.
  Maps cannot provide durable transactional integrity.
- **Review point:** Benchmark concurrent transfers and document exact pool/WAL
  settings rather than trusting defaults.

### TD-4: Migrations

- **Decision:** Use `github.com/pressly/goose/v3` with SQL embedded by `embed.FS`.
- **Recommendation:** Apply upward migrations before readiness; test up/down, but
  never auto-downgrade at production startup.
- **Reasoning:** Goose is mature, supports SQLite and embedded migrations, and
  avoids custom schema-version code.
- **Alternatives:** `golang-migrate/migrate/v4` is also mature; Goose offers a
  smaller embedded-library workflow here. Home-grown migration code is rejected.

### TD-5: SQL and transaction boundaries

- **Decision:** Write explicit SQL in the SQLite adapter; do not use an ORM.
- **Recommendation:** Map private persistence rows to domain types and provide a
  purpose-built transaction operation for transfer/idempotency/ledger writes.
- **Reasoning:** The schema is small and exact locking/invariants must be easy to
  review.
- **Alternatives:** `sqlc` is worth revisiting if query count grows. GORM/Ent add
  abstraction and can obscure transaction details without enough v1 benefit.

### TD-6: Validation

- **Decision:** Combine SDK JSON schemas, explicit domain constructors, and
  `github.com/go-playground/validator/v10` for structural DTO rules.
- **Recommendation:** Keep cross-field, normalization, money, currency, and
  storage-dependent checks in domain/application code.
- **Reasoning:** This gives useful discovery-time schemas without trusting
  transport validation for business invariants.
- **Alternatives:** Fully manual validation reduces one dependency but risks
  inconsistent field errors. Schema-only checks are insufficient.

### TD-7: IDs, time, and money

- **Decision:** Use `github.com/google/uuid`, UTC timestamps, injected clock/ID
  functions, and `int64` minor units.
- **Recommendation:** Prefer UUIDv7 if supported by the pinned version, otherwise
  UUIDv4; never expose database row IDs. Check arithmetic before updates.
- **Reasoning:** Choices are portable, testable, and avoid floating-point errors.
- **Alternatives:** Decimal libraries fit FX/multiple scales, neither in scope.
  ULIDs are viable but less universally recognized.

### TD-8: Configuration

- **Decision:** Use `github.com/caarlos0/env/v11` plus a small standard-library
  flag layer for non-secret overrides.
- **Recommendation:** Parse once into an immutable struct, validate explicitly,
  and document `.env.example` without implicitly loading it in production.
- **Reasoning:** It is focused and mature; typed configuration is simpler than
  Viper's broad global/file surface for this service.
- **Alternatives:** Koanf is appropriate if multiple file formats/live merging
  become requirements. Raw `os.LookupEnv` is verbose and error-prone.

### TD-9: Observability

- **Decision:** Use `slog` for logs and OpenTelemetry Go API/SDK for metrics and
  traces, with configurable OTLP/HTTP and Prometheus exporters.
- **Recommendation:** Instrument tool/database boundaries and use `otelhttp` for
  HTTP. Exporters/tracing are optional; metrics use bounded enumerated labels.
- **Reasoning:** OTel traces/metrics are stable, vendor-neutral, and common in
  enterprise collectors; logs work even with export disabled.
- **Alternatives:** Direct `prometheus/client_golang` is simpler for
  Prometheus-only deployments but splits instrumentation. OTel log export is not
  required.

### TD-10: Testing and quality

- **Decision:** Use Go `testing`, optional `testify` assertions, temporary SQLite,
  built-in fuzzing, and SDK in-memory transport tests.
- **Recommendation:** CI runs format checks, `go test ./...`, race tests, vet,
  Staticcheck, `govulncheck`, and migration verification.
- **Reasoning:** This is repeatable and validates the real embedded database.
- **Alternatives:** SQL mocks test expected calls rather than SQLite behavior and
  are rejected for repository correctness; container databases are out of scope.

## 4. Data design

The first migration defines at least:

- `service_metadata`: initialized currency and domain settings.
- `accounts`: funding/recipient account, currency, non-negative balance, type,
  timestamps.
- `recipients`: opaque ID, unique normalized external reference/handle, display
  fields, status, account foreign key, timestamps.
- `transfers`: IDs/references, amount, currency, status, safe failure code,
  idempotency key, versioned normalized-request fingerprint, note, timestamps.
- `ledger_entries`: transfer/account references, debit-or-credit direction,
  amount, timestamp, and lookup indexes.

Use `CHECK`, `UNIQUE`, and foreign keys as a second defense. Store canonical UTC
times. Preserve display values separately from normalized searchable forms.

### Atomic transfer algorithm

Within one write transaction:

1. Look up the idempotency key.
2. If found, compare the normalized-input fingerprint; return the original on a
   match or conflict on a mismatch.
3. Load the active recipient and both accounts.
4. Validate currency, amount, maximum, and available balance.
5. Insert the transfer.
6. Debit funding with a conditional balance update and require one changed row.
7. Credit the recipient with overflow protection.
8. Insert equal debit and credit ledger entries.
9. Complete the transfer and commit.

Unique idempotency plus conditional debit makes concurrent retries safe. On a
uniqueness race, re-read and repeat step 2. Tests must force competing calls.

## 5. Interface design

### Tool registration and results

- Register five strongly typed handlers in one composition module.
- Include synthetic-only warnings and constraints in every relevant schema.
- Map transport DTOs to domain commands; never expose SQLite structs.
- Centralize typed error mapping.
- Return structured content as authoritative, with concise text only when client
  compatibility needs it.

Expected errors use a documented shape:

```json
{
  "error": {
    "code": "INSUFFICIENT_FUNDS",
    "message": "The sandbox funding account has insufficient synthetic funds.",
    "retryable": false,
    "details": []
  },
  "request_id": "..."
}
```

Clients branch on `code`; request IDs are correlation-only and never metric
labels.

### Cursor format

Encode version, resource kind, normalized-filter digest, last timestamp, and last
ID as size-bounded URL-safe data. Authenticate with HMAC-SHA256 using the cursor
secret. Decode strictly and compare signatures in constant time. Document that
secret rotation invalidates outstanding cursors.

## 6. Lifecycle

1. Parse and validate configuration.
2. Initialize stderr/JSON logging and build metadata.
3. Open SQLite with explicit settings; verify permissions and ping.
4. Apply migrations and validate persistent currency metadata.
5. Assemble repositories, services, telemetry, health, and MCP tools.
6. Start selected MCP transport and optional diagnostics listener.
7. Mark ready only after mandatory startup succeeds.
8. On SIGINT/SIGTERM, mark unready, stop acceptance, cancel root context, drain
   within deadline, flush telemetry, close SQLite, and log final outcome.

Stdio always reserves stdout. Its diagnostics listener is opt-in.

## 7. Work breakdown

Each task is independently reviewable. Dependencies are explicit.

### Task 1: Go module and engineering baseline

**Depends on:** approved planning PR.

- Add module, pinned dependencies, build metadata, notices, Make targets, CI
  gates, and a minimal executable skeleton.
- Document supported Go/platform versions and reproducible commands.

**Acceptance:** Clean checkout formats, vets, builds, and tests in CI; no service
accepts requests yet.

### Task 2: Configuration and bootstrap logging

**Depends on:** Task 1.

- Add typed config, environment names, safe defaults, `.env.example`, validation,
  redacted summaries, and `slog` setup.
- Test unsafe remote bind, secrets, monetary overflow, and conflicts.

**Acceptance:** Bad config fails before database/listeners; snapshots contain no
secrets.

### Task 3: Domain and error taxonomy

**Depends on:** Task 1.

- Add recipient, account, money, transfer/status, pagination, typed errors,
  normalization, checked arithmetic, test clock/IDs, and fingerprints.
- Unit/fuzz-test boundaries.

**Acceptance:** Every invariant/error code is tested without MCP or SQLite
imports.

### Task 4: SQLite schema and repositories

**Depends on:** Tasks 2-3.

- Add Goose migrations, metadata, seed-once funding account, settings,
  constraints, indexes, queries, and deterministic keyset pagination.
- Test migrate up/down, restart, rollback, constraints, permissions/corruption
  where portable, and cursor boundaries with temp files.

**Acceptance:** Reopen preserves state; reinitialization never resets funds;
constraint and transaction tests prove no partial writes.

### Task 5: Recipient service

**Depends on:** Tasks 3-4.

- Implement add/list with normalization, conflicts, cursor handling,
  cancellation, and narrow store interfaces.

**Acceptance:** Creation atomically includes an account; collisions map
consistently; multi-page listing is complete/stable.

### Task 6: Transfer and transaction services

**Depends on:** Tasks 3-4.

- Implement atomic transfer, idempotent replay/conflict, status, filtered history,
  and safe errors.
- Test concurrent same-key and competing-balance calls under race detection.

**Acceptance:** Funds move exactly once; ledger balances; failure/cancellation
leaves no partial state; results survive restart.

### Task 7: MCP contracts

**Depends on:** Tasks 5-6.

- Register all five tools with structured schemas/results and warnings.
- Map domain versus internal errors correctly.
- Test discovery, schemas, validation, calls, idempotency, pagination,
  cancellation, and correlation through in-memory transport.

**Acceptance:** Contract snapshots are stable and handlers cannot bypass services.

### Task 8: HTTP and stdio adapters

**Depends on:** Tasks 2 and 7.

- Wire stateless Streamable HTTP at `/mcp` with timeouts, limits, host/origin
  policy, IDs, recovery, and concurrency bounds.
- Wire stdio with stdout isolation; add transport/protocol smoke tests.

**Acceptance:** Both modes complete an end-to-end workflow; oversized, timed-out,
cancelled, and incompatible requests fail safely.

### Task 9: Observability and diagnostics

**Depends on:** Tasks 2, 5, 6, and 8.

- Add boundary logs, OTel, optional exporters, health state, health/readiness/
  metrics endpoints, and version information.
- Test label cardinality, redaction, readiness, and exporter isolation.

**Acceptance:** Tool outcomes/latency are visible without payloads or IDs as
labels; disabled/unavailable exporters need no collector and do not affect funds.

### Task 10: Lifecycle, packaging, and operations

**Depends on:** Tasks 4, 8, and 9.

- Add signal shutdown, non-root container, writable data-mount guidance, run
  examples, backup/restore notes, external access-control/TLS guidance, warnings,
  restart/termination tests, and release checklist.

**Acceptance:** Shutdown drains/cancels bounded work, flushes, closes SQLite, and
exits by deadline; documented local/container flows need no financial system.

### Task 11: Final hardening

**Depends on:** Tasks 1-10.

- Run race, fuzz smoke, vulnerability, static, migration, contract, and E2E
  suites; verify every requirement and review output for leakage/ambiguity.

**Acceptance:** Each requirement acceptance criterion has automated evidence or a
documented manual check, with no unresolved high-severity finding.

## 8. Verification matrix

| Capability | Primary evidence | Failure/edge evidence |
| --- | --- | --- |
| Add recipient | service + SQLite + MCP | normalized duplicates, rollback, invalid input |
| List recipients | repository + MCP | empty/exact page, tied times, tampered cursor |
| Transfer | integration + concurrency | retry race, insufficient funds, overflow, cancel |
| List transactions | repository + MCP | filters, tied times, cursor/filter mismatch |
| Check status | service + restart E2E | unknown ID, persisted terminal status |
| Configuration | table-driven unit tests | unsafe bind, state mismatch, redaction |
| Lifecycle | transport/E2E | migration failure, signal in-flight, timeout |
| Observability | recorder/snapshots | bounded labels, redaction, exporter down |

## 9. Security and reliability checklist

- No production endpoint, credential field, or real-money claim exists.
- Threat-model remote exposure, malicious payloads, cursor tampering, replay,
  resource exhaustion, and database-file access.
- Parameterize all SQL and configure explicit HTTP limits.
- Prove idempotency at database/transaction level and ledger consistency under
  concurrency/rollback.
- Keep notes, handles, keys, cursors, and raw SQL out of telemetry/errors.
- Document SQLite single-process and backup/restore limits.
- Verify provenance, pins, licenses, and vulnerabilities.

## 10. Delivery strategy

1. Land engineering/domain/storage foundations without network exposure.
2. Land use cases and MCP contracts behind in-memory tests.
3. Enable stdio, then hardened HTTP and diagnostics.
4. Add packaging and run restart/idempotency/concurrency acceptance.
5. Tag only after the verification matrix passes.

Pre-release schema changes use forward migrations. After the first tag, every
schema change requires compatibility and backup guidance.

## 11. Assumptions, risks, and decision gates

### Assumptions

- One funding account/currency, synchronous completion, external access control,
  one process/database owner, and SQLite-suitable volume are accepted.

### Risks and mitigations

- **SQLite contention:** short writes, WAL/busy timeout, bounded concurrency, load
  test the intended envelope.
- **MCP evolution:** pin SDK/protocol and run negotiation contracts.
- **Duplicate transfers:** uniqueness, fingerprints, conditional debit, races.
- **Sandbox misuse:** repeated warnings and no payment integration points.
- **Unauthenticated exposure:** loopback, explicit opt-in, separate diagnostics,
  external gateway/TLS guidance.
- **Telemetry leakage/cardinality:** allowlisted attributes and recorder tests.

### Decisions requested before implementation

1. Confirm the funding-account model and balance exposure.
2. Confirm whether recipient creation needs an idempotency key.
3. Select default currency, initial balance, and maximum amount.
4. Confirm synchronous v1 versus later durable settlement.
5. Confirm MCP protocol versions and deployment environment.

A durable pending/settlement worker is a material scope addition and should get a
separate design update. The other answers refine defaults without changing the
layered architecture.
