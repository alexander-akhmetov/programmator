.PHONY: build test lint fmt install clean

build:
	go build ./...

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
	go install ./cmd/programmator

run:
	@echo "Usage: make run TICKET=<ticket-id>"
	@test -n "$(TICKET)" || (echo "Error: TICKET is required" && exit 1)
	go run ./cmd/programmator start $(TICKET)

clean:
	go clean ./...
	rm -rf .pytest_cache .ruff_cache
