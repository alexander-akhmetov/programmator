.PHONY: build test lint fmt install clean snapshot e2e-prep e2e-review e2e-plan

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o programmator ./cmd/programmator

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run
	@golangci-lint fmt --diff | grep -q . && { echo "Formatting issues found. Run 'make fmt' to fix."; exit 1; } || true

fmt:
	golangci-lint fmt

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/programmator

run:
	@echo "Usage: make run TICKET=<ticket-id>"
	@test -n "$(TICKET)" || (echo "Error: TICKET is required" && exit 1)
	go run ./cmd/programmator start $(TICKET)

clean:
	go clean ./...
	rm -rf .pytest_cache .ruff_cache dist/

snapshot:
	goreleaser release --snapshot --clean

# E2E test targets - create toy projects for manual integration testing
e2e-prep:
	@./scripts/prep-toy-test.sh

e2e-review:
	@./scripts/prep-review-test.sh

e2e-plan:
	@./scripts/prep-plan-test.sh
