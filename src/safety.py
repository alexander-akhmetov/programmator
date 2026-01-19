"""Safety gates for the programmator loop."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from enum import Enum


class ExitReason(Enum):
    COMPLETE = "complete"
    MAX_ITERATIONS = "max_iterations"
    STAGNATION = "stagnation"
    BLOCKED = "blocked"
    USER_INTERRUPT = "user_interrupt"


@dataclass
class SafetyConfig:
    max_iterations: int = 50
    stagnation_limit: int = 3
    timeout_seconds: int = 900  # 15 minutes

    @classmethod
    def from_env(cls) -> SafetyConfig:
        return cls(
            max_iterations=int(os.environ.get("PROGRAMMATOR_MAX_ITERATIONS", 50)),
            stagnation_limit=int(os.environ.get("PROGRAMMATOR_STAGNATION_LIMIT", 3)),
            timeout_seconds=int(os.environ.get("PROGRAMMATOR_TIMEOUT", 900)),
        )


@dataclass
class SafetyState:
    iteration: int = 0
    consecutive_no_changes: int = 0
    last_error: str | None = None
    consecutive_errors: int = 0
    files_changed_history: list[list[str]] = field(default_factory=list)

    def record_iteration(self, files_changed: list[str], error: str | None = None) -> None:
        self.iteration += 1
        self.files_changed_history.append(files_changed)

        if files_changed:
            self.consecutive_no_changes = 0
        else:
            self.consecutive_no_changes += 1

        if error:
            if error == self.last_error:
                self.consecutive_errors += 1
            else:
                self.consecutive_errors = 1
            self.last_error = error
        else:
            self.consecutive_errors = 0
            self.last_error = None


def check_safety(config: SafetyConfig, state: SafetyState) -> ExitReason | None:
    if state.iteration >= config.max_iterations:
        return ExitReason.MAX_ITERATIONS

    if state.consecutive_no_changes >= config.stagnation_limit:
        return ExitReason.STAGNATION

    if state.consecutive_errors >= 3:
        return ExitReason.BLOCKED

    return None
