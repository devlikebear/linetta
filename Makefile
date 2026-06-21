.PHONY: help dev test test-go test-desktop test-tauri test-mobile-engine validate-distribution build-engine build-mobile-engine-ios build-mobile-engine-android mobile-ios-init mobile-android-init build-mobile-ios-sim smoke-mobile-ios-sim build-mobile-android-debug build-mobile-android-release-smoke patch-mobile-android-signing build-desktop release-macos-local build-mas-local release-mas-local bump-version ci

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z0-9_-]+:.*##/ {printf "%-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Start the desktop app in dev mode
	bash scripts/dev.sh

test: test-go test-desktop test-tauri ## Run all local verification

ci: test ## Alias for the CI verification contract

test-go: ## Run Go engine tests
	cd engine && go test ./...

test-desktop: ## Run desktop frontend tests and production build
	cd apps/desktop && pnpm test && pnpm build

test-tauri: ## Type-check the Tauri shell
	cd apps/desktop/src-tauri && cargo check

test-mobile-engine: ## Run the Go engine suite under the mobile build tag
	cd engine && go test -tags mobile ./...

validate-distribution: ## Validate release packaging metadata
	bash scripts/validate-distribution.sh

build-engine: ## Build the standalone JSONRPC debug engine
	bash scripts/build-engine.sh

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

build-mobile-android-debug: ## Build a local Android debug APK for arm64
	cd apps/desktop && pnpm tauri android build --debug --apk --target aarch64 --ci

build-mobile-android-release-smoke: ## Build signed Android release APK/AAB with a temporary local keystore
	bash scripts/build-android-release-smoke.sh

patch-mobile-android-signing: ## Patch generated Android Gradle signing from keystore.properties
	bash scripts/patch-tauri-android-signing.sh

build-desktop: ## Build the desktop release binary for the current OS
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
