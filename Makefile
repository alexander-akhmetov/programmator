.PHONY: test tests run install uninstall dev clean lint fmt

test tests:
	uv run pytest -v

lint:
	uv run ruff format --check src tests
	uv run ruff check src tests
	uv run ty check src tests

fmt:
	uv run ruff format src tests
	uv run ruff check --fix src tests

run:
	@echo "Usage: make run TICKET=<ticket-id> [DIR=/path/to/project]"
	@test -n "$(TICKET)" || (echo "Error: TICKET is required" && exit 1)
	uv run programmator start $(TICKET) $(if $(DIR),-d $(DIR))

install:
	uv tool install -e .

uninstall:
	uv tool uninstall programmator

dev:
	uv sync

clean:
	rm -rf .pytest_cache __pycache__ src/__pycache__ tests/__pycache__ .venv
