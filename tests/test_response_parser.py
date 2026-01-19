"""Tests for response_parser module."""

from src.response_parser import Status, parse_response


class TestParseResponse:
    def test_parses_continue_status(self):
        output = """
Some Claude output here...

PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1: Investigation"
  status: CONTINUE
  files_changed:
    - src/foo.py
    - src/bar.py
  summary: "Investigated the codebase structure"
"""
        result = parse_response(output)

        assert result is not None
        assert result.phase_completed == "Phase 1: Investigation"
        assert result.status == Status.CONTINUE
        assert result.files_changed == ["src/foo.py", "src/bar.py"]
        assert result.summary == "Investigated the codebase structure"
        assert result.error is None

    def test_parses_done_status(self):
        output = """
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 4: Cleanup"
  status: DONE
  files_changed: []
  summary: "All phases complete"
"""
        result = parse_response(output)

        assert result is not None
        assert result.status == Status.DONE

    def test_parses_blocked_status_with_error(self):
        output = """
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "Could not proceed"
  error: "Missing API credentials"
"""
        result = parse_response(output)

        assert result is not None
        assert result.phase_completed is None
        assert result.status == Status.BLOCKED
        assert result.error == "Missing API credentials"

    def test_parses_status_in_code_block(self):
        output = """
Here's my status:

```
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 2: Implementation"
  status: CONTINUE
  files_changed:
    - main.go
  summary: "Added new feature"
```
"""
        result = parse_response(output)

        assert result is not None
        assert result.phase_completed == "Phase 2: Implementation"
        assert result.status == Status.CONTINUE

    def test_returns_none_when_no_status_block(self):
        output = "Just some regular Claude output without any status block."

        result = parse_response(output)

        assert result is None

    def test_handles_single_file_changed(self):
        output = """
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: main.py
  summary: "Did work"
"""
        result = parse_response(output)

        assert result is not None
        assert result.files_changed == ["main.py"]

    def test_handles_null_files_changed(self):
        output = """
PROGRAMMATOR_STATUS:
  phase_completed: "Phase 1"
  status: CONTINUE
  files_changed: null
  summary: "Research only"
"""
        result = parse_response(output)

        assert result is not None
        assert result.files_changed == []

    def test_handles_string_null_phase(self):
        output = """
PROGRAMMATOR_STATUS:
  phase_completed: "null"
  status: CONTINUE
  files_changed: []
  summary: "Still working"
"""
        result = parse_response(output)

        assert result is not None
        assert result.phase_completed is None
