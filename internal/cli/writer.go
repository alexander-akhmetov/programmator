package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

// ANSI 256-color codes approximating Flat UI Colors "DEFO" palette.
const (
	colorOrange  = 214 // Orange (#f39c12)
	colorGreen   = 41  // Emerald (#2ecc71)
	colorRed     = 203 // Alizarin (#e74c3c)
	colorCyan    = 68  // Peter River (#3498db)
	colorDim     = 102 // Asbestos (#7f8c8d)
	colorDimmer  = 109 // Concrete (#95a5a6)
	colorWhite   = 255 // Clouds (#ecf0f1)
	colorMagenta = 134 // Amethyst (#9b59b6)
	colorPink    = 97  // Wisteria (#8e44ad)

	footerIDPrefixChars = 12
)

type bubbleFooterMsg struct {
	lines []string
}

type bubbleModel struct {
	footer []string
	ready  chan struct{}
	once   sync.Once
}

func (m *bubbleModel) Init() tea.Cmd {
	return func() tea.Msg {
		if m.ready != nil {
			m.once.Do(func() { close(m.ready) })
		}
		return nil
	}
}

func (m *bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(bubbleFooterMsg); ok {
		m.footer = append([]string(nil), msg.lines...)
	}
	return m, nil
}

func (m *bubbleModel) View() string {
	return strings.Join(m.footer, "\n")
}

// Writer prints events to stdout and redraws a sticky footer in TTY mode.
// In non-TTY mode, it prints plain text without ANSI escapes or footer.
//
// In TTY mode, Writer uses inline Bubble Tea mode (no alt screen):
// - content is printed above the program via tea.Printf/tea.Println
// - View() renders sticky footer at the bottom
// - terminal scrollback remains standard.
type Writer struct {
	out      io.Writer
	isTTY    bool
	width    int
	height   int // terminal height in rows (0 = unknown)
	mu       sync.Mutex
	renderer *glamour.TermRenderer

	footerLines int
	lastFooter  []string
	midLine     bool
	pendingLine string

	pid            int
	executorName   string
	claudeConfigDir string

	useTea    bool
	tea       *tea.Program
	teaDone   chan struct{}
	teaActive bool
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
		useTea: isTTY,
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

func (w *Writer) colorEnabled() bool {
	return w.isTTY
}

func (w *Writer) ensureTeaLocked() {
	if !w.useTea || !w.isTTY || w.teaActive {
		return
	}

	ready := make(chan struct{})
	model := &bubbleModel{ready: ready}
	p := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(w.out),
		// Let programmator's signal.NotifyContext own SIGINT/SIGTERM handling.
		tea.WithoutSignalHandler(),
	)
	done := make(chan struct{})

	go func() {
		_, _ = p.Run()
		close(done)
	}()

	select {
	case <-ready:
		w.tea = p
		w.teaDone = done
		w.teaActive = true
	case <-done:
		w.useTea = false
	case <-time.After(2 * time.Second):
		// Initialization should be quick; fallback to direct writer if not.
		w.useTea = false
	}
}

// WriteEvent prints a single event to the output stream.
func (w *Writer) WriteEvent(ev event.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Iteration separator text is internally generated; skip sanitization
	// to preserve the tab-delimited protocol (sanitize replaces \t).
	if ev.Kind != event.KindIterationSeparator {
		ev.Text = sanitizeTerminalText(ev.Text)
	}
	w.ensureTeaLocked()

	if w.teaActive {
		if ev.Kind == event.KindStreamingText {
			w.writeTeaStreamingLocked(ev.Text)
			return
		}

		w.flushTeaPendingLocked()

		line := w.formatEventLine(ev)
		w.tea.Println(line)
		return
	}

	// Fallback mode (non-TTY or Bubble Tea unavailable).
	if !w.isTTY {
		if ev.Kind == event.KindStreamingText {
			fmt.Fprint(w.out, ev.Text)
			w.midLine = !strings.HasSuffix(ev.Text, "\n")
			return
		}
		if w.midLine {
			fmt.Fprintln(w.out)
			w.midLine = false
		}
		fmt.Fprintln(w.out, w.formatEventLine(ev))
		return
	}

	w.legacyEraseFooter()
	if ev.Kind == event.KindStreamingText {
		fmt.Fprint(w.out, ev.Text)
		w.midLine = !strings.HasSuffix(ev.Text, "\n")
		w.legacyRedrawFooter()
		return
	}
	if w.midLine {
		fmt.Fprintln(w.out)
		w.midLine = false
	}
	fmt.Fprintln(w.out, w.formatEventLine(ev))
	w.legacyRedrawFooter()
}

