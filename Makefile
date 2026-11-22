# Developer Makefile for qi-json (packages qijson at the module root, internal/codec, scanner and cmd/qi-json-gen).
#
# The required verification order is defined in docs/constitution.md:
#   test -> gofmt check -> vet -> golangci-lint
# Run everything with `make verify`.

GO            ?= go
GOLANGCI_LINT ?= golangci-lint
PKG           ?= ./...
SCANNER       ?= ./scanner
FUZZ_TIME     ?= 10s
BENCH_TIME    ?= 1s
COVER_PROFILE ?= cover.out

.DEFAULT_GOAL := help
.PHONY: help verify test race generate fmt fmt-check vet lint fuzz bench cover clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

verify: ## Run the required checks in order: test, fmt-check, vet, lint
	@$(MAKE) --no-print-directory test
	@$(MAKE) --no-print-directory fmt-check
	@$(MAKE) --no-print-directory vet
	@$(MAKE) --no-print-directory lint

test: ## Run the test suite (no cache)
	$(GO) test $(PKG) -count=1

race: ## Run the test suite with the race detector
	$(GO) test $(PKG) -race -count=1

generate: ## Run go generate across the module (regenerates codegen fixtures)
	$(GO) generate $(PKG)

fmt: ## Format all Go files in place
	gofmt -w .

fmt-check: ## Fail if any Go file is not gofmt-formatted
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "unformatted Go files:"; \
		echo "$$files"; \
		echo "run 'make fmt' to fix"; \
		exit 1; \
	fi

vet: ## Run go vet
	$(GO) vet $(PKG)

lint: ## Run golangci-lint (v2, default linters)
	@command -v $(GOLANGCI_LINT) >/dev/null || { echo "$(GOLANGCI_LINT) is not installed"; exit 1; }
	$(GOLANGCI_LINT) run

fuzz: ## Fuzz the validator for FUZZ_TIME (default 10s)
	$(GO) test -run=^$$ -fuzz=FuzzValid -fuzztime=$(FUZZ_TIME) $(SCANNER)

bench: ## Run benchmarks for BENCH_TIME (default 1s), with memory stats
	$(GO) test -run=^$$ -bench=. -benchmem -benchtime=$(BENCH_TIME) $(PKG)

cover: ## Write a coverage profile and print per-function coverage
	$(GO) test $(PKG) -count=1 -coverprofile=$(COVER_PROFILE)
	$(GO) tool cover -func=$(COVER_PROFILE)

clean: ## Remove generated artifacts
	rm -f $(COVER_PROFILE)
