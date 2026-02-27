package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

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

	maxFrameHistoryRows = 6000
)

type frameRow struct {
	text string
	kind event.Kind
}

// Writer prints events to stdout and redraws a sticky footer in TTY mode.
// In non-TTY mode, it prints plain text without ANSI escapes or footer.
//
// In TTY mode with known terminal height, Writer uses a viewport model:
// it stores content rows and footer rows, then redraws the full frame.
// This keeps the footer stable independently of event source/executor.
type Writer struct {
	out      io.Writer
	isTTY    bool
	width    int
	height   int // terminal height in rows (0 = unknown)
	mu       sync.Mutex
	renderer *glamour.TermRenderer

	// Footer state (used by both viewport and legacy mode).
	footerLines int
	lastFooter  []string

	// Legacy mode state (TTY with unknown height).
	midLine bool

	// Viewport state (TTY with known height).
	frameRows    []frameRow
	frameCurrent frameRow
	frameClosed  bool

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

// useFrameRenderer reports whether the writer should render a full viewport
// with sticky footer and internal scrolling.
func (w *Writer) useFrameRenderer() bool {
	return w.isTTY && w.height > 0 && !w.frameClosed
}

func (w *Writer) colorEnabled() bool {
	return w.isTTY && !w.useFrameRenderer()
}

// WriteEvent prints a single event to the output stream.
func (w *Writer) WriteEvent(ev event.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.frameClosed {
		return
	}

	ev.Text = sanitizeTerminalText(ev.Text)

	if w.useFrameRenderer() {
		w.writeEventFrame(ev)
		w.renderFrame()
		return
	}

	// Legacy mode: erase footer before printing.
	w.legacyEraseFooter()

	// Streaming text: print inline without trailing newline.
	if ev.Kind == event.KindStreamingText {
		fmt.Fprint(w.out, ev.Text)
		w.midLine = !strings.HasSuffix(ev.Text, "\n")
		w.footerLines = 0
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
	w.legacyRedrawFooter()
}

func (w *Writer) writeEventFrame(ev event.Event) {
	if ev.Kind == event.KindStreamingText {
		w.appendFrameText(ev.Text, event.KindStreamingText)
		return
	}

	if w.frameCurrent.text != "" {
		w.pushFrameRow(w.frameCurrent)
		w.frameCurrent = frameRow{}
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

	w.appendFrameText(line, ev.Kind)
	w.appendFrameText("\n", ev.Kind)
}

func (w *Writer) appendFrameText(text string, kind event.Kind) {
	if w.frameCurrent.text != "" && w.frameCurrent.kind != kind {
		w.pushFrameRow(w.frameCurrent)
		w.frameCurrent = frameRow{}
	}

	for {
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			w.appendFrameChunk(text, kind)
			return
		}

		w.appendFrameChunk(text[:idx], kind)
		w.pushFrameRow(w.frameCurrent)
		w.frameCurrent = frameRow{}
		text = text[idx+1:]
	}
}

func (w *Writer) appendFrameChunk(chunk string, kind event.Kind) {
	if chunk == "" {
		if w.frameCurrent.text == "" {
			w.frameCurrent.kind = kind
		}
		return
	}

	w.frameCurrent.kind = kind

	r := []rune(w.frameCurrent.text + chunk)
	for len(r) > w.width {
		w.pushFrameRow(frameRow{text: string(r[:w.width]), kind: kind})
		r = r[w.width:]
	}
	w.frameCurrent = frameRow{text: string(r), kind: kind}
}

func (w *Writer) pushFrameRow(row frameRow) {
	row.text = trimRunes(row.text, w.width)
	w.frameRows = append(w.frameRows, row)
	if len(w.frameRows) > maxFrameHistoryRows {
		drop := len(w.frameRows) - maxFrameHistoryRows
		w.frameRows = w.frameRows[drop:]
	}
}

func (w *Writer) renderFrame() {
	if !w.useFrameRenderer() {
		return
	}

	footerCount := len(w.lastFooter)
	contentHeight := max(w.height-footerCount, 1)

	// Always keep one editable content line (frameCurrent) visible.
	totalContentRows := len(w.frameRows) + 1
	start := max(totalContentRows-contentHeight, 0)

	var buf strings.Builder
	buf.WriteString("\033[?25l") // hide cursor while redrawing

	for row := 1; row <= w.height; row++ {
		fmt.Fprintf(&buf, "\033[%d;1H\033[2K", row)

		var line string
		if row <= contentHeight {
			idx := start + (row - 1)
			switch {
			case idx < len(w.frameRows):
				segment := w.frameRows[idx]
				line = w.applyFrameContentStyle(segment.kind, segment.text)
			case idx == len(w.frameRows):
				line = w.applyFrameContentStyle(w.frameCurrent.kind, w.frameCurrent.text)
			}
		} else {
			fidx := row - contentHeight - 1
			if fidx >= 0 && fidx < len(w.lastFooter) {
				line = w.applyFrameFooterStyle(fidx, w.lastFooter[fidx])
			}
		}

		if line != "" {
			buf.WriteString(line)
		}
	}

	cursorRow := len(w.frameRows) - start + 1
	cursorRow = max(cursorRow, 1)
	cursorRow = min(cursorRow, contentHeight)

	cursorCol := utf8.RuneCountInString(w.frameCurrent.text) + 1
	cursorCol = max(cursorCol, 1)
	cursorCol = min(cursorCol, max(w.width, 1))

	fmt.Fprintf(&buf, "\033[%d;%dH", cursorRow, cursorCol)
	buf.WriteString("\033[?25h") // show cursor

	fmt.Fprint(w.out, buf.String())
}

// UpdateFooter redraws the sticky footer with current state.
func (w *Writer) UpdateFooter(state *safety.State, item *domain.WorkItem, cfg safety.Config) {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.frameClosed {
		return
	}

	lines := w.buildFooter(state, item, cfg)

	if w.useFrameRenderer() {
		maxFooterLines := max(w.height-1, 0)
		if maxFooterLines == 0 {
			return
		}

		normalized := make([]string, 0, len(lines))
		for _, line := range lines {
			normalized = append(normalized, trimRunes(sanitizeTerminalText(line), w.width))
		}
		if len(normalized) > maxFooterLines {
			normalized = normalized[:maxFooterLines]
		}

		w.lastFooter = normalized
		w.footerLines = len(normalized)
		w.renderFrame()
		return
	}

	w.lastFooter = lines
	w.legacyEraseFooter()
	w.footerLines = len(lines)
	for _, line := range lines {
		fmt.Fprintln(w.out, line)
	}
}

// ClearFooter clears the footer overlay.
func (w *Writer) ClearFooter() {
	if !w.isTTY {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.useFrameRenderer() {
		w.lastFooter = nil
		w.footerLines = 0
		w.renderFrame()
		w.frameClosed = true
		fmt.Fprint(w.out, "\n")
		return
	}

	w.legacyEraseFooter()
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

// --- Legacy footer (no viewport mode) ---

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
	if w.colorEnabled() {
		return fgBold(colorOrange, "▶ "+prefix) + text
	}
	if w.isTTY {
		return "▶ " + prefix + text
	}
	return prefix + text
}

func (w *Writer) formatTool(text string) string {
	if w.colorEnabled() {
		return fg(colorDim, "> "+text)
	}
	return "> " + text
}

func (w *Writer) formatToolResult(text string) string {
	if w.colorEnabled() {
		return fg(colorDimmer, text)
	}
	return text
}

func (w *Writer) formatReview(text string) string {
	if w.colorEnabled() {
		return fg(colorCyan, text)
	}
	return text
}

func (w *Writer) formatDiffAdd(text string) string {
	if w.colorEnabled() {
		return fg(colorGreen, text)
	}
	return text
}

func (w *Writer) formatDiffDel(text string) string {
	if w.colorEnabled() {
		return fg(colorRed, text)
	}
	return text
}

func (w *Writer) formatDiffCtx(text string) string {
	if w.colorEnabled() {
		return fg(colorDim, text)
	}
	return text
}

func (w *Writer) formatDiffHunk(text string) string {
	if w.colorEnabled() {
		return fg(colorCyan, text)
	}
	return text
}

func (w *Writer) formatMarkdown(text string) string {
	if w.renderer != nil && !w.useFrameRenderer() {
		if rendered, err := w.renderer.Render(text); err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}

func (w *Writer) formatIterSep(text string) string {
	if w.colorEnabled() {
		return bold(text)
	}
	return text
}

func (w *Writer) applyFrameContentStyle(kind event.Kind, text string) string {
	if !w.isTTY || text == "" {
		return text
	}

	switch kind {
	case event.KindProg:
		return fgBold(colorOrange, text)
	case event.KindToolUse:
		return fg(colorDim, text)
	case event.KindToolResult:
		return fg(colorDimmer, text)
	case event.KindReview:
		return fg(colorCyan, text)
	case event.KindDiffAdd:
		return fg(colorGreen, text)
	case event.KindDiffDel:
		return fg(colorRed, text)
	case event.KindDiffCtx:
		return fg(colorDim, text)
	case event.KindDiffHunk:
		return fg(colorCyan, text)
	case event.KindIterationSeparator:
		return bold(text)
	default:
		return text
	}
}

func (w *Writer) applyFrameFooterStyle(index int, text string) string {
	if !w.isTTY || text == "" {
		return text
	}

	switch index {
	case 0:
		return fg(colorOrange, text)
	case 1:
		return fg(colorDim, text)
	case 2:
		if strings.Contains(text, "all phases complete") {
			return fg(colorGreen, text)
		}
		return fg(colorPink, text)
	default:
		return fg(colorDim, text)
	}
}

// style wraps text with 256-color foreground in color-enabled mode, plain otherwise.
func (w *Writer) style(color int, text string) string {
	if w.colorEnabled() {
		return fg(color, text)
	}
	return text
}

// styleBold wraps text with 256-color foreground and bold in color-enabled mode.
func (w *Writer) styleBold(color int, text string) string {
	if w.colorEnabled() {
		return fgBold(color, text)
	}
	return text
}

func trimRunes(s string, width int) string {
	if width <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width])
}

// sanitizeTerminalText removes control sequences that can move the cursor or
// otherwise disrupt sticky-footer rendering.
func sanitizeTerminalText(text string) string {
	if text == "" {
		return text
	}

	// Normalize control characters that rewrite in place.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	text = stripANSISequences(text)

	var b strings.Builder
	b.Grow(len(text))
	for i := range len(text) {
		ch := text[i]
		// Drop remaining C0 controls except LF.
		if ch < 0x20 && ch != '\n' {
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

// stripANSISequences removes common ANSI control sequences (CSI/OSC/ESC).
func stripANSISequences(text string) string {
	if !strings.ContainsRune(text, '\x1b') {
		return text
	}

	var b strings.Builder
	b.Grow(len(text))

	for i := 0; i < len(text); i++ {
		if text[i] != '\x1b' {
			b.WriteByte(text[i])
			continue
		}

		// ESC at end of input: ignore it.
		if i+1 >= len(text) {
			break
		}

		switch text[i+1] {
		case '[':
			// CSI sequence: ESC [ ... final-byte(0x40-0x7E)
			i += 2
			for ; i < len(text); i++ {
				if text[i] >= 0x40 && text[i] <= 0x7e {
					break
				}
			}
		case ']':
			// OSC sequence: ESC ] ... BEL or ST (ESC \\)
			i += 2
			for ; i < len(text); i++ {
				if text[i] == '\a' {
					break
				}
				if text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '\\' {
					i++
					break
				}
			}
		default:
			// Other single-character ESC sequence.
			i++
		}
	}

	return b.String()
}
