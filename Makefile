.PHONY: build test lint fmt install clean

build:
	go build ./...

test:
	go test ./...

test-race:
	go test -race ./...

lint:
	gofmt -l .
	go vet ./...

fmt:
	gofmt -w .

install:
	go install ./cmd/programmator

run:
	@echo "Usage: make run TICKET=<ticket-id>"
	@test -n "$(TICKET)" || (echo "Error: TICKET is required" && exit 1)
	go run ./cmd/programmator start $(TICKET)

clean:
	go clean ./...
	rm -rf .pytest_cache .ruff_cache
