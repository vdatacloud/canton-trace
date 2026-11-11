.PHONY: help build vet test lint verify vuln-check install-hooks

help: ## List targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-14s %s\n", $$1, $$2}'

build: ## Build every package and the CLI
	go build ./...

vet: ## go vet
	go vet ./...

test: ## Unit tests (no network, no ledger)
	go test ./...

lint: ## golangci-lint, default config (as CI)
	golangci-lint run ./...

verify: build vet test ## Full local gate: build, vet, test (run before pushing)
	@echo "All verification checks passed."

GOVULNCHECK_VERSION ?= v1.9.0
vuln-check: ## Fail on code that calls a known-vulnerable function (needs network)
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

install-hooks: ## Install the pre-push hook (make verify)
	./scripts/install-git-hooks.sh
