.PHONY: help dev test test-go test-desktop test-tauri test-mobile-engine audit audit-go audit-desktop audit-rust validate-actions-runtime validate-distribution build-engine build-mcp-bridge build-mobile-engine-ios build-mobile-engine-android mobile-ios-init mobile-android-init build-mobile-ios-sim smoke-mobile-ios-sim dev-mobile-ios build-mobile-android-debug build-mobile-android-release-smoke patch-mobile-android-signing build-desktop release-macos-local build-mas-local release-mas-local bump-version ci

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Start the desktop app in dev mode
	bash scripts/dev.sh

test: test-go test-desktop test-tauri ## Run all local verification

ci: validate-actions-runtime validate-distribution test ## Run the CI verification contract

audit: audit-go audit-desktop audit-rust ## Check reachable and locked dependency vulnerabilities

audit-go: ## Check reachable Go vulnerabilities (requires govulncheck)
	cd engine && govulncheck ./...

audit-desktop: ## Check production frontend dependencies
	@# GHSA-qwww-vcr4-c8h2 only affects React Router's unstable RSC APIs.
	@if rg -n 'react-router/unstable_rsc|unstable_RSC|RSCRouter|RSCStaticRouter|getRSCStream|createCallServer' apps/desktop/src apps/desktop/package.json; then \
		echo "React Router RSC usage detected; remove the GHSA-qwww-vcr4-c8h2 audit exception"; \
		exit 1; \
	fi
	cd apps/desktop && pnpm audit --prod

audit-rust: ## Check RustSec advisories (requires cargo-audit)
	cd apps/desktop/src-tauri && cargo audit

test-go: ## Run Go engine tests
	cd engine && go test ./...
	bash scripts/validate-story-core-deps.sh

test-desktop: ## Run desktop frontend tests and production build
	cd apps/desktop && pnpm lint && pnpm test && pnpm build

test-tauri: ## Type-check the Tauri shell and run its unit tests
	cd apps/desktop/src-tauri && cargo check && cargo test

test-mobile-engine: ## Run the Go engine suite under the mobile build tag
	cd engine && go test -tags mobile ./...

validate-actions-runtime: ## Validate GitHub Actions use non-deprecated runtimes
	bash scripts/validate-actions-runtime.sh

validate-distribution: ## Validate release packaging metadata
	bash scripts/validate-distribution.sh

build-engine: ## Build the standalone JSONRPC debug engine
	bash scripts/build-engine.sh

build-mcp-bridge: ## Build the stdio MCP bridge Claude Desktop launches
	bash scripts/build-mcp-bridge.sh

build-mobile-engine-ios: ## Build the embedded Go engine xcframework for iOS
	bash apps/desktop/src-tauri/scripts/build-engine-ios.sh

build-mobile-engine-android: ## Build embedded Go engine shared libraries for Android
	bash apps/desktop/src-tauri/scripts/build-engine-android.sh

mobile-ios-init: ## Generate the ignored Tauri iOS project
	cd apps/desktop && pnpm tauri ios init --ci --skip-targets-install

mobile-android-init: ## Generate the ignored Tauri Android project
	cd apps/desktop && pnpm tauri android init --ci --skip-targets-install

build-mobile-ios-sim: ## Build a no-sign iOS simulator app bundle
	rm -rf apps/desktop/src-tauri/gen/apple/build/arm64-sim/Linetta.app apps/desktop/src-tauri/gen/apple/build/linetta-desktop_iOS.xcarchive
	cd apps/desktop && pnpm tauri ios build --debug --target aarch64-sim --ci --no-sign

smoke-mobile-ios-sim: ## Build, install, launch, and verify the iOS simulator app
	bash scripts/smoke-ios-simulator.sh

IOS_SIM ?= iPad Pro 11-inch (M4)
dev-mobile-ios: build-mobile-engine-ios ## Run the app on an iOS simulator with live reload (override sim: make dev-mobile-ios IOS_SIM="iPhone 16")
	bash scripts/dev-mobile-ios.sh "$(IOS_SIM)"

build-mobile-android-debug: ## Build a local Android debug APK for arm64
	cd apps/desktop && pnpm tauri android build --debug --apk --target aarch64 --ci

build-mobile-android-release-smoke: ## Build signed Android release APK/AAB with a temporary local keystore
	bash scripts/build-android-release-smoke.sh

patch-mobile-android-signing: ## Patch generated Android Gradle signing from keystore.properties
	bash scripts/patch-tauri-android-signing.sh

build-desktop: build-mcp-bridge ## Build the desktop release binary for the current OS
	cd apps/desktop && pnpm tauri build --no-bundle

release-macos-local: ## Build, sign, notarize, and staple the macOS app + dmg locally
	bash scripts/release-macos-local.sh

build-mas-local: ## Build + sign a sandboxed macOS app locally (MAS prep, Developer ID signed)
	bash scripts/build-mas-local.sh

release-mas-local: ## Build + sign + package the Mac App Store .pkg locally
	bash scripts/release-mas-local.sh

bump-version: ## Bump app versions. Usage: make bump-version VERSION=0.2.0
	@test -n "$(VERSION)" || (echo "VERSION is required, for example: make bump-version VERSION=0.2.0" >&2; exit 2)
	bash scripts/bump-version.sh "$(VERSION)"
