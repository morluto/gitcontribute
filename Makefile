.PHONY: all test test-cover test-race lint lint-fix vet fmt clean build cover-report cover-check

GO ?= go
GOLANGCI_LINT ?= golangci-lint

all: fmt lint test-race vet

build:
	$(GO) build ./...

clean:
	$(GO) clean -cache -testcache
	rm -f coverage.out

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

lint-fix:
	$(GOLANGCI_LINT) run --fix ./...

test:
	$(GO) test -short -parallel=4 -count=1 -timeout 120s ./...

test-race:
	$(GO) test -short -race -parallel=4 -count=1 -timeout 120s ./...

test-race-full:
	$(GO) test -race -parallel=4 -count=1 -timeout 300s ./...

test-verbose:
	$(GO) test -short -v -parallel=4 -count=1 -timeout 120s ./...

test-cover:
	$(GO) test -short -count=1 -race -parallel=4 \
		-coverprofile=coverage.out \
		-covermode=atomic \
		-coverpkg=./internal/... \
		-timeout 180s \
		./...

cover-report: test-cover
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

cover-check: test-cover
	@COVERAGE=$$($(GO) tool cover -func=coverage.out | tail -n1 | awk '{print $$NF}' | tr -d '%'); \
	if [ "$$(echo "$$COVERAGE < 70" | bc -l 2>/dev/null || echo 0)" = "1" ]; then \
		echo "Coverage $$COVERAGE% is below 70% threshold"; \
		exit 1; \
	fi; \
	echo "Coverage: $$COVERAGE% (passes 70% threshold)"

tidy:
	$(GO) mod tidy
	@if [ -n "$$(git status --porcelain go.mod go.sum)" ]; then \
		echo "go.mod or go.sum not tidy; run 'go mod tidy'"; \
		exit 1; \
	fi

generate:
	$(GO) generate ./...
