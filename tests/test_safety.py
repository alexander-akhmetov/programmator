"""Tests for safety module."""

from src.safety import ExitReason, SafetyConfig, SafetyState, check_safety


class TestSafetyState:
    def test_records_iteration_with_changes(self):
        state = SafetyState()

        state.record_iteration(["foo.py", "bar.py"])

        assert state.iteration == 1
        assert state.consecutive_no_changes == 0
        assert state.files_changed_history == [["foo.py", "bar.py"]]

    def test_tracks_consecutive_no_changes(self):
        state = SafetyState()

        state.record_iteration([])
        assert state.consecutive_no_changes == 1

        state.record_iteration([])
        assert state.consecutive_no_changes == 2

        state.record_iteration(["foo.py"])
        assert state.consecutive_no_changes == 0

    def test_tracks_consecutive_errors(self):
        state = SafetyState()

        state.record_iteration([], "error A")
        assert state.consecutive_errors == 1

        state.record_iteration([], "error A")
        assert state.consecutive_errors == 2

        state.record_iteration([], "error B")
        assert state.consecutive_errors == 1

        state.record_iteration([])
        assert state.consecutive_errors == 0


class TestCheckSafety:
    def test_returns_none_when_safe(self):
        config = SafetyConfig(max_iterations=50, stagnation_limit=3)
        state = SafetyState(iteration=5, consecutive_no_changes=1)

        result = check_safety(config, state)

        assert result is None

    def test_returns_max_iterations_when_exceeded(self):
        config = SafetyConfig(max_iterations=10)
        state = SafetyState(iteration=10)

        result = check_safety(config, state)

        assert result == ExitReason.MAX_ITERATIONS

    def test_returns_stagnation_when_no_changes(self):
        config = SafetyConfig(stagnation_limit=3)
        state = SafetyState(consecutive_no_changes=3)

        result = check_safety(config, state)

        assert result == ExitReason.STAGNATION

    def test_returns_blocked_on_repeated_errors(self):
        config = SafetyConfig()
        state = SafetyState(consecutive_errors=3)

        result = check_safety(config, state)

        assert result == ExitReason.BLOCKED
