# Makefile for Acthur development
# Usage: make <target>

BINARY     = acthur
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    = -ldflags "-s -w -X github.com/acthur/acthur/cmd/acthur.Version=$(VERSION)"
BUILD_DIR  = bin
MAIN       = ./cmd/acthur

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build binary for current platform
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) $(MAIN)
	@echo "  ✓  built $(BUILD_DIR)/$(BINARY) (version: $(VERSION))"

.PHONY: build-all
build-all: ## Build binaries for all supported platforms
	@mkdir -p $(BUILD_DIR)
	GOOS=linux   GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-amd64   $(MAIN)
	GOOS=linux   GOARCH=arm64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-linux-arm64   $(MAIN)
	GOOS=darwin  GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-amd64  $(MAIN)
	GOOS=darwin  GOARCH=arm64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-darwin-arm64  $(MAIN)
	GOOS=windows GOARCH=amd64  go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY)-windows-amd64.exe $(MAIN)
	@echo "  ✓  built all platforms"

.PHONY: install
install: build ## Install binary to GOPATH/bin
	cp $(BUILD_DIR)/$(BINARY) $(GOPATH)/bin/$(BINARY)
	@echo "  ✓  installed to $(GOPATH)/bin/$(BINARY)"

# ── Test ─────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all unit tests
	go test ./... -v -count=1

.PHONY: test-race
test-race: ## Run tests with race detector
	go test ./... -race -count=1

.PHONY: coverage
coverage: ## Run tests and open coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out

# ── Quality ──────────────────────────────────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format all Go files
	gofmt -w .
	goimports -w .

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

# ── Development ──────────────────────────────────────────────────────────────

.PHONY: dev
dev: build ## Build and run acthur dev on the test fixture
	./$(BUILD_DIR)/$(BINARY) dev

.PHONY: watch
watch: ## Rebuild on file change (requires air)
	air -c .air.toml

# ── Release ──────────────────────────────────────────────────────────────────

.PHONY: snapshot
snapshot: ## Build snapshot release (goreleaser)
	goreleaser release --snapshot --clean

.PHONY: release
release: ## Cut a release (goreleaser — requires git tag)
	goreleaser release --clean

# ── Housekeeping ─────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR) coverage.out dist/

.PHONY: help
help: ## Show this help
	@echo ""
	@echo "  Acthur — Development Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo ""
