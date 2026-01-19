"""CLI entry point for programmator."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .loop import Loop
from .safety import ExitReason, SafetyConfig
from .tui import ProgrammatorTUI


def cmd_start(args: argparse.Namespace) -> int:
    config = SafetyConfig.from_env()
    if args.max_iterations:
        config.max_iterations = args.max_iterations

    return _run_tui(args.ticket_id, config, args.dir)


def _run_tui(ticket_id: str, config: SafetyConfig, working_dir: str | None) -> int:
    import threading

    from .loop import LoopResult

    app = ProgrammatorTUI()
    result_holder: list[LoopResult] = []

    def on_output(text: str) -> None:
        app.call_from_thread(app.write_output, text)

    def on_state_change(state, ticket, files_changed) -> None:
        app.call_from_thread(
            app.update_status,
            ticket=ticket,
            state=state,
            config=config,
            files_changed=files_changed,
        )

    loop = Loop(
        config=config,
        working_dir=working_dir,
        on_output=on_output,
        on_state_change=on_state_change,
        streaming=True,
    )

    def run_loop() -> None:
        try:
            result = loop.run(ticket_id)
            result_holder.append(result)
        finally:
            app.call_from_thread(app.exit)

    loop_thread = threading.Thread(target=run_loop, daemon=True)
    loop_thread.start()

    app.run()

    loop_thread.join(timeout=1.0)

    if result_holder:
        result = result_holder[0]
        print("\n[programmator] Session complete")
        print(f"  Exit reason: {result.exit_reason.value}")
        print(f"  Iterations: {result.iterations}")
        print(f"  Files changed: {len(result.total_files_changed)}")

        if result.exit_reason == ExitReason.COMPLETE:
            return 0
        elif result.exit_reason == ExitReason.USER_INTERRUPT:
            return 130
        return 1

    return 0


def cmd_status(args: argparse.Namespace) -> int:
    state_dir = Path("/tmp")
    active_sessions = list(state_dir.glob("programmator-*"))

    if not active_sessions:
        print("No active programmator sessions")
        return 0

    for session_dir in active_sessions:
        ticket_id = session_dir.name.replace("programmator-", "")
        iter_file = session_dir / "iteration"
        if iter_file.exists():
            iteration = iter_file.read_text().strip()
            print(f"  {ticket_id}: iteration {iteration}")
        else:
            print(f"  {ticket_id}: running")

    return 0


def cmd_logs(args: argparse.Namespace) -> int:
    state_dir = Path(f"/tmp/programmator-{args.ticket_id}")
    log_file = state_dir / "session.log"

    if not log_file.exists():
        print(f"No logs found for ticket {args.ticket_id}")
        return 1

    print(log_file.read_text())
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(
        prog="programmator",
        description="Ticket-driven autonomous Claude Code loop orchestrator",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    start_parser = subparsers.add_parser("start", help="Start loop on ticket")
    start_parser.add_argument("ticket_id", help="Ticket ID to work on")
    start_parser.add_argument(
        "-d",
        "--dir",
        help="Working directory for Claude",
        default=None,
    )
    start_parser.add_argument(
        "-n",
        "--max-iterations",
        type=int,
        help="Maximum iterations (default: 50)",
        default=None,
    )
    start_parser.set_defaults(func=cmd_start)

    status_parser = subparsers.add_parser("status", help="Show active loop status")
    status_parser.set_defaults(func=cmd_status)

    logs_parser = subparsers.add_parser("logs", help="Show execution logs")
    logs_parser.add_argument("ticket_id", help="Ticket ID to show logs for")
    logs_parser.set_defaults(func=cmd_logs)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
