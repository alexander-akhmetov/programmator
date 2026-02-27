package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ANSI color codes matching the old lipgloss styles.
const (
	colorOrange  = 208 // prog prefix
	colorGreen   = 42  // diff add, running state
	colorRed     = 196 // diff del, stopped state
	colorCyan    = 117 // diff hunk, review, file paths
	colorDim     = 241 // labels, tool, diff context
	colorDimmer  = 245 // tool result
	colorWhite   = 255 // values
	colorMagenta = 205 // title, current phase
	colorPink    = 212 // phase arrow
)

// Writer prints events to stdout and redraws a sticky footer in TTY mode.
// In non-TTY mode, it prints plain text without ANSI escapes or footer.
type Writer struct {
	out         io.Writer
	isTTY       bool
	width       int
	mu          sync.Mutex
	renderer    *glamour.TermRenderer
	footerLines int
	lastFooter  []string // last rendered footer lines for redraw
	pid         int
	memKB       int64
}

// NewWriter creates a Writer. If width is <= 0, defaults to 80.
func NewWriter(out io.Writer, isTTY bool, width int) *Writer {
	if width <= 0 {
		width = 80
	}

	w := &Writer{
		out:   out,
		isTTY: isTTY,
		width: width,
	}

	if isTTY {
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle("dark"),
			glamour.WithWordWrap(max(width-6, 40)),
		)
		if err == nil {
			w.renderer = r
		}
	}

	return w
}

// WriteEvent prints a single event to the output stream.
func (w *Writer) WriteEvent(ev event.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.eraseFooter()

	var line string
	switch ev.Kind {
	case event.KindProg:
		line = w.formatProg(ev.Text)
	case event.KindToolUse:
		line = w.formatTool(ev.Text)
	case event.KindToolResult:
		line = w.formatToolResult(ev.Text)
	case event.KindReview:
		line = w.formatReview(ev.Text)
	case event.KindDiffAdd:
		line = w.formatDiffAdd(ev.Text)
	case event.KindDiffDel:
		line = w.formatDiffDel(ev.Text)
	case event.KindDiffCtx:
		line = w.formatDiffCtx(ev.Text)
	case event.KindDiffHunk:
		line = w.formatDiffHunk(ev.Text)
	case event.KindMarkdown:
		line = w.formatMarkdown(ev.Text)
	case event.KindIterationSeparator:
		line = w.formatIterSep(ev.Text)
	}

	fmt.Fprintln(w.out, line)
	w.redrawFooter()
}

// UpdateFooter redraws the sticky footer with current state.
func (w *Writer) UpdateFooter(state *safety.State, item *domain.WorkItem, cfg safety.Config) {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.eraseFooter()

	lines := w.buildFooter(state, item, cfg)
	w.lastFooter = lines
	w.footerLines = len(lines)

	for _, line := range lines {
		fmt.Fprintln(w.out, line)
	}
}

// ClearFooter erases the sticky footer from the terminal.
func (w *Writer) ClearFooter() {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	w.eraseFooter()
	w.footerLines = 0
	w.lastFooter = nil
}

// SetProcessStats updates the PID and memory fields used by the footer.
func (w *Writer) SetProcessStats(pid int, memKB int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pid = pid
	w.memKB = memKB
}

// eraseFooter moves cursor up and clears the footer lines. Must be called with mu held.
func (w *Writer) eraseFooter() {
	if w.footerLines == 0 || !w.isTTY {
		return
	}
	// Move cursor up N lines and clear each line
	for range w.footerLines {
		fmt.Fprint(w.out, "\033[A\033[2K")
	}
}

// redrawFooter redraws the last-known footer after an event line was printed.
// Must be called with mu held.
func (w *Writer) redrawFooter() {
	if len(w.lastFooter) == 0 || !w.isTTY {
		return
	}
	for _, line := range w.lastFooter {
		fmt.Fprintln(w.out, line)
	}
	w.footerLines = len(w.lastFooter)
}

// buildFooter composes the footer lines.
func (w *Writer) buildFooter(state *safety.State, item *domain.WorkItem, cfg safety.Config) []string {
	var lines []string

	// Separator
	sep := strings.Repeat("─", min(w.width, 80))
	lines = append(lines, w.style(colorDim, sep))

	// Status line: ID | iteration | stagnation | files
	var parts []string
	if item != nil {
		parts = append(parts, w.styleBold(colorMagenta, item.ID))
	}
	if state != nil {
		parts = append(parts, fmt.Sprintf("iter %s",
			w.style(colorWhite, fmt.Sprintf("%d/%d", state.Iteration, cfg.MaxIterations))))
		parts = append(parts, fmt.Sprintf("stag %s",
			w.style(colorWhite, fmt.Sprintf("%d/%d", state.ConsecutiveNoChanges, cfg.StagnationLimit))))
		parts = append(parts, fmt.Sprintf("files %s",
			w.style(colorWhite, fmt.Sprintf("%d", len(state.TotalFilesChanged)))))
	}
	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, w.style(colorDim, " | ")))
	}

	// Current phase
	if item != nil {
		if phase := item.CurrentPhase(); phase != nil {
			lines = append(lines, w.style(colorPink, "-> ")+w.styleBold(colorPink, phase.Name))
		} else if item.AllPhasesComplete() {
			lines = append(lines, w.style(colorGreen, "all phases complete"))
		}
	}

	// Process stats
	if w.pid > 0 {
		lines = append(lines, w.style(colorDim, fmt.Sprintf("pid %d | %s", w.pid, formatMemory(w.memKB))))
	}

	return lines
}

// Formatting methods per event kind.

func (w *Writer) formatProg(text string) string {
	prefix := "programmator: "
	if w.isTTY {
		return fgBold(colorOrange, "▶ "+prefix) + text
	}
	return prefix + text
}

func (w *Writer) formatTool(text string) string {
	if w.isTTY {
		return fg(colorDim, "> "+text)
	}
	return "> " + text
}

func (w *Writer) formatToolResult(text string) string {
	if w.isTTY {
		return fg(colorDimmer, text)
	}
	return text
}

func (w *Writer) formatReview(text string) string {
	if w.isTTY {
		return fg(colorCyan, text)
	}
	return text
}

func (w *Writer) formatDiffAdd(text string) string {
	if w.isTTY {
		return fg(colorGreen, text)
	}
	return text
}

func (w *Writer) formatDiffDel(text string) string {
	if w.isTTY {
		return fg(colorRed, text)
	}
	return text
}

func (w *Writer) formatDiffCtx(text string) string {
	if w.isTTY {
		return fg(colorDim, text)
	}
	return text
}

func (w *Writer) formatDiffHunk(text string) string {
	if w.isTTY {
		return fg(colorCyan, text)
	}
	return text
}

func (w *Writer) formatMarkdown(text string) string {
	if w.renderer != nil {
		if rendered, err := w.renderer.Render(text); err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}

func (w *Writer) formatIterSep(text string) string {
	if w.isTTY {
		return bold(text)
	}
	return text
}

// style wraps text with 256-color foreground in TTY mode, plain in non-TTY.
func (w *Writer) style(color int, text string) string {
	if w.isTTY {
		return fg(color, text)
	}
	return text
}

// styleBold wraps text with 256-color foreground and bold in TTY mode.
func (w *Writer) styleBold(color int, text string) string {
	if w.isTTY {
		return fgBold(color, text)
	}
	return text
}
