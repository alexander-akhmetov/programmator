"""TUI interface for programmator using Textual."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING, ClassVar

from textual.app import App, ComposeResult
from textual.binding import BindingType
from textual.containers import Horizontal
from textual.widgets import Footer, RichLog, Static

if TYPE_CHECKING:
    from .loop import Loop
    from .safety import SafetyConfig, SafetyState
    from .ticket_client import Ticket


class StatusPanel(Static):
    """Left panel showing ticket and loop status."""

    def __init__(self, **kwargs) -> None:
        super().__init__(**kwargs)
        self._ticket: Ticket | None = None
        self._state: SafetyState | None = None
        self._config: SafetyConfig | None = None
        self._files_changed: list[str] = []
        self._start_time: float = time.time()
        self._paused: bool = False

    def update_status(
        self,
        ticket: Ticket | None = None,
        state: SafetyState | None = None,
        config: SafetyConfig | None = None,
        files_changed: list[str] | None = None,
        paused: bool | None = None,
    ) -> None:
        if ticket is not None:
            self._ticket = ticket
        if state is not None:
            self._state = state
        if config is not None:
            self._config = config
        if files_changed is not None:
            self._files_changed = files_changed
        if paused is not None:
            self._paused = paused
        self._render_status()

    def _format_elapsed(self) -> str:
        elapsed = int(time.time() - self._start_time)
        mins, secs = divmod(elapsed, 60)
        hours, mins = divmod(mins, 60)
        if hours:
            return f"{hours}h {mins}m"
        elif mins:
            return f"{mins}m {secs}s"
        return f"{secs}s"

    def _render_status(self) -> None:
        lines = []

        header = "[bold cyan]PROGRAMMATOR[/]"
        if self._paused:
            header += " [yellow][PAUSED][/]"
        lines.append(header)
        lines.append(f"[dim]Elapsed: {self._format_elapsed()}[/]")
        lines.append("")

        if self._ticket:
            lines.append(f"[bold]Ticket:[/] {self._ticket.id}")
            title = (
                self._ticket.title[:28] + "..."
                if len(self._ticket.title) > 31
                else self._ticket.title
            )
            lines.append(f"[dim]{title}[/]")
            lines.append("")

            lines.append("[bold]Phases:[/]")
            for phase in self._ticket.phases:
                check = "[green]✓[/]" if phase.completed else "[dim]○[/]"
                name = phase.name[:25] + "..." if len(phase.name) > 28 else phase.name
                lines.append(f"  {check} {name}")
            lines.append("")

        if self._state and self._config:
            lines.append("[bold]Progress:[/]")
            lines.append(f"  Iteration: {self._state.iteration}/{self._config.max_iterations}")
            if self._state.consecutive_no_changes > 0:
                lines.append(f"  [yellow]Stagnant:[/] {self._state.consecutive_no_changes}")
            if self._state.consecutive_errors > 0:
                lines.append(f"  [red]Errors:[/] {self._state.consecutive_errors}")
            lines.append("")

        if self._files_changed:
            lines.append(f"[bold]Files changed:[/] {len(self._files_changed)}")
            for f in self._files_changed[-5:]:
                short = f if len(f) < 28 else "..." + f[-25:]
                lines.append(f"  [dim]{short}[/]")
            if len(self._files_changed) > 5:
                lines.append(f"  [dim]...+{len(self._files_changed) - 5} more[/]")

        self.update("\n".join(lines))


class ProgrammatorTUI(App):
    """TUI application for programmator."""

    CSS = """
    #status {
        width: 35;
        padding: 1;
        border: solid green;
        background: $surface;
    }
    #logs {
        padding: 1;
        border: solid $primary;
    }
    """

    BINDINGS: ClassVar[list[BindingType]] = [
        ("q", "quit", "Quit"),
        ("ctrl+c", "quit", "Quit"),
        ("p", "toggle_pause", "Pause"),
        ("j", "scroll_down", "Down"),
        ("k", "scroll_up", "Up"),
        ("ctrl+d", "page_down", "Page Down"),
        ("ctrl+u", "page_up", "Page Up"),
        ("g", "scroll_top", "Top"),
        ("G", "scroll_bottom", "Bottom"),
    ]

    def __init__(self) -> None:
        super().__init__()
        self._loop: Loop | None = None

    def set_loop(self, loop: Loop) -> None:
        self._loop = loop

    def compose(self) -> ComposeResult:
        with Horizontal():
            yield StatusPanel(id="status")
            yield RichLog(id="logs", highlight=True, markup=True)
        yield Footer()

    def on_mount(self) -> None:
        self.query_one("#status", StatusPanel).update_status()
        self.set_interval(1, self._refresh_elapsed)

    def _refresh_elapsed(self) -> None:
        self.query_one("#status", StatusPanel)._render_status()

    def update_status(
        self,
        ticket: Ticket | None = None,
        state: SafetyState | None = None,
        config: SafetyConfig | None = None,
        files_changed: list[str] | None = None,
    ) -> None:
        self.query_one("#status", StatusPanel).update_status(
            ticket=ticket,
            state=state,
            config=config,
            files_changed=files_changed,
        )

    def write_output(self, text: str) -> None:
        log = self.query_one("#logs", RichLog)
        for line in text.splitlines():
            log.write(line)

    async def action_quit(self) -> None:
        self.exit()

    def action_toggle_pause(self) -> None:
        if self._loop:
            paused = self._loop.toggle_pause()
            self.query_one("#status", StatusPanel).update_status(paused=paused)
            status = "paused" if paused else "resumed"
            self.query_one("#logs", RichLog).write(f"[yellow]>>> Loop {status}[/]")

    def action_scroll_down(self) -> None:
        self.query_one("#logs", RichLog).scroll_relative(y=1)

    def action_scroll_up(self) -> None:
        self.query_one("#logs", RichLog).scroll_relative(y=-1)

    def action_page_down(self) -> None:
        log = self.query_one("#logs", RichLog)
        log.scroll_relative(y=log.size.height // 2)

    def action_page_up(self) -> None:
        log = self.query_one("#logs", RichLog)
        log.scroll_relative(y=-log.size.height // 2)

    def action_scroll_top(self) -> None:
        self.query_one("#logs", RichLog).scroll_home()

    def action_scroll_bottom(self) -> None:
        self.query_one("#logs", RichLog).scroll_end()
