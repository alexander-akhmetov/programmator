package cli

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			event:    event.IterationSeparator("ITER\t3\t10"),
			contains: []string{"Iteration", "3", "/10"},
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

func TestFormatProg_FailurePrefix(t *testing.T) {
	var buf bytes.Buffer
	wTTY := newTestWriterTTY(&buf)
	wNoTTY := newTestWriter(&buf)

	ttyLine := wTTY.formatProg("Invocation failed: claude exited: signal: interrupt")
	assert.Contains(t, ttyLine, "X programmator:")
	assert.Contains(t, ttyLine, fmt.Sprintf("\033[1;38;5;%dm", colorRed))

	plainLine := wNoTTY.formatProg("Invocation failed: claude exited: signal: interrupt")
	assert.Contains(t, plainLine, "X programmator:")
	assert.NotContains(t, plainLine, "\033[")
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
			contains:   []string{"test-123", "iteration 3 of 10", "Working on: Phase 2"},
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

			output := stripANSISequences(buf.String())
			if tt.wantOutput {
				for _, s := range tt.contains {
					assert.Contains(t, output, s)
				}
				assert.NotContains(t, output, "stag ")
				assert.NotContains(t, output, "files ")
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
	// Should draw footer content.
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
			if tt.wantOutput {
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

	buf.Reset()

	w.ClearFooter()

	output := buf.String()
	assert.Contains(t, output, "\033[")
	assert.Equal(t, 0, w.footerLines)
	assert.Nil(t, w.lastFooter)
}

func TestSetProcessStats(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTY(&buf)

	w.SetExecutorName("claude")
	w.SetProcessStats(12345, 512*1024)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "pid-test"}
	w.UpdateFooter(state, item, safety.Config{MaxIterations: 5})

	output := stripANSISequences(buf.String())
	assert.Equal(t, 12345, w.pid)
	assert.Contains(t, output, "claude pid 12345")
	assert.NotContains(t, output, "MB")
}

func TestUpdateFooter_PhaseOnSecondLineAndPIDOnFirst(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTY(&buf)

	w.SetExecutorName("pi")
	w.SetProcessStats(9876, 0)

	state := safety.NewState()
	state.Iteration = 2
	item := &domain.WorkItem{
		ID: "phase-order",
		Phases: []domain.Phase{
			{Name: "Implement parser", Completed: false},
		},
	}

	w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})

	require.GreaterOrEqual(t, len(w.lastFooter), 3)
	firstStatusLine := stripANSISequences(w.lastFooter[1])
	secondPhaseLine := stripANSISequences(w.lastFooter[2])

	assert.Contains(t, firstStatusLine, "pi pid 9876")
	assert.NotContains(t, secondPhaseLine, "pid")
	assert.Contains(t, secondPhaseLine, "Working on: Implement parser")
	assert.Contains(t, w.lastFooter[2], fmt.Sprintf("\033[38;5;%dm", colorDimmer))
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
	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "t-1", Phases: []domain.Phase{{Name: "Phase 1"}}}
	w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})
	buf.Reset()

	w.WriteEvent(event.StreamingText(strings.Repeat("x", 25)))

	output := buf.String()
	assert.Contains(t, output, strings.Repeat("x", 25))
	assert.True(t, w.midLine)
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
	// The separator should use the configured orange palette color.
	assert.Contains(t, output, "─")
	assert.Contains(t, output, fmt.Sprintf("\033[38;5;%dm", colorOrange))
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
	footerSnapshot := strings.Join(stripANSISlice(w.lastFooter), "\n")
	w.ClearFooter()

	output := stripANSISequences(buf.String())
	assert.Contains(t, output, "command done")
	assert.Contains(t, footerSnapshot, "ticket-1")
	assert.Contains(t, footerSnapshot, "iteration 2 of 10")
	assert.Contains(t, footerSnapshot, "Phase 1")
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
	footerSnapshot := strings.Join(stripANSISlice(w.lastFooter), "\n")
	w.ClearFooter()

	output := stripANSISequences(buf.String())
	assert.Contains(t, output, "file-25.go")
	assert.Contains(t, output, "file-01.go")
	assert.Contains(t, footerSnapshot, truncateRunes("scroll-ticket", footerIDPrefixChars))
}

