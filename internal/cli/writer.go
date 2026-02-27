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
	colorOrange  = 208 // prog prefix, footer separator
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
//
// When terminal height is known, the footer uses a scroll region (DECSTBM)
// so content scrolls independently and the footer stays visible at all times,
// including during fine-grained streaming output.
type Writer struct {
	out      io.Writer
	isTTY    bool
	width    int
	height   int // terminal height in rows (0 = unknown)
	mu       sync.Mutex
	renderer *glamour.TermRenderer

	// Footer state.
	footerLines int      // number of footer lines currently on screen
	lastFooter  []string // last rendered footer content for redraw
	midLine     bool     // cursor is mid-line from streaming text

	// Scroll region mode (used when height > 0).
	scrollSet bool // whether DECSTBM scroll region is active

	pid   int
	memKB int64
}

// NewWriter creates a Writer. If width is <= 0, defaults to 80.
func NewWriter(out io.Writer, isTTY bool, width, height int) *Writer {
	if width <= 0 {
		width = 80
	}

	w := &Writer{
		out:    out,
		isTTY:  isTTY,
		width:  width,
		height: height,
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

// useScrollRegion reports whether the writer should use DECSTBM scroll regions.
func (w *Writer) useScrollRegion() bool {
	return w.isTTY && w.height > 0
}

// WriteEvent prints a single event to the output stream.
func (w *Writer) WriteEvent(ev event.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Legacy mode: erase footer before printing (no scroll region).
	if !w.useScrollRegion() {
		w.legacyEraseFooter()
	}

	// Streaming text: print inline without trailing newline.
	if ev.Kind == event.KindStreamingText {
		fmt.Fprint(w.out, ev.Text)
		if strings.HasSuffix(ev.Text, "\n") {
			w.midLine = false
		} else {
			w.midLine = true
		}
		// Legacy mode: clear footer tracking (footer is gone).
		if !w.useScrollRegion() {
			w.footerLines = 0
		}
		return
	}

	// Transition from mid-line streaming to a structured event.
	if w.midLine {
		fmt.Fprintln(w.out)
		w.midLine = false
	}

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
	case event.KindStreamingText:
		// handled above; unreachable
	}

	fmt.Fprintln(w.out, line)

	// Legacy mode: redraw footer after the event line.
	if !w.useScrollRegion() {
		w.legacyRedrawFooter()
	}
}

// UpdateFooter redraws the sticky footer with current state.
func (w *Writer) UpdateFooter(state *safety.State, item *domain.WorkItem, cfg safety.Config) {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	lines := w.buildFooter(state, item, cfg)
	w.lastFooter = lines
	footerCount := len(lines)

	if w.useScrollRegion() {
		w.drawFooterScrollRegion(lines, footerCount)
	} else {
		w.legacyEraseFooter()
		w.footerLines = footerCount
		for _, line := range lines {
			fmt.Fprintln(w.out, line)
		}
	}
}

// ClearFooter clears the footer overlay and resets the scroll region,
// leaving all content output visible. The cursor is positioned at the
// end of the content area so subsequent output (e.g. summary) prints cleanly.
func (w *Writer) ClearFooter() {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.useScrollRegion() && w.scrollSet {
		scrollBottom := w.height - w.footerLines

		// Erase footer lines at their absolute positions.
		for i := range w.footerLines {
			row := scrollBottom + 1 + i
			fmt.Fprintf(w.out, "\033[%d;1H\033[2K", row)
		}

		// Reset scroll region and position cursor at end of content area.
		fmt.Fprint(w.out, "\033[r")                    // reset DECSTBM
		fmt.Fprintf(w.out, "\033[%d;1H", scrollBottom) // move to last content row
		fmt.Fprintln(w.out)                            // advance to fresh line
		w.scrollSet = false
	} else {
		w.legacyEraseFooter()
	}

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

// --- Scroll region footer ---

// drawFooterScrollRegion sets the DECSTBM scroll region and draws footer
// at absolute row positions below the region.
func (w *Writer) drawFooterScrollRegion(lines []string, footerCount int) {
	scrollBottom := w.height - footerCount

	// Set or adjust scroll region when footer height changes.
	if !w.scrollSet || w.footerLines != footerCount {
		if w.scrollSet {
			// Already in scroll-region mode: cursor is within the region,
			// so save/restore is safe across the DECSTBM change.
			fmt.Fprint(w.out, "\0337")                     // DEC save cursor
			fmt.Fprintf(w.out, "\033[1;%dr", scrollBottom) // DECSTBM
			fmt.Fprint(w.out, "\0338")                     // DEC restore cursor
		} else {
			// First activation: the cursor may already be in what is now
			// the footer area (e.g. output filled the screen before footer
			// existed). Scroll content up to make room, then set the region
			// and place the cursor at the bottom of the content area.
			for range footerCount {
				fmt.Fprint(w.out, "\n") // push content up
			}
			fmt.Fprintf(w.out, "\033[1;%dr", scrollBottom) // DECSTBM
			fmt.Fprintf(w.out, "\033[%d;1H", scrollBottom) // cursor to content bottom
		}
		w.scrollSet = true
	}

	w.footerLines = footerCount

	// Draw footer at absolute positions below the scroll region.
	fmt.Fprint(w.out, "\0337") // DEC save cursor
	for i, line := range lines {
		row := scrollBottom + 1 + i
		fmt.Fprintf(w.out, "\033[%d;1H\033[2K%s", row, line)
	}
	fmt.Fprint(w.out, "\0338") // DEC restore cursor
}

// --- Legacy footer (no scroll region) ---

// legacyEraseFooter moves cursor up and clears footer lines. Must be called with mu held.
func (w *Writer) legacyEraseFooter() {
	if w.footerLines == 0 || !w.isTTY {
		return
	}
	for range w.footerLines {
		fmt.Fprint(w.out, "\033[A\033[2K")
	}
}

// legacyRedrawFooter redraws the last-known footer after an event line.
// Must be called with mu held.
func (w *Writer) legacyRedrawFooter() {
	if len(w.lastFooter) == 0 || !w.isTTY {
		return
	}
	for _, line := range w.lastFooter {
		fmt.Fprintln(w.out, line)
	}
	w.footerLines = len(w.lastFooter)
}

// --- Footer content ---

// buildFooter composes the footer lines.
func (w *Writer) buildFooter(state *safety.State, item *domain.WorkItem, cfg safety.Config) []string {
	var lines []string

	// Orange separator line (like Pi's status bar separator).
	sep := strings.Repeat("─", w.width)
	lines = append(lines, w.style(colorOrange, sep))

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
