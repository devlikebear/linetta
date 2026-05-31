.PHONY: help test test-go test-desktop test-tauri build-engine ci

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: test-go test-desktop test-tauri ## Run all local verification

ci: test ## Alias for the CI verification contract

test-go: ## Run Go engine tests
	cd engine && go test ./...

test-desktop: ## Run desktop frontend tests and production build
	cd apps/desktop && pnpm test && pnpm build

test-tauri: ## Type-check the Tauri shell
	cd apps/desktop/src-tauri && cargo check

build-engine: ## Build the Go sidecar into the Tauri binaries directory
	bash scripts/build-engine.sh