func TestWriterTeaMode_ChunkedStreamingRemainsInline(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true, 40, 8)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{
		ID: "chunk-ticket",
		Phases: []domain.Phase{
			{Name: "Phase 1"},
		},
	}
	cfg := safety.Config{MaxIterations: 10, StagnationLimit: 3}

	w.UpdateFooter(state, item, cfg)
	w.WriteEvent(event.StreamingText("A"))
	w.WriteEvent(event.StreamingText("N"))
	w.WriteEvent(event.StreamingText("N"))
	w.WriteEvent(event.StreamingText("O"))
	w.WriteEvent(event.StreamingText("U"))
	w.WriteEvent(event.StreamingText("N"))
	w.WriteEvent(event.StreamingText("C"))
	w.WriteEvent(event.StreamingText("E"))
	w.WriteEvent(event.ToolResult("done"))
	w.ClearFooter()

	output := stripANSISequences(buf.String())
	assert.Contains(t, output, "ANNOUNCE")
	assert.NotContains(t, output, "A\nN\n")
	assert.Contains(t, output, "done")
}

func TestWriterTeaMode_ClearFooterFlushesPendingStreaming(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, true, 40, 8)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "flush-ticket"}
	w.UpdateFooter(state, item, safety.Config{MaxIterations: 10, StagnationLimit: 3})
	w.WriteEvent(event.StreamingText("partial-line"))
	w.ClearFooter()

	output := stripANSISequences(buf.String())
	assert.Contains(t, output, "partial-line")
}

func TestFormatIterationHeader(t *testing.T) {
	tests := []struct {
		name     string
		isTTY    bool
		iter     string
		maxIter  string
		contains []string
		excludes []string
	}{
		{
			name:    "TTY renders colored iteration header",
			isTTY:   true,
			iter:    "3",
			maxIter: "10",
			contains: []string{
				"─",         // horizontal line
				"Iteration", // label
				"3",         // iteration number
				"/10",       // max iterations
				"\033[",     // ANSI escape present
			},
		},
		{
			name:    "TTY uses dim for horizontal line",
			isTTY:   true,
			iter:    "1",
			maxIter: "50",
			contains: []string{
				"\033[2m", // dim escape for the line
			},
		},
		{
			name:    "TTY uses bold white for iteration number",
			isTTY:   true,
			iter:    "7",
			maxIter: "20",
			contains: []string{
				fmt.Sprintf("\033[1;38;5;%dm7\033[0m", colorWhite), // bold white "7"
			},
		},
		{
			name:    "non-TTY renders plain text header",
			isTTY:   false,
			iter:    "2",
			maxIter: "50",
			contains: []string{
				"──",             // plain horizontal markers
				"Iteration 2/50", // plain text iteration info
			},
			excludes: []string{
				"\033[", // no ANSI escapes
			},
		},
		{
			name:    "non-TTY with single-digit iterations",
			isTTY:   false,
			iter:    "1",
			maxIter: "5",
			contains: []string{
				"Iteration 1/5",
			},
			excludes: []string{
				"\033[",
			},
		},
		{
			name:    "TTY horizontal line is 36 chars",
			isTTY:   true,
			iter:    "1",
			maxIter: "10",
			contains: []string{
				strings.Repeat("─", 36),
			},
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

			result := w.formatIterationHeader(tt.iter, tt.maxIter)

			for _, s := range tt.contains {
				assert.Contains(t, result, s, "expected %q in output", s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, result, s, "unexpected %q in output", s)
			}
		})
	}
}

func TestFormatIterSep_DispatchesIterPrefix(t *testing.T) {
	tests := []struct {
		name     string
		isTTY    bool
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "ITER prefix dispatches to formatIterationHeader (non-TTY)",
			isTTY:    false,
			input:    "ITER\t3\t10",
			contains: []string{"Iteration 3/10"},
		},
		{
			name:     "ITER prefix dispatches to formatIterationHeader (TTY)",
			isTTY:    true,
			input:    "ITER\t1\t50",
			contains: []string{"Iteration", "1", "/50", "\033["},
		},
		{
			name:     "non-ITER text dispatches to formatStartBanner (non-TTY)",
			isTTY:    false,
			input:    "──────\n[programmator]\nStarting plan i-123: Title\n──────",
			contains: []string{"[programmator]", "Starting plan i-123: Title"},
		},
		{
			name:     "non-ITER text dispatches to formatStartBanner (TTY)",
			isTTY:    true,
			input:    "──────\n[programmator]\nStarting plan i-123: Title\n──────",
			contains: []string{"[programmator]", "i-123"},
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

			result := w.formatIterSep(tt.input)

			for _, s := range tt.contains {
				assert.Contains(t, result, s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, result, s)
			}
		})
	}
}

