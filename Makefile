.DEFAULT_GOAL := help

.PHONY: help all build clean fmt fmt-check vet lint lint-changed lint-full install-tools \
	test test-uncached test-race test-race-full test-verbose test-cover \
	cover-report cover-check tidy tidy-check generate generate-check docs \
	docs-check check verify

GO ?= go
GOFMT ?= gofmt
GOLANGCI_LINT ?= $(shell command -v golangci-lint 2>/dev/null || printf '%s/bin/golangci-lint' "$$($(GO) env GOPATH)")
GOLANGCI_LINT_VERSION ?= v2.12.2
GOLANGCI_LINT_BIN ?= $(shell $(GO) env GOPATH)/bin

help:
	@echo "Common targets:"
	@echo "  make test          cached local test suite"
	@echo "  make check         fast formatting, test, and changed-code lint checks"
	@echo "  make verify        complete uncached local validation"
	@echo "  make test-race     focused race tests for stateful packages"
	@echo "  make lint-full     audit the complete repository, including existing debt"
	@echo "  make fmt           format all Go packages"
	@echo "  make generate      refresh generated outputs"
	@echo "  make install-tools install the pinned golangci-lint version"

all: build

build:
	$(GO) build ./...

clean:
	$(GO) clean ./...
	rm -f coverage.out coverage.html

fmt:
	$(GO) fmt ./...

fmt-check:
	@unformatted="$$($(GOFMT) -l .)" || exit 1; \
	if [ -n "$$unformatted" ]; then \
		echo "The following Go files need formatting:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	$(GO) vet ./...

lint: lint-changed

lint-changed:
	$(GOLANGCI_LINT) run --new-from-rev=HEAD ./...

lint-full:
	$(GOLANGCI_LINT) run ./...

install-tools:
	@installer=$$(mktemp); \
	trap 'rm -f "$$installer"' EXIT; \
	curl -sSfL https://golangci-lint.run/install.sh -o "$$installer"; \
	sh "$$installer" -b "$(GOLANGCI_LINT_BIN)" "$(GOLANGCI_LINT_VERSION)"

test:
	$(GO) test -short -parallel=4 -timeout 120s ./...

test-uncached:
	$(GO) test -short -parallel=4 -count=1 -timeout 120s ./...

test-race:
	$(GO) test -short -race -parallel=2 -timeout 300s ./internal/app ./internal/corpus ./internal/workspace

test-race-full:
	$(GO) test -race -parallel=4 -count=1 -timeout 300s ./...

test-verbose:
	$(GO) test -short -v -parallel=4 -timeout 120s ./...

test-cover:
	$(GO) test -short -parallel=4 \
		-coverprofile=coverage.out \
		-covermode=set \
		-coverpkg=./internal/... \
		-timeout 180s \
		./...

cover-report: test-cover
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

cover-check: test-cover
	@coverage=$$($(GO) tool cover -func=coverage.out | tail -n1 | awk '{print $$NF}' | tr -d '%'); \
	awk -v coverage="$$coverage" 'BEGIN { if (coverage < 70) exit 1 }' || { \
		echo "Coverage $$coverage% is below 70% threshold"; \
		exit 1; \
	}; \
	echo "Coverage: $$coverage% (passes 70% threshold)"

tidy:
	$(GO) mod tidy

tidy-check:
	$(GO) mod tidy -diff

generate:
	$(GO) generate ./...

generate-check:
	./scripts/check-generated.sh

docs: docs-check

docs-check:
	./scripts/validate-agents-md.sh

check: fmt-check test lint-changed

verify: fmt-check test-uncached lint tidy-check generate-check docs-check
