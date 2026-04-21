SHELL := /bin/sh

BINARY      := bot
PKG         := ./...
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)

.PHONY: help
help:  ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Targets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build:  ## Compile the bot binary to ./$(BINARY)
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/bot

.PHONY: test
test: test-unit test-integration  ## Run all tests

.PHONY: test-unit
test-unit:  ## Short/unit tests
	go test -short -race $(PKG)

.PHONY: test-integration
test-integration:  ## Run tests including integration (storage)
	go test -race -count=1 $(PKG)

.PHONY: lint
lint:  ## gofmt + go vet + golangci-lint
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt issues in:"; echo "$$unformatted"; exit 1; \
	fi
	go vet $(PKG)
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping"; \
	fi

.PHONY: tidy
tidy:  ## go mod tidy
	go mod tidy

.PHONY: dev
dev:  ## Run the bot with air hot-reload (requires TELEGRAM_BOT_TOKEN and OCTOPUS_API_KEY in env)
	@command -v air >/dev/null || { echo "install air: go install github.com/air-verse/air@latest"; exit 1; }
	air

.PHONY: run
run: build  ## Build and run once
	./$(BINARY)

.PHONY: docker
docker:  ## Build the docker image as octopus-agile-bot:local
	docker build --build-arg VERSION=$(VERSION) -t octopus-agile-bot:local .

.PHONY: clean
clean:  ## Remove build artifacts
	rm -f $(BINARY) coverage.txt