func TestFormatStartBanner(t *testing.T) {
	// Build a realistic banner matching logStartBanner output.
	banner := strings.Join([]string{
		"──────────────────────────────────────────",
		"[programmator]",
		"",
		"Starting plan i-123: Fix the bug",
		" Tasks (3):",
		"   ✓ Phase 1",
		"   → Phase 2",
		"   ○ Phase 3",
		"──────────────────────────────────────────",
	}, "\n")

	t.Run("non-TTY returns text unchanged", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriter(&buf)

		result := w.formatStartBanner(banner)

		assert.Equal(t, banner, result)
		assert.NotContains(t, result, "\033[")
	})

	t.Run("TTY colorizes separator lines as dim", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		// First and last lines are separators — should be dim.
		assert.Contains(t, lines[0], "\033[2m", "first separator should be dim")
		assert.Contains(t, lines[len(lines)-1], "\033[2m", "last separator should be dim")
	})

	t.Run("TTY colorizes [programmator] as orange bold", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		// [programmator] line should use orange bold (color 208).
		assert.Contains(t, lines[1], fmt.Sprintf("\033[1;38;5;%dm", colorOrange))
		assert.Contains(t, lines[1], "[programmator]")
	})

	t.Run("TTY colorizes Starting line with dim type, magenta ID, bold white title", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		startLine := lines[3] // "Starting plan i-123: Fix the bug"
		// ID in magenta bold.
		assert.Contains(t, startLine, fmt.Sprintf("\033[1;38;5;%dm", colorMagenta))
		assert.Contains(t, startLine, "i-123")
		// Title in bold white.
		assert.Contains(t, startLine, fmt.Sprintf("\033[1;38;5;%dm", colorWhite))
		assert.Contains(t, startLine, "Fix the bug")
	})

	t.Run("TTY colorizes done phase with green checkmark and dim name", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		doneLine := lines[5] // "   ✓ Phase 1"
		// Green checkmark.
		assert.Contains(t, doneLine, fmt.Sprintf("\033[38;5;%dm", colorGreen))
		assert.Contains(t, doneLine, "✓")
		// Phase name in dim.
		assert.Contains(t, doneLine, "\033[2m")
		assert.Contains(t, doneLine, "Phase 1")
	})

	t.Run("TTY colorizes current phase with orange bold arrow and bold white name", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		currentLine := lines[6] // "   → Phase 2"
		// Orange bold arrow.
		assert.Contains(t, currentLine, fmt.Sprintf("\033[1;38;5;%dm", colorOrange))
		assert.Contains(t, currentLine, "→")
		// Phase name in bold white.
		assert.Contains(t, currentLine, fmt.Sprintf("\033[1;38;5;%dm", colorWhite))
		assert.Contains(t, currentLine, "Phase 2")
	})

	t.Run("TTY colorizes pending phase as all dim", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		pendingLine := lines[7] // "   ○ Phase 3"
		assert.Contains(t, pendingLine, "\033[2m")
		assert.Contains(t, pendingLine, "○")
		assert.Contains(t, pendingLine, "Phase 3")
		// Should NOT contain bold or bright colors.
		assert.NotContains(t, pendingLine, fmt.Sprintf("\033[1;38;5;%dm", colorWhite))
	})

	t.Run("TTY colorizes Phases/Tasks label as dim", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		labelLine := lines[4] // " Tasks (3):"
		assert.Contains(t, labelLine, "\033[2m")
		assert.Contains(t, labelLine, "Tasks (3):")
	})

	t.Run("TTY preserves empty lines", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(banner)
		lines := strings.Split(result, "\n")

		// Line index 2 should remain empty.
		assert.Equal(t, "", lines[2])
	})

	t.Run("TTY with ticket source type", func(t *testing.T) {
		ticketBanner := strings.Join([]string{
			"──────────────────────────────────────────",
			"[programmator]",
			"",
			"Starting ticket pro-abc: Some Title",
			" Phases (1):",
			"   → Phase 1",
			"──────────────────────────────────────────",
		}, "\n")

		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		result := w.formatStartBanner(ticketBanner)

		assert.Contains(t, result, "pro-abc")
		assert.Contains(t, result, "Some Title")
		assert.Contains(t, result, fmt.Sprintf("\033[1;38;5;%dm", colorMagenta))
	})
}