func (w *Writer) formatEventLine(ev event.Event) string {
	switch ev.Kind {
	case event.KindProg:
		return w.formatProg(ev.Text)
	case event.KindToolUse:
		return w.formatTool(ev.Text)
	case event.KindToolResult:
		return w.formatToolResult(ev.Text)
	case event.KindReview:
		return w.formatReview(ev.Text)
	case event.KindDiffAdd:
		return w.formatDiffAdd(ev.Text)
	case event.KindDiffDel:
		return w.formatDiffDel(ev.Text)
	case event.KindDiffCtx:
		return w.formatDiffCtx(ev.Text)
	case event.KindDiffHunk:
		return w.formatDiffHunk(ev.Text)
	case event.KindMarkdown:
		return w.formatMarkdown(ev.Text)
	case event.KindIterationSeparator:
		return w.formatIterSep(ev.Text)
	case event.KindStreamingText:
		return ev.Text
	default:
		return ev.Text
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
	if w.height > 0 {
		maxFooterLines := max(w.height-1, 0)
		if maxFooterLines <= 0 {
			return
		}
		if len(lines) > maxFooterLines {
			lines = lines[:maxFooterLines]
		}
	}

	w.lastFooter = lines
	w.footerLines = len(lines)

	w.ensureTeaLocked()
	if w.teaActive {
		w.tea.Send(bubbleFooterMsg{lines: lines})
		return
	}

	w.legacyEraseFooter()
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

	if w.teaActive {
		w.flushTeaPendingLocked()
		w.tea.Send(bubbleFooterMsg{lines: nil})
		w.tea.Quit()
		done := w.teaDone
		w.teaActive = false
		w.tea = nil
		w.teaDone = nil
		w.footerLines = 0
		w.lastFooter = nil
		w.midLine = false
		w.pendingLine = ""
		if done != nil {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
			}
		}
		return
	}

	w.legacyEraseFooter()
	w.footerLines = 0
	w.lastFooter = nil

	if w.midLine {
		fmt.Fprintln(w.out)
		w.midLine = false
	}
}

func (w *Writer) writeTeaStreamingLocked(text string) {
	if text == "" {
		return
	}

	combined := w.pendingLine + text
	parts := strings.Split(combined, "\n")
	if len(parts) == 1 {
		w.pendingLine = combined
		w.midLine = true
		return
	}

	for _, line := range parts[:len(parts)-1] {
		w.tea.Println(line)
	}

	w.pendingLine = parts[len(parts)-1]
	w.midLine = w.pendingLine != ""
}

func (w *Writer) flushTeaPendingLocked() {
	if !w.teaActive || w.pendingLine == "" {
		w.midLine = false
		return
	}
	w.tea.Println(w.pendingLine)
	w.pendingLine = ""
	w.midLine = false
}

// SetExecutorName sets the executor label used in footer status (e.g. claude, pi).
func (w *Writer) SetExecutorName(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if name == "" {
		name = "claude"
	}
	w.executorName = sanitizeTerminalText(strings.ToLower(name))
}

// SetClaudeConfigDir sets a non-default Claude config directory to display in the footer.
func (w *Writer) SetClaudeConfigDir(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.claudeConfigDir = dir
}

// SetProcessStats updates the PID field used by the footer.
func (w *Writer) SetProcessStats(pid int, memKB int64) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pid = pid
	_ = memKB
}

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

	// Orange separator line.
	sep := strings.Repeat("─", w.width)
	lines = append(lines, w.style(colorOrange, sep))

	stageName := ""
	if item != nil {
		if phase := item.CurrentPhase(); phase != nil {
			stageName = phase.Name
		} else if item.AllPhasesComplete() {
			stageName = "complete"
		}
	}

	// Status line: [claude_dir] | item | iteration | elapsed | pid
	var parts []string
	if w.claudeConfigDir != "" {
		parts = append(parts, w.style(colorDim, "claude_dir=")+w.style(colorDimmer, sanitizeTerminalText(w.claudeConfigDir)))
	}
	if item != nil {
		parts = append(parts, w.styleBold(colorMagenta, sanitizeTerminalText(truncateRunes(item.ID, footerIDPrefixChars))))
		if state != nil {
			parts = append(parts, w.style(colorWhite, fmt.Sprintf("iteration %d of %d", state.Iteration, cfg.MaxIterations)))
		}
	} else if state != nil {
		parts = append(parts, w.style(colorWhite, fmt.Sprintf("iteration %d of %d", state.Iteration, cfg.MaxIterations)))
	}
	if w.pid > 0 {
		name := w.executorName
		if name == "" {
			name = "claude"
		}
		parts = append(parts, w.style(colorDim, fmt.Sprintf("%s pid %d", name, w.pid)))
	}
	if len(parts) > 0 {
		parts = sanitizeSlice(parts)
		statusLine := strings.Join(parts, w.style(colorDim, " | "))
		if state != nil && !state.StartTime.IsZero() {
			statusLine += w.style(colorDim, " | ") + w.style(colorWhite, formatElapsed(time.Since(state.StartTime)))
		}
		lines = append(lines, statusLine)
	}

	// Current work line on its own row.
	if stageName != "" {
		lines = append(lines,
			w.style(colorDim, "Working on: ")+
				w.style(colorDimmer, sanitizeTerminalText(stageName)),
		)
	}

	return lines
}

