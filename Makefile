.PHONY: help test test-go test-desktop test-tauri build-engine build-desktop bump-version ci

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: test-go test-desktop test-tauri ## Run all local verification

ci: test ## Alias for the CI verification contract

test-go: ## Run Go engine tests
	cd engine && go test ./...

test-desktop: ## Run desktop frontend tests and production build
	cd apps/desktop && pnpm test && pnpm build

test-tauri: build-engine ## Type-check the Tauri shell
	cd apps/desktop/src-tauri && cargo check

build-engine: ## Build the Go sidecar into the Tauri binaries directory
	bash scripts/build-engine.sh

build-desktop: build-engine ## Build the desktop release binary for the current OS
	cd apps/desktop && pnpm tauri build --no-bundle

bump-version: ## Bump app versions. Usage: make bump-version VERSION=0.1.0
	@test -n "$(VERSION)" || (echo "VERSION is required, for example: make bump-version VERSION=0.1.0" >&2; exit 2)
	bash scripts/bump-version.sh "$(VERSION)"
