package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventConstructors(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) Event
		kind Kind
	}{
		{"Prog", Prog, KindProg},
		{"ToolUse", ToolUse, KindToolUse},
		{"ToolResult", ToolResult, KindToolResult},
		{"Review", Review, KindReview},
		{"DiffAdd", DiffAdd, KindDiffAdd},
		{"DiffDel", DiffDel, KindDiffDel},
		{"DiffCtx", DiffCtx, KindDiffCtx},
		{"DiffHunk", DiffHunk, KindDiffHunk},
		{"Markdown", Markdown, KindMarkdown},
		{"IterationSeparator", IterationSeparator, KindIterationSeparator},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := tc.fn("hello")
			assert.Equal(t, tc.kind, e.Kind)
			assert.Equal(t, "hello", e.Text)
		})
	}
}

func TestKindValues(t *testing.T) {
	// Verify kinds are distinct.
	kinds := []Kind{
		KindProg, KindToolUse, KindToolResult, KindReview,
		KindDiffAdd, KindDiffDel, KindDiffCtx, KindDiffHunk,
		KindMarkdown, KindIterationSeparator,
	}
	seen := make(map[Kind]bool)
	for _, k := range kinds {
		assert.False(t, seen[k], "duplicate Kind value %d", k)
		seen[k] = true
	}
}

func TestHandler(t *testing.T) {
	var received []Event
	h := Handler(func(e Event) {
		received = append(received, e)
	})

	h(Prog("one"))
	h(Review("two"))

	assert.Len(t, received, 2)
	assert.Equal(t, KindProg, received[0].Kind)
	assert.Equal(t, "one", received[0].Text)
	assert.Equal(t, KindReview, received[1].Kind)
	assert.Equal(t, "two", received[1].Text)
}

func TestEventEmptyText(t *testing.T) {
	e := Prog("")
	assert.Equal(t, KindProg, e.Kind)
	assert.Equal(t, "", e.Text)
}

func TestEventConstructorsPreserveWhitespace(t *testing.T) {
	text := "  leading and trailing  "
	e := ToolUse(text)
	assert.Equal(t, text, e.Text)
}
