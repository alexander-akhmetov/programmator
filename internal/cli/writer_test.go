package cli

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/alexander-akhmetov/programmator/internal/domain"
	"github.com/alexander-akhmetov/programmator/internal/event"
	"github.com/alexander-akhmetov/programmator/internal/safety"
)

func newTestWriter(buf *bytes.Buffer) *Writer {
	return &Writer{
		out:   buf,
		isTTY: false,
		width: 80,
		mu:    sync.Mutex{},
	}
}

func newTestWriterTTY(buf *bytes.Buffer) *Writer {
	return &Writer{
		out:   buf,
		isTTY: true,
		width: 80,
		mu:    sync.Mutex{},
	}
}

func newTestWriterTTYWithHeight(buf *bytes.Buffer, height int) *Writer {
	return &Writer{
		out:    buf,
		isTTY:  true,
		width:  80,
		height: height,
		mu:     sync.Mutex{},
	}
}

func TestWriteEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    event.Event
		contains []string
	}{
		{
			name:     "prog",
			event:    event.Prog("Starting phase 1"),
			contains: []string{"programmator:", "Starting phase 1"},
		},
		{
			name:     "tool use",
			event:    event.ToolUse("Read /foo/bar.go"),
			contains: []string{"Read /foo/bar.go"},
		},
		{
			name:     "tool result",
			event:    event.ToolResult("  42 lines"),
			contains: []string{"42 lines"},
		},
		{
			name:     "review",
			event:    event.Review("Running agent: quality"),
			contains: []string{"Running agent: quality"},
		},
		{
			name:     "markdown",
			event:    event.Markdown("Some **bold** text"),
			contains: []string{"bold"},
		},
		{
			name:     "iteration separator",
			event:    event.IterationSeparator("--- Iteration 3 ---"),
			contains: []string{"Iteration 3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := newTestWriter(&buf)

			w.WriteEvent(tt.event)

			output := buf.String()
			for _, s := range tt.contains {
				assert.Contains(t, output, s)
			}
		})
	}
}

func TestWriteEvent_DiffLines(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.DiffHunk("@@ -1,3 +1,4 @@"))
	w.WriteEvent(event.DiffAdd("+new line"))
	w.WriteEvent(event.DiffDel("-old line"))
	w.WriteEvent(event.DiffCtx(" context"))

	output := buf.String()
	assert.Contains(t, output, "@@ -1,3 +1,4 @@")
	assert.Contains(t, output, "+new line")
	assert.Contains(t, output, "-old line")
	assert.Contains(t, output, "context")
}

func TestWriteEvent_TTYMode(t *testing.T) {
	tests := []struct {
		name    string
		isTTY   bool
		hasANSI bool
		hasProg bool
	}{
		{"non-TTY has no ANSI", false, false, true},
		{"TTY has ANSI", true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var w *Writer
			if tt.isTTY {
				w = newTestWriterTTY(&buf)
			} else {
				w = newTestWriter(&buf)
			}

			w.WriteEvent(event.Prog("test text"))

			output := buf.String()
			if tt.hasANSI {
				assert.Contains(t, output, "\033[")
			} else {
				assert.NotContains(t, output, "\033[")
			}
			if tt.hasProg {
				assert.Contains(t, output, "programmator:")
				assert.Contains(t, output, "test text")
			}
		})
	}
}

func TestUpdateFooter(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		wantOutput bool
		contains   []string
	}{
		{
			name:       "TTY renders footer",
			isTTY:      true,
			wantOutput: true,
			contains:   []string{"test-123", "3/10", "Phase 2"},
		},
		{
			name:       "non-TTY produces no footer",
			isTTY:      false,
			wantOutput: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var w *Writer
			if tt.isTTY {
				w = newTestWriterTTY(&buf)
			} else {
				w = newTestWriter(&buf)
			}

			state := safety.NewState()
			state.Iteration = 3
			item := &domain.WorkItem{
				ID:    "test-123",
				Title: "Test Ticket",
				Phases: []domain.Phase{
					{Name: "Phase 1", Completed: true},
					{Name: "Phase 2", Completed: false},
				},
			}

			w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})

			output := buf.String()
			if tt.wantOutput {
				for _, s := range tt.contains {
					assert.Contains(t, output, s)
				}
			} else {
				assert.Empty(t, output)
			}
		})
	}
}

func TestUpdateFooter_FrameRenderer(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTYWithHeight(&buf, 40)

	state := safety.NewState()
	state.Iteration = 2
	item := &domain.WorkItem{
		ID:    "test-sr",
		Title: "Scroll Region Test",
		Phases: []domain.Phase{
			{Name: "Phase 1", Completed: false},
		},
	}

	w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})

	output := buf.String()
	// Should contain frame cursor positioning and footer content.
	assert.Contains(t, output, "\033[1;1H")
	assert.Contains(t, output, "test-sr")
	assert.Contains(t, output, "Phase 1")
	assert.Equal(t, len(w.lastFooter), w.footerLines)
}

