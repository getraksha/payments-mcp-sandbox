GO ?= go
GOFMT ?= gofmt

MODULE := github.com/getraksha/payments-mcp-sandbox
BINARY := bin/payments-mcp-sandbox
TOOLS_BIN := $(CURDIR)/.bin

VERSION ?= dev
COMMIT ?= unknown

STATICCHECK_VERSION := v0.7.0
GOVULNCHECK_VERSION := v1.6.0
STATICCHECK := $(TOOLS_BIN)/staticcheck
GOVULNCHECK := $(TOOLS_BIN)/govulncheck

LDFLAGS := -X $(MODULE)/internal/buildinfo.version=$(VERSION) -X $(MODULE)/internal/buildinfo.commit=$(COMMIT)

.PHONY: build ci deps fmt fmt-check govulncheck migration-check staticcheck test test-race tools vet

build:
	@mkdir -p bin
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/payments-mcp-sandbox

deps:
	$(GO) mod download
	$(GO) mod verify

fmt:
	$(GOFMT) -w cmd internal

fmt-check:
	@unformatted="$$( $(GOFMT) -l cmd internal )"; \
	if [ -n "$$unformatted" ]; then \
		echo "Go files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

staticcheck: $(STATICCHECK)
	$(STATICCHECK) ./...

govulncheck: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

migration-check:
	@if [ -d migrations ]; then \
		test -n "$$(find migrations -type f -name '*.sql' -print -quit)" || \
			(echo "migrations/ exists but contains no SQL migrations" && exit 1); \
	fi

tools: $(STATICCHECK) $(GOVULNCHECK)

$(STATICCHECK):
	@mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) $(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)

$(GOVULNCHECK):
	@mkdir -p $(TOOLS_BIN)
	GOBIN=$(TOOLS_BIN) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

ci: deps fmt-check vet staticcheck test test-race migration-check govulncheck build