func sanitizeSlice(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, sanitizeTerminalText(s))
	}
	return out
}

func truncateRunes(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	if maxChars <= 3 {
		return string(runes[:maxChars])
	}
	return string(runes[:maxChars-3]) + "..."
}

// Formatting methods per event kind.

func (w *Writer) formatProg(text string) string {
	prefix := "programmator: "
	isFailure := strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "invocation failed:")
	if w.colorEnabled() {
		if isFailure {
			return fgBold(colorRed, "X "+prefix) + text
		}
		return fgBold(colorOrange, prefix) + text
	}
	if isFailure {
		return "X " + prefix + text
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
	if w.renderer != nil {
		if rendered, err := w.renderer.Render(text); err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}

func (w *Writer) formatIterSep(text string) string {
	if strings.HasPrefix(text, "ITER\t") {
		parts := strings.SplitN(text, "\t", 3)
		if len(parts) == 3 {
			return w.formatIterationHeader(parts[1], parts[2])
		}
	}
	return w.formatStartBanner(text)
}

func (w *Writer) formatIterationHeader(iter, maxIter string) string {
	line := strings.Repeat("─", 36)
	if w.colorEnabled() {
		return dim(line) + "\n  " + dim("Iteration ") + fgBold(colorWhite, iter) + dim("/"+maxIter)
	}
	return "── Iteration " + iter + "/" + maxIter + " ──"
}

func (w *Writer) formatStartBanner(text string) string {
	if !w.colorEnabled() {
		return text
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			// Preserve empty lines.
		case strings.HasPrefix(trimmed, "──"):
			lines[i] = dim(line)
		case trimmed == "[programmator]":
			lines[i] = fgBold(colorOrange, trimmed)
		case strings.HasPrefix(line, "Starting "):
			lines[i] = w.colorizeStartingLine(line)
		case strings.Contains(trimmed, "✓"):
			before, after, _ := strings.Cut(line, "✓")
			lines[i] = dim(before) + fg(colorGreen, "✓") + dim(after)
		case strings.Contains(trimmed, "→"):
			before, after, _ := strings.Cut(line, "→")
			name := strings.TrimSpace(after)
			lines[i] = dim(before) + fgBold(colorOrange, "→") + " " + fgBold(colorWhite, name)
		case strings.Contains(trimmed, "○"):
			lines[i] = dim(line)
		case strings.HasSuffix(trimmed, ":"):
			lines[i] = dim(line)
		default:
			lines[i] = bold(line)
		}
	}
	return strings.Join(lines, "\n")
}

func (w *Writer) colorizeStartingLine(line string) string {
	if !w.colorEnabled() {
		return line
	}

	// Parse "Starting <type> <id>: <title>"
	const prefix = "Starting "
	if !strings.HasPrefix(line, prefix) {
		return bold(line)
	}

	rest := line[len(prefix):]
	srcType, remainder, found := strings.Cut(rest, " ")
	if !found {
		return bold(line)
	}

	id, title, hasTitle := strings.Cut(remainder, ": ")
	if !hasTitle {
		return dim("Starting "+srcType+" ") + fgBold(colorMagenta, remainder)
	}
	return dim("Starting "+srcType+" ") + fgBold(colorMagenta, id) + dim(": ") + fgBold(colorWhite, title)
}

// style wraps text with 256-color foreground in TTY mode, plain otherwise.
func (w *Writer) style(color int, text string) string {
	if w.colorEnabled() {
		return fg(color, text)
	}
	return text
}

// styleBold wraps text with 256-color foreground and bold in TTY mode.
func (w *Writer) styleBold(color int, text string) string {
	if w.colorEnabled() {
		return fgBold(color, text)
	}
	return text
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