func TestUpdateFooter_FrameRendererClampsFooterToTerminalHeight(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTYWithHeight(&buf, 2)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{
		ID: "tiny-term",
		Phases: []domain.Phase{
			{Name: "Phase 1"},
		},
	}

	w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})

	assert.Equal(t, 1, w.footerLines)
	assert.Len(t, w.lastFooter, 1)
}

func TestClearFooter(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		wantOutput bool
	}{
		{"TTY clears footer", true, true},
		{"non-TTY is noop", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			var w *Writer
			if tt.isTTY {
				w = newTestWriterTTY(&buf)
				state := safety.NewState()
				state.Iteration = 1
				item := &domain.WorkItem{ID: "t-1"}
				w.UpdateFooter(state, item, safety.Config{MaxIterations: 5})
				buf.Reset()
			} else {
				w = newTestWriter(&buf)
			}

			w.ClearFooter()

			output := buf.String()
			if tt.wantOutput && w.footerLines > 0 {
				assert.Contains(t, output, "\033[")
			} else if !tt.wantOutput {
				assert.Empty(t, output)
			}
		})
	}
}

func TestClearFooter_FrameRenderer(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTYWithHeight(&buf, 40)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "t-sr"}
	w.UpdateFooter(state, item, safety.Config{MaxIterations: 5})

	assert.False(t, w.frameClosed)
	buf.Reset()

	w.ClearFooter()

	output := buf.String()
	assert.Contains(t, output, "\n")
	assert.True(t, w.frameClosed)
	assert.Equal(t, 0, w.footerLines)
}

func TestSetProcessStats(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTY(&buf)

	w.SetProcessStats(12345, 512*1024)

	assert.Equal(t, 12345, w.pid)
	assert.Equal(t, int64(512*1024), w.memKB)
}

func TestWriter_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			w.WriteEvent(event.Prog("concurrent message"))
			_ = n
		}(i)
	}
	wg.Wait()

	output := buf.String()
	count := strings.Count(output, "programmator:")
	assert.Equal(t, 100, count)
}

func TestWriter_ConcurrentWriteAndFooter(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTY(&buf)

	state := safety.NewState()
	item := &domain.WorkItem{ID: "t-race"}
	cfg := safety.Config{MaxIterations: 10}

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			w.WriteEvent(event.Prog("event"))
		}()
		go func() {
			defer wg.Done()
			w.UpdateFooter(state, item, cfg)
		}()
	}
	wg.Wait()

	assert.NotEmpty(t, buf.String())
}

func TestWriteEvent_StreamingText(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.StreamingText("Hello"))
	w.WriteEvent(event.StreamingText(" World"))

	output := buf.String()
	assert.Equal(t, "Hello World", output)
	assert.True(t, w.midLine)
}

func TestWriteEvent_StreamingTextWithNewline(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.StreamingText("Hello\n"))

	assert.Equal(t, "Hello\n", buf.String())
	assert.False(t, w.midLine)
}

func TestWriteEvent_FrameRendererWrapsLongStreamingLines(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTYWithHeight(&buf, 6)
	w.width = 10

	w.WriteEvent(event.StreamingText(strings.Repeat("x", 25)))

	assert.GreaterOrEqual(t, len(w.frameRows), 2)
	assert.Equal(t, 5, utf8.RuneCountInString(w.frameCurrent.text))
}

func TestWriteEvent_StreamingTextConvertsCarriageReturnToNewline(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.StreamingText("line1\rline2"))

	assert.Equal(t, "line1\nline2", buf.String())
	assert.True(t, w.midLine)
}

func TestWriteEvent_StripsANSISequences(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.ToolResult("\x1b[31mred\x1b[0m output"))

	output := buf.String()
	assert.Contains(t, output, "red output")
	assert.NotContains(t, output, "\x1b")
}

func TestWriteEvent_StreamingToStructuredTransition(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.StreamingText("partial"))
	w.WriteEvent(event.Prog("next event"))

	output := buf.String()
	assert.Contains(t, output, "partial\n")
	assert.Contains(t, output, "programmator:")
}

func TestSanitizeTerminalText(t *testing.T) {
	got := sanitizeTerminalText("a\r\nb\rc\x1b[31mred\x1b[0m\x00")
	assert.Equal(t, "a\nb\ncred", got)
}

func TestNewWriter(t *testing.T) {
	tests := []struct {
		name       string
		isTTY      bool
		width      int
		height     int
		wantTTY    bool
		wantWidth  int
		wantHeight int
	}{
		{"non-TTY with zero width defaults to 80", false, 0, 0, false, 80, 0},
		{"TTY with custom dimensions", true, 120, 40, true, 120, 40},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewWriter(&buf, tt.isTTY, tt.width, tt.height)

			assert.Equal(t, tt.wantTTY, w.isTTY)
			assert.Equal(t, tt.wantWidth, w.width)
			assert.Equal(t, tt.wantHeight, w.height)
		})
	}
}

