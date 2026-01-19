"""Wrapper for the ticket CLI."""

from __future__ import annotations

import os
import re
import subprocess
from dataclasses import dataclass, field


@dataclass
class Ticket:
    id: str
    title: str
    status: str
    body: str
    phases: list[Phase] = field(default_factory=list)

    @property
    def current_phase(self) -> Phase | None:
        for phase in self.phases:
            if not phase.completed:
                return phase
        return None

    @property
    def all_phases_complete(self) -> bool:
        return all(p.completed for p in self.phases)


@dataclass
class Phase:
    name: str
    completed: bool


class TicketClient:
    def get(self, ticket_id: str) -> Ticket:
        result = subprocess.run(
            ["ticket", "show", ticket_id],
            capture_output=True,
            text=True,
            check=True,
        )
        return self._parse_ticket(ticket_id, result.stdout)

    def add_note(self, ticket_id: str, note: str) -> None:
        subprocess.run(
            ["ticket", "add-note", ticket_id, note],
            capture_output=True,
            text=True,
            check=True,
        )

    def set_status(self, ticket_id: str, status: str) -> None:
        subprocess.run(
            ["ticket", "status", ticket_id, status],
            capture_output=True,
            text=True,
            check=True,
        )

    def update_phase(self, ticket_id: str, phase_name: str, completed: bool) -> None:
        ticket_path = self._get_ticket_path(ticket_id)

        with open(ticket_path) as f:
            content = f.read()

        checkbox = "[x]" if completed else "[ ]"
        opposite = "[ ]" if completed else "[x]"

        pattern = rf"- {re.escape(opposite)} {re.escape(phase_name)}"
        replacement = f"- {checkbox} {phase_name}"
        updated_content = re.sub(pattern, replacement, content)

        with open(ticket_path, "w") as f:
            f.write(updated_content)

    def _get_ticket_path(self, ticket_id: str) -> str:
        tickets_dir = os.environ.get("TICKETS_DIR", os.path.expanduser("~/.tickets"))
        return os.path.join(tickets_dir, f"{ticket_id}.md")

    def _parse_ticket(self, ticket_id: str, content: str) -> Ticket:
        lines = content.strip().split("\n")

        title = ""
        status = "open"
        body_lines = []
        phases = []

        in_frontmatter = False
        frontmatter_count = 0

        for line in lines:
            if line.strip() == "---":
                frontmatter_count += 1
                in_frontmatter = frontmatter_count == 1
                continue

            if in_frontmatter:
                if line.startswith("status:"):
                    status = line.split(":", 1)[1].strip()
                continue

            if line.startswith("# "):
                title = line[2:].strip()
                continue

            body_lines.append(line)

            phase_match = re.match(r"- \[([ x])\] (.+)", line)
            if phase_match:
                completed = phase_match.group(1) == "x"
                phase_name = phase_match.group(2).strip()
                phases.append(Phase(name=phase_name, completed=completed))

        return Ticket(
            id=ticket_id,
            title=title,
            status=status,
            body="\n".join(body_lines),
            phases=phases,
        )
