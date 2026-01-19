"""Parse PROGRAMMATOR_STATUS from Claude's output."""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from enum import Enum

import yaml


class Status(Enum):
    CONTINUE = "CONTINUE"
    DONE = "DONE"
    BLOCKED = "BLOCKED"


@dataclass
class ProgrammatorStatus:
    phase_completed: str | None
    status: Status
    files_changed: list[str] = field(default_factory=list)
    summary: str = ""
    error: str | None = None


def parse_response(output: str) -> ProgrammatorStatus | None:
    pattern = r"PROGRAMMATOR_STATUS:\s*\n((?:[ \t]+.+\n?)+)"

    match = re.search(pattern, output)
    if not match:
        pattern_code_block = r"```\s*\n?PROGRAMMATOR_STATUS:\s*\n((?:[ \t]+.+\n?)+)```"
        match = re.search(pattern_code_block, output)

    if not match:
        return None

    yaml_content = match.group(1)

    try:
        data = yaml.safe_load(yaml_content)
    except yaml.YAMLError:
        return None

    if not isinstance(data, dict):
        return None

    phase = data.get("phase_completed")
    if phase == "null" or phase is None:
        phase = None

    status_str = data.get("status", "CONTINUE")
    try:
        status = Status(status_str)
    except ValueError:
        status = Status.CONTINUE

    files = data.get("files_changed", [])
    if files is None:
        files = []
    elif isinstance(files, str):
        files = [files]

    return ProgrammatorStatus(
        phase_completed=phase,
        status=status,
        files_changed=files,
        summary=data.get("summary", ""),
        error=data.get("error"),
    )
