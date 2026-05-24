.PHONY: help test test-go test-macos build build-go build-macos run serve macos-run vet fmt check clean export-library import-library visualize

APP_NAME := linetta
BINDIR ?= bin
BIN := $(BINDIR)/$(APP_NAME)
MACOS_DIR := macos/Linetta

GOAL ?= Draft a hopeful climate fiction opening
TITLE ?= Harbor of Glass
DB ?= .linetta/dev.db
ADDR ?= 127.0.0.1:43190
CONFIG ?= examples/tessera.yaml
BACKUP ?= backup.zip
RESTORE_DB ?= .linetta/restored.db
EVENTS ?= .tessera/runs/linetta/events.jsonl
REPORT ?= .tessera/runs/linetta/report.html

help: ## Show available targets.
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

test: test-go test-macos ## Run Go and Swift tests.

test-go: ## Run Go tests.
	go test ./...

test-macos: ## Run Swift package tests.
	cd $(MACOS_DIR) && swift test

build: build-go build-macos ## Build Go CLI and Swift package.

build-go: ## Build the Go CLI into bin/linetta.
	mkdir -p $(BINDIR)
	go build -o $(BIN) ./cmd/linetta

build-macos: ## Build the Swift package.
	cd $(MACOS_DIR) && swift build

run: ## Run the CLI with GOAL and TITLE variables.
	go run ./cmd/linetta --goal "$(GOAL)" --title "$(TITLE)"

serve: ## Start the local engine with DB and ADDR variables.
	mkdir -p .linetta
	go run ./cmd/linetta serve --db "$(DB)" --addr "$(ADDR)"

macos-run: build-go ## Run the SwiftUI macOS app with the embedded engine.
	cd $(MACOS_DIR) && LINETTA_BIN=$(PWD)/$(BIN) swift run Linetta

vet: ## Run Go vet.
	go vet ./...

fmt: ## Format Go code.
	go fmt ./...

check: vet test ## Run static checks and tests.

export-library: ## Export the local library to BACKUP.
	go run ./cmd/linetta export-library --db "$(DB)" --config "$(CONFIG)" --out "$(BACKUP)"

import-library: ## Import BACKUP into RESTORE_DB.
	go run ./cmd/linetta import-library --in "$(BACKUP)" --db "$(RESTORE_DB)"

visualize: ## Write an HTML report from EVENTS to REPORT.
	go run ./cmd/linetta visualize "$(EVENTS)" --out "$(REPORT)"

clean: ## Remove local build artifacts.
	rm -rf $(BINDIR) $(MACOS_DIR)/.build
