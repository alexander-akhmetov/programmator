// Package event defines typed events emitted by the loop and review runner,
// consumed by the TUI, progress logger, and CLI output. These replace the
// string-based [PROG]/[TOOL]/[REVIEW] log markers with structured types.
package event

// Kind identifies the type of event.
type Kind int

const (
	// KindProg is a progress message from the orchestration loop.
	KindProg Kind = iota
	// KindToolUse is a tool invocation (name + arguments).
	KindToolUse
	// KindToolResult is a summary of a tool result.
	KindToolResult
	// KindReview is a progress message from the review runner.
	KindReview
	// KindDiffAdd is an added line in a diff.
	KindDiffAdd
	// KindDiffDel is a deleted line in a diff.
	KindDiffDel
	// KindDiffCtx is a context line in a diff.
	KindDiffCtx
	// KindDiffHunk is a hunk header / summary line in a diff.
	KindDiffHunk
	// KindMarkdown is a raw markdown text fragment (rendered by TUI via glamour).
	KindMarkdown
	// KindIterationSeparator is the header between loop iterations.
	KindIterationSeparator
)

// Event is a single typed event emitted by the loop or review runner.
type Event struct {
	Kind Kind
	Text string // the payload text (meaning depends on Kind)
}

// Handler is a callback that receives typed events.
type Handler func(Event)

// Prog creates a KindProg event.
func Prog(text string) Event { return Event{Kind: KindProg, Text: text} }

// ToolUse creates a KindToolUse event.
func ToolUse(text string) Event { return Event{Kind: KindToolUse, Text: text} }

// ToolResult creates a KindToolResult event.
func ToolResult(text string) Event { return Event{Kind: KindToolResult, Text: text} }

// Review creates a KindReview event.
func Review(text string) Event { return Event{Kind: KindReview, Text: text} }

// DiffAdd creates a KindDiffAdd event.
func DiffAdd(text string) Event { return Event{Kind: KindDiffAdd, Text: text} }

// DiffDel creates a KindDiffDel event.
func DiffDel(text string) Event { return Event{Kind: KindDiffDel, Text: text} }

// DiffCtx creates a KindDiffCtx event.
func DiffCtx(text string) Event { return Event{Kind: KindDiffCtx, Text: text} }

// DiffHunk creates a KindDiffHunk event.
func DiffHunk(text string) Event { return Event{Kind: KindDiffHunk, Text: text} }

// Markdown creates a KindMarkdown event.
func Markdown(text string) Event { return Event{Kind: KindMarkdown, Text: text} }

// IterationSeparator creates a KindIterationSeparator event.
func IterationSeparator(text string) Event { return Event{Kind: KindIterationSeparator, Text: text} }
