package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/worksonmyai/programmator/internal/event"
	"github.com/worksonmyai/programmator/internal/protocol"
)

// wrapLogs renders the log viewport content from typed events or legacy logs.
func (m Model) wrapLogs() string {
	if len(m.events) > 0 {
		return m.renderEvents()
	}

	content := strings.Join(m.logs, "")
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	var processed []string
	var markdownBuffer []string

	flushMarkdown := func() {
		if len(markdownBuffer) > 0 {
			md := strings.Join(markdownBuffer, "\n")
			if m.renderer != nil {
				if rendered, err := m.renderer.Render(md); err == nil {
					processed = append(processed, strings.TrimSpace(rendered))
				} else {
					processed = append(processed, md)
				}
			} else {
				processed = append(processed, md)
			}
			markdownBuffer = nil
		}
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, protocol.MarkerProg):
			flushMarkdown()
			processed = append(processed, m.renderProgLine(strings.TrimPrefix(line, protocol.MarkerProg)))
		case strings.HasPrefix(line, protocol.MarkerTool):
			flushMarkdown()
			processed = append(processed, m.renderToolLine(strings.TrimPrefix(line, protocol.MarkerTool)))
		case strings.HasPrefix(line, protocol.MarkerToolRes):
			flushMarkdown()
			processed = append(processed, toolResStyle.Render(strings.TrimPrefix(line, protocol.MarkerToolRes)))
		case strings.HasPrefix(line, protocol.MarkerDiffAdd):
			flushMarkdown()
			processed = append(processed, diffAddStyle.Render(strings.TrimPrefix(line, protocol.MarkerDiffAdd)))
		case strings.HasPrefix(line, protocol.MarkerDiffDel):
			flushMarkdown()
			processed = append(processed, diffDelStyle.Render(strings.TrimPrefix(line, protocol.MarkerDiffDel)))
		case strings.HasPrefix(line, protocol.MarkerDiffAt):
			flushMarkdown()
			processed = append(processed, diffHunkStyle.Render(strings.TrimPrefix(line, protocol.MarkerDiffAt)))
		case strings.HasPrefix(line, protocol.MarkerDiffCtx):
			flushMarkdown()
			processed = append(processed, diffCtxStyle.Render(strings.TrimPrefix(line, protocol.MarkerDiffCtx)))
		case strings.HasPrefix(line, protocol.MarkerReview):
			flushMarkdown()
			processed = append(processed, reviewStyle.Render(strings.TrimPrefix(line, protocol.MarkerReview)))
		default:
			markdownBuffer = append(markdownBuffer, line)
		}
	}
	flushMarkdown()

	return strings.Join(processed, "\n")
}

// renderEvents renders typed events without scanning for markers.
func (m Model) renderEvents() string {
	var processed []string
	var markdownBuffer []string

	flushMarkdown := func() {
		if len(markdownBuffer) > 0 {
			md := strings.Join(markdownBuffer, "\n")
			if m.renderer != nil {
				if rendered, err := m.renderer.Render(md); err == nil {
					processed = append(processed, strings.TrimSpace(rendered))
				} else {
					processed = append(processed, md)
				}
			} else {
				processed = append(processed, md)
			}
			markdownBuffer = nil
		}
	}

	for _, ev := range m.events {
		switch ev.Kind {
		case event.KindProg:
			flushMarkdown()
			processed = append(processed, m.renderProgLine(ev.Text))
		case event.KindToolUse:
			flushMarkdown()
			processed = append(processed, m.renderToolLine(ev.Text))
		case event.KindToolResult:
			flushMarkdown()
			processed = append(processed, toolResStyle.Render(ev.Text))
		case event.KindReview:
			flushMarkdown()
			processed = append(processed, reviewStyle.Render(ev.Text))
		case event.KindDiffAdd:
			flushMarkdown()
			processed = append(processed, diffAddStyle.Render(ev.Text))
		case event.KindDiffDel:
			flushMarkdown()
			processed = append(processed, diffDelStyle.Render(ev.Text))
		case event.KindDiffCtx:
			flushMarkdown()
			processed = append(processed, diffCtxStyle.Render(ev.Text))
		case event.KindDiffHunk:
			flushMarkdown()
			processed = append(processed, diffHunkStyle.Render(ev.Text))
		case event.KindIterationSeparator:
			flushMarkdown()
			markdownBuffer = append(markdownBuffer, ev.Text)
			flushMarkdown()
		case event.KindMarkdown:
			markdownBuffer = append(markdownBuffer, ev.Text)
		}
	}
	flushMarkdown()

	return strings.Join(processed, "\n")
}

// renderProgLine styles a progress message with the programmator prefix.
func (m Model) renderProgLine(msg string) string {
	prefix := progPrefixStyle.Render("▶ programmator:") + " "
	prefixLen := lipgloss.Width(prefix)
	availWidth := m.logViewport.Width - prefixLen
	if availWidth > 20 {
		indent := strings.Repeat(" ", prefixLen)
		wrapped := wrapText(msg, m.logViewport.Width, indent, 0)
		parts := strings.SplitN(wrapped, "\n", 2)
		if len(parts) == 2 {
			return prefix + parts[0] + "\n" + parts[1]
		}
		return prefix + wrapped
	}
	return prefix + msg
}

// renderToolLine styles a tool use message with the "> " prefix.
func (m Model) renderToolLine(msg string) string {
	toolPrefix := toolStyle.Render("> ")
	toolPrefixLen := lipgloss.Width(toolPrefix)
	availWidth := m.logViewport.Width - toolPrefixLen
	if availWidth > 20 {
		indent := strings.Repeat(" ", toolPrefixLen)
		wrapped := wrapText(msg, m.logViewport.Width, indent, 0)
		parts := strings.SplitN(wrapped, "\n", 2)
		if len(parts) == 2 {
			return toolPrefix + parts[0] + "\n" + parts[1]
		}
		return toolPrefix + wrapped
	}
	return toolStyle.Render("> " + msg)
}
