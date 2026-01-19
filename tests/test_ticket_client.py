"""Tests for ticket_client module."""

from src.ticket_client import Phase, Ticket, TicketClient


class TestTicketParsing:
    def test_parses_ticket_with_phases(self):
        content = """---
id: p-test
status: open
type: task
priority: 2
---
# [programmator] Test ticket

## Goal
Test the parser.

## Design
- [ ] Phase 1: Investigation
- [ ] Phase 2: Implementation
- [x] Phase 0: Setup

## Notes
Some notes here.
"""
        client = TicketClient()
        ticket = client._parse_ticket("p-test", content)

        assert ticket.id == "p-test"
        assert ticket.title == "[programmator] Test ticket"
        assert ticket.status == "open"
        assert len(ticket.phases) == 3
        assert ticket.phases[0] == Phase(name="Phase 1: Investigation", completed=False)
        assert ticket.phases[1] == Phase(name="Phase 2: Implementation", completed=False)
        assert ticket.phases[2] == Phase(name="Phase 0: Setup", completed=True)

    def test_current_phase_returns_first_incomplete(self):
        ticket = Ticket(
            id="t-1",
            title="Test",
            status="open",
            body="",
            phases=[
                Phase(name="Phase 1", completed=True),
                Phase(name="Phase 2", completed=False),
                Phase(name="Phase 3", completed=False),
            ],
        )

        assert ticket.current_phase == Phase(name="Phase 2", completed=False)

    def test_current_phase_returns_none_when_all_complete(self):
        ticket = Ticket(
            id="t-1",
            title="Test",
            status="open",
            body="",
            phases=[
                Phase(name="Phase 1", completed=True),
                Phase(name="Phase 2", completed=True),
            ],
        )

        assert ticket.current_phase is None

    def test_all_phases_complete(self):
        ticket = Ticket(
            id="t-1",
            title="Test",
            status="open",
            body="",
            phases=[
                Phase(name="Phase 1", completed=True),
                Phase(name="Phase 2", completed=True),
            ],
        )

        assert ticket.all_phases_complete is True

    def test_not_all_phases_complete(self):
        ticket = Ticket(
            id="t-1",
            title="Test",
            status="open",
            body="",
            phases=[
                Phase(name="Phase 1", completed=True),
                Phase(name="Phase 2", completed=False),
            ],
        )

        assert ticket.all_phases_complete is False

    def test_parses_status_from_frontmatter(self):
        content = """---
status: in_progress
---
# Title

Body
"""
        client = TicketClient()
        ticket = client._parse_ticket("t-1", content)

        assert ticket.status == "in_progress"
