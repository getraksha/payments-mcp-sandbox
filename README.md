# payments-mcp-sandbox

A synthetic, enterprise-style MCP server for demonstrating shared payment
operations governed by AI agents.

The server exposes simulated payment tools such as:

- `add_recipient`
- `list_recipients`
- `deposit_money`
- `get_current_account_balance`
- `transfer_money`
- `list_transactions`
- `check_transfer_status`

The MCP server represents a shared enterprise service and intentionally does not
implement end-user authentication or authorization. Access to tools and actions
can be governed externally by systems such as AGP.

All recipients, accounts, transactions, and money transfers are synthetic.
**No real financial transactions are performed.** The project contains no real
payment endpoint, credential field, or external financial-system dependency.

## Current status

The repository currently contains the Task 1 executable foundation only. The
binary does not start an MCP transport, diagnostics listener, database, or any
other long-running service yet.

## Supported development baseline

- Go 1.25.x.
- Linux, macOS, and Windows on the GitHub-hosted amd64 runners.
- Local development on amd64 or arm64 where Go 1.25 is supported.

CI builds and tests all three operating systems. The complete quality suite,
including race detection and vulnerability analysis, runs on Linux.

## Reproducible developer commands

The repository's quality tools are version-pinned by the `Makefile` and installed
into the ignored local `.bin/` directory.

```sh
make tools             # install pinned Staticcheck and govulncheck
make fmt-check         # verify formatting without changing files
make vet               # run go vet
make staticcheck       # run Staticcheck
make test              # run unit tests
make test-race         # run tests with the race detector
make migration-check   # verify migrations when that directory is introduced
make govulncheck       # query the Go vulnerability database
make build             # build bin/payments-mcp-sandbox
make ci                # run the complete local CI baseline
```

Use `make fmt` to format Go sources. Builds can carry reviewed version and commit
values without reading repository state implicitly:

```sh
make build VERSION=v0.1.0 COMMIT=0123abcd
./bin/payments-mcp-sandbox -version
```

An executable run without `-version` exits successfully without opening a
listener or serving requests.