func TestFooterHasOrangeSeparator(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTY(&buf)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "t-1"}

	w.UpdateFooter(state, item, safety.Config{MaxIterations: 5})

	output := buf.String()
	// The separator should use the orange color (208).
	assert.Contains(t, output, "─")
	assert.Contains(t, output, "\033[38;5;208m")
}

func TestWriterFrameRenderer_HighLevelStickyFooter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true, 40, 10)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{
		ID: "ticket-1",
		Phases: []domain.Phase{
			{Name: "Phase 1"},
		},
	}
	cfg := safety.Config{MaxIterations: 10, StagnationLimit: 3}

	w.UpdateFooter(state, item, cfg)
	w.WriteEvent(event.ToolUse("Bash " + strings.Repeat("echo very-long-command ", 4)))
	w.WriteEvent(event.StreamingText("stream " + strings.Repeat("x", 120)))
	w.WriteEvent(event.StreamingText("\n"))
	w.WriteEvent(event.ToolResult("command done"))

	state.Iteration = 2
	w.UpdateFooter(state, item, cfg)

	screen := simulateScreen(buf.String(), 40, 10)
	footer := w.lastFooter
	requireFooterAtBottom(t, screen, footer)

	content := strings.Join(screen[:10-len(footer)], "\n")
	assert.Contains(t, content, "command done")
	assert.NotContains(t, content, "ticket-1")
}

func TestWriterFrameRenderer_HighLevelScrollAndFooter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true, 30, 7)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{
		ID: "scroll-ticket",
		Phases: []domain.Phase{
			{Name: "Phase 1"},
		},
	}
	cfg := safety.Config{MaxIterations: 50, StagnationLimit: 3}

	w.UpdateFooter(state, item, cfg)
	for i := 1; i <= 25; i++ {
		w.WriteEvent(event.ToolUse(fmt.Sprintf("Read file-%02d.go", i)))
	}

	screen := simulateScreen(buf.String(), 30, 7)
	footer := w.lastFooter
	requireFooterAtBottom(t, screen, footer)

	content := strings.Join(screen[:7-len(footer)], "\n")
	assert.Contains(t, content, "file-25.go")
	assert.NotContains(t, content, "file-01.go")
}

func requireFooterAtBottom(t *testing.T, screen, footer []string) {
	t.Helper()
	for i := range footer {
		row := len(screen) - len(footer) + i
		assert.Equal(t, strings.TrimRight(footer[i], " "), strings.TrimRight(screen[row], " "))
	}
}

func simulateScreen(output string, width, height int) []string {
	screen := make([][]rune, height)
	for i := range screen {
		screen[i] = []rune(strings.Repeat(" ", width))
	}

	row, col := 1, 1

	for i := 0; i < len(output); {
		if output[i] != '\x1b' {
			r, size := utf8.DecodeRuneInString(output[i:])
			if r == '\n' {
				row++
				col = 1
				if row > height {
					row = height
				}
			} else {
				if row >= 1 && row <= height && col >= 1 && col <= width {
					screen[row-1][col-1] = r
				}
				col++
				if col > width {
					col = width
				}
			}
			i += size
			continue
		}

		if i+1 >= len(output) || output[i+1] != '[' {
			i++
			continue
		}

		j := i + 2
		for ; j < len(output); j++ {
			if output[j] >= 0x40 && output[j] <= 0x7e {
				break
			}
		}
		if j >= len(output) {
			break
		}

		params := output[i+2 : j]
		cmd := output[j]

		switch cmd {
		case 'H', 'f':
			row, col = parseCursorPosition(params)
			if row < 1 {
				row = 1
			}
			if row > height {
				row = height
			}
			if col < 1 {
				col = 1
			}
			if col > width {
				col = width
			}
		case 'K':
			// 2K = clear entire current line.
			if params == "2" && row >= 1 && row <= height {
				screen[row-1] = []rune(strings.Repeat(" ", width))
				col = 1
			}
		}

		i = j + 1
	}

	lines := make([]string, height)
	for i := range screen {
		lines[i] = strings.TrimRight(string(screen[i]), " ")
	}
	return lines
}

func parseCursorPosition(params string) (int, int) {
	if params == "" {
		return 1, 1
	}

	parts := strings.Split(params, ";")
	row, col := 1, 1

	if len(parts) > 0 && parts[0] != "" {
		if v, err := strconv.Atoi(parts[0]); err == nil {
			row = v
		}
	}
	if len(parts) > 1 && parts[1] != "" {
		if v, err := strconv.Atoi(parts[1]); err == nil {
			col = v
		}
	}

	return row, col
}
