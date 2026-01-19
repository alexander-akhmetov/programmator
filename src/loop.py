"""Main orchestration loop for programmator."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time
from collections.abc import Callable
from dataclasses import dataclass

from .prompt_builder import build_prompt
from .response_parser import Status, parse_response
from .safety import ExitReason, SafetyConfig, SafetyState, check_safety
from .ticket_client import Ticket, TicketClient


@dataclass
class LoopResult:
    exit_reason: ExitReason
    iterations: int
    total_files_changed: list[str]


class Loop:
    def __init__(
        self,
        ticket_client: TicketClient | None = None,
        config: SafetyConfig | None = None,
        working_dir: str | None = None,
        on_output: Callable[[str], None] | None = None,
        on_state_change: Callable[[SafetyState, Ticket, list[str]], None] | None = None,
        streaming: bool = False,
    ):
        self.ticket_client = ticket_client or TicketClient()
        self.config = config or SafetyConfig.from_env()
        self.working_dir = working_dir
        self.on_output = on_output
        self.on_state_change = on_state_change
        self.streaming = streaming
        self.state = SafetyState()
        self._stop_requested = False
        self._paused = False

    def request_stop(self) -> None:
        self._stop_requested = True

    def toggle_pause(self) -> bool:
        """Toggle pause state. Returns new paused state."""
        self._paused = not self._paused
        return self._paused

    @property
    def is_paused(self) -> bool:
        return self._paused

    def _notify_state_change(self, ticket: Ticket, files_changed: list[str]) -> None:
        if self.on_state_change:
            self.on_state_change(self.state, ticket, files_changed)

    def run(self, ticket_id: str) -> LoopResult:
        ticket = self.ticket_client.get(ticket_id)
        self._log(f"Starting on ticket {ticket_id}: {ticket.title}")
        self.ticket_client.set_status(ticket_id, "in_progress")

        all_files_changed: list[str] = []
        notes: list[str] = []

        self._notify_state_change(ticket, all_files_changed)

        while not self._stop_requested:
            while self._paused and not self._stop_requested:
                time.sleep(0.1)

            ticket = self.ticket_client.get(ticket_id)

            if ticket.all_phases_complete:
                self._log("All phases complete!")
                self.ticket_client.set_status(ticket_id, "closed")
                self.ticket_client.add_note(
                    ticket_id,
                    f"progress: Completed all phases in {self.state.iteration} iterations",
                )
                return LoopResult(
                    exit_reason=ExitReason.COMPLETE,
                    iterations=self.state.iteration,
                    total_files_changed=all_files_changed,
                )

            safety_exit = check_safety(self.config, self.state)
            if safety_exit:
                self._log(f"Safety exit: {safety_exit.value}")
                msg = f"error: Safety exit after {self.state.iteration} iters: {safety_exit.value}"
                self.ticket_client.add_note(ticket_id, msg)
                return LoopResult(
                    exit_reason=safety_exit,
                    iterations=self.state.iteration,
                    total_files_changed=all_files_changed,
                )

            current_phase = ticket.current_phase
            self._log(f"Iteration {self.state.iteration + 1}/{self.config.max_iterations}")
            self._log(f"Current phase: {current_phase.name if current_phase else 'None'}")

            prompt = build_prompt(ticket, notes)
            self._log("Invoking Claude...")

            output = self._invoke_claude(prompt)

            status = parse_response(output)

            if status is None:
                self._log("Warning: No PROGRAMMATOR_STATUS found in output")
                self.state.record_iteration([], "no_status_block")
                self._notify_state_change(ticket, all_files_changed)
                notes.append(f"[iter {self.state.iteration}] No status block returned")
                continue

            self._log(f"Status: {status.status.value}")
            self._log(f"Summary: {status.summary}")

            if status.phase_completed:
                self._log(f"Phase completed: {status.phase_completed}")
                self.ticket_client.update_phase(ticket_id, status.phase_completed, True)
                notes.append(f"[iter {self.state.iteration}] Completed: {status.phase_completed}")
                self.ticket_client.add_note(
                    ticket_id,
                    f"progress: [iter {self.state.iteration}] Completed {status.phase_completed}",
                )
            else:
                notes.append(f"[iter {self.state.iteration}] {status.summary}")
                self.ticket_client.add_note(
                    ticket_id,
                    f"progress: [iter {self.state.iteration}] {status.summary}",
                )

            if status.files_changed:
                self._log(f"Files changed: {', '.join(status.files_changed)}")
                all_files_changed.extend(status.files_changed)

            self.state.record_iteration(
                status.files_changed,
                status.error,
            )
            self._notify_state_change(ticket, all_files_changed)

            if status.status == Status.DONE:
                self._log("Claude reported DONE")
                self.ticket_client.set_status(ticket_id, "closed")
                self.ticket_client.add_note(
                    ticket_id,
                    f"progress: Completed in {self.state.iteration} iterations",
                )
                return LoopResult(
                    exit_reason=ExitReason.COMPLETE,
                    iterations=self.state.iteration,
                    total_files_changed=all_files_changed,
                )

            if status.status == Status.BLOCKED:
                self._log(f"Claude reported BLOCKED: {status.error}")
                self.ticket_client.add_note(
                    ticket_id,
                    f"error: [iter {self.state.iteration}] BLOCKED: {status.error}",
                )
                return LoopResult(
                    exit_reason=ExitReason.BLOCKED,
                    iterations=self.state.iteration,
                    total_files_changed=all_files_changed,
                )

        self._log("Stop requested by user")
        self.ticket_client.add_note(
            ticket_id,
            f"progress: Stopped by user after {self.state.iteration} iterations",
        )
        return LoopResult(
            exit_reason=ExitReason.USER_INTERRUPT,
            iterations=self.state.iteration,
            total_files_changed=all_files_changed,
        )

    def _invoke_claude(self, prompt: str) -> str:
        claude_flags = os.environ.get("PROGRAMMATOR_CLAUDE_FLAGS", "--dangerously-skip-permissions")

        cmd = ["claude", "--print", *claude_flags.split()]

        if self.streaming:
            cmd.extend(["--output-format", "stream-json", "--verbose"])

        try:
            process = subprocess.Popen(
                cmd,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                cwd=self.working_dir,
            )

            assert process.stdin is not None
            assert process.stdout is not None
            process.stdin.write(prompt)
            process.stdin.close()

            if self.streaming:
                return self._process_streaming_output(process)
            else:
                return self._process_text_output(process)

        except subprocess.TimeoutExpired:
            process.kill()
            return (
                "PROGRAMMATOR_STATUS:\n"
                "  phase_completed: null\n"
                "  status: BLOCKED\n"
                "  files_changed: []\n"
                '  summary: "Timeout"\n'
                '  error: "Claude invocation timed out"'
            )

    def _process_text_output(self, process: subprocess.Popen) -> str:
        assert process.stdout is not None
        output_lines = []
        for line in process.stdout:
            if self.on_output:
                self.on_output(line)
            else:
                print(line, end="", flush=True)
            output_lines.append(line)

        process.wait(timeout=self.config.timeout_seconds)
        return "".join(output_lines)

    def _process_streaming_output(self, process: subprocess.Popen) -> str:
        assert process.stdout is not None
        full_output: list[str] = []

        for line in process.stdout:
            line = line.strip()
            if not line:
                continue

            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue

            event_type = event.get("type")

            if event_type == "assistant":
                message = event.get("message", {})
                content = message.get("content", [])
                for block in content:
                    if block.get("type") == "text":
                        text = block.get("text", "")
                        if text:
                            full_output.append(text)
                            if self.on_output:
                                self.on_output(text)
                            else:
                                print(text, end="", flush=True)

            elif event_type == "result":
                result_text = event.get("result", "")
                if result_text and not full_output:
                    full_output.append(result_text)

        process.wait(timeout=self.config.timeout_seconds)
        return "".join(full_output)

    def _log(self, message: str) -> None:
        print(f"[programmator] {message}", file=sys.stderr)
