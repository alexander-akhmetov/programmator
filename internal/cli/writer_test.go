package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"

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

func TestUpdateFooter_ScrollRegion(t *testing.T) {
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
	// Should contain DECSTBM (scroll region set).
	assert.Contains(t, output, "\033[1;")
	assert.Contains(t, output, "r")
	// Should contain footer content.
	assert.Contains(t, output, "test-sr")
	assert.Contains(t, output, "Phase 1")
	assert.True(t, w.scrollSet)
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

func TestClearFooter_ScrollRegion(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriterTTYWithHeight(&buf, 40)

	state := safety.NewState()
	state.Iteration = 1
	item := &domain.WorkItem{ID: "t-sr"}
	w.UpdateFooter(state, item, safety.Config{MaxIterations: 5})

	assert.True(t, w.scrollSet)
	buf.Reset()

	w.ClearFooter()

	output := buf.String()
	// Should reset scroll region.
	assert.Contains(t, output, "\033[r")
	assert.False(t, w.scrollSet)
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

func TestWriteEvent_StreamingToStructuredTransition(t *testing.T) {
	var buf bytes.Buffer
	w := newTestWriter(&buf)

	w.WriteEvent(event.StreamingText("partial"))
	w.WriteEvent(event.Prog("next event"))

	output := buf.String()
	assert.Contains(t, output, "partial\n")
	assert.Contains(t, output, "programmator:")
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
