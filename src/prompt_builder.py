"""Build prompts from ticket content for Claude."""

from __future__ import annotations

from .ticket_client import Ticket

PROMPT_TEMPLATE = """You are working on ticket {ticket_id}: {title}

## Current State
{body}

## Progress Notes
{notes}

## Instructions
1. Read the ticket phases above (in the Design section)
2. Work on the FIRST uncompleted phase: [ ] (not [x])
3. Complete ONE phase per session - implement, test, verify
4. When done with the phase, output your status

## Current Phase
{current_phase}

## Session End Protocol
When you've completed your work for this iteration, you MUST end with exactly this block:

```
PROGRAMMATOR_STATUS:
  phase_completed: "{phase_name_placeholder}"
  status: CONTINUE
  files_changed:
    - file1.py
    - file2.py
  summary: "One line describing what you did"
```

Status values:
- CONTINUE: Phase done or in progress, more work remains
- DONE: ALL phases complete, project finished
- BLOCKED: Cannot proceed without human intervention (add error: field)

If blocked:
```
PROGRAMMATOR_STATUS:
  phase_completed: null
  status: BLOCKED
  files_changed: []
  summary: "What was attempted"
  error: "Description of what's blocking progress"
```
"""


def build_prompt(ticket: Ticket, notes: list[str] | None = None) -> str:
    current = ticket.current_phase
    current_phase_str = f"**{current.name}**" if current else "All phases complete"
    phase_name = current.name if current else "null"

    notes_str = "\n".join(f"- {note}" for note in (notes or [])) or "(No previous notes)"

    return PROMPT_TEMPLATE.format(
        ticket_id=ticket.id,
        title=ticket.title,
        body=ticket.body,
        notes=notes_str,
        current_phase=current_phase_str,
        phase_name_placeholder=phase_name,
    )