func TestColorizeStartingLine(t *testing.T) {
	tests := []struct {
		name     string
		isTTY    bool
		line     string
		contains []string
		excludes []string
	}{
		{
			name:  "TTY applies dim to source type",
			isTTY: true,
			line:  "Starting plan i-123: Fix the bug",
			contains: []string{
				"\033[2m", // dim present (for "Starting plan")
				"plan",
			},
		},
		{
			name:  "TTY applies magenta bold to ID",
			isTTY: true,
			line:  "Starting plan i-123: Fix the bug",
			contains: []string{
				fmt.Sprintf("\033[1;38;5;%dmi-123\033[0m", colorMagenta),
			},
		},
		{
			name:  "TTY applies bold white to title",
			isTTY: true,
			line:  "Starting plan i-123: Fix the bug",
			contains: []string{
				fmt.Sprintf("\033[1;38;5;%dmFix the bug\033[0m", colorWhite),
			},
		},
		{
			name:  "TTY with ticket type",
			isTTY: true,
			line:  "Starting ticket pro-abc: Some Title",
			contains: []string{
				fmt.Sprintf("\033[1;38;5;%dmpro-abc\033[0m", colorMagenta),
				fmt.Sprintf("\033[1;38;5;%dmSome Title\033[0m", colorWhite),
			},
		},
		{
			name:  "non-TTY returns line unchanged",
			isTTY: false,
			line:  "Starting plan i-123: Fix the bug",
			contains: []string{
				"Starting plan i-123: Fix the bug",
			},
			excludes: []string{
				"\033[",
			},
		},
		{
			name:  "TTY with no colon falls back gracefully",
			isTTY: true,
			line:  "Starting plan i-123",
			contains: []string{
				"Starting",
				"i-123",
			},
		},
		{
			name:  "TTY with title containing colons",
			isTTY: true,
			line:  "Starting plan i-123: Fix: the colon bug",
			contains: []string{
				fmt.Sprintf("\033[1;38;5;%dmi-123\033[0m", colorMagenta),
				"Fix: the colon bug",
			},
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

			result := w.colorizeStartingLine(tt.line)

			for _, s := range tt.contains {
				assert.Contains(t, result, s, "expected %q in output", s)
			}
			for _, s := range tt.excludes {
				assert.NotContains(t, result, s, "unexpected %q in output", s)
			}
		})
	}
}

func TestUpdateFooter_ElapsedTimer(t *testing.T) {
	t.Run("TTY footer contains elapsed timer text", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		state := safety.NewState()
		state.StartTime = time.Now().Add(-76 * time.Second) // 1m 16s ago
		state.Iteration = 2
		item := &domain.WorkItem{ID: "timer-test"}
		cfg := safety.Config{MaxIterations: 10, StagnationLimit: 3}

		w.UpdateFooter(state, item, cfg)

		output := buf.String()
		stripped := stripANSISequences(output)
		assert.Regexp(t, `1m 1[67]s`, stripped, "expected elapsed ~1m16s in footer")
	})

	t.Run("TTY footer uses white color for elapsed", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriterTTY(&buf)

		state := safety.NewState()
		state.StartTime = time.Now().Add(-45 * time.Second)
		state.Iteration = 1
		item := &domain.WorkItem{ID: "white-test"}
		cfg := safety.Config{MaxIterations: 10, StagnationLimit: 3}

		w.UpdateFooter(state, item, cfg)

		output := buf.String()
		// Elapsed should no longer use lime (154).
		assert.NotContains(t, output, "\033[38;5;154m")
		// White color 255 should be present in the footer.
		assert.Contains(t, output, "\033[38;5;255m")
	})

	t.Run("non-TTY footer omits elapsed", func(t *testing.T) {
		var buf bytes.Buffer
		w := newTestWriter(&buf)

		state := safety.NewState()
		state.StartTime = time.Now().Add(-30 * time.Second)
		state.Iteration = 1
		item := &domain.WorkItem{ID: "no-tty"}
		cfg := safety.Config{MaxIterations: 10, StagnationLimit: 3}

		w.UpdateFooter(state, item, cfg)

		// Non-TTY produces no footer at all
		assert.Empty(t, buf.String())
	})
}

func stripANSISlice(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, stripANSISequences(line))
	}
	return out
}
