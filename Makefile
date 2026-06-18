.PHONY: help dev test test-go test-desktop test-tauri validate-distribution build-engine build-desktop release-macos-local build-mas-local bump-version ci

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Build the engine and start the desktop app in dev mode
	bash scripts/dev.sh

test: test-go test-desktop test-tauri ## Run all local verification

ci: test ## Alias for the CI verification contract

test-go: ## Run Go engine tests
	cd engine && go test ./...

test-desktop: ## Run desktop frontend tests and production build
	cd apps/desktop && pnpm test && pnpm build

test-tauri: build-engine ## Type-check the Tauri shell
	cd apps/desktop/src-tauri && cargo check

validate-distribution: ## Validate release packaging metadata
	bash scripts/validate-distribution.sh

build-engine: ## Build the Go sidecar into the Tauri binaries directory
	bash scripts/build-engine.sh

build-desktop: build-engine ## Build the desktop release binary for the current OS
	cd apps/desktop && pnpm tauri build --no-bundle

release-macos-local: ## Build, sign, notarize, and staple the macOS app + dmg locally
	bash scripts/release-macos-local.sh

build-mas-local: ## Build + sign a sandboxed macOS app locally (MAS prep, Developer ID signed)
	bash scripts/build-mas-local.sh

bump-version: ## Bump app versions. Usage: make bump-version VERSION=0.2.0
	@test -n "$(VERSION)" || (echo "VERSION is required, for example: make bump-version VERSION=0.2.0" >&2; exit 2)
	bash scripts/bump-version.sh "$(VERSION)"
