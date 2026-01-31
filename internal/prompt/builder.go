// Package prompt builds prompts for Claude Code invocations.
package prompt

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/worksonmyai/programmator/internal/config"
	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/protocol"
)

// Builder creates prompts using customizable templates.
type Builder struct {
	phasedTmpl       *template.Template
	phaselessTmpl    *template.Template
	reviewFirstTmpl  *template.Template
	reviewSecondTmpl *template.Template
	planCreateTmpl   *template.Template
}

// NewBuilder creates a prompt builder from loaded prompts.
// If prompts is nil, embedded defaults are used.
func NewBuilder(prompts *config.Prompts) (*Builder, error) {
	if prompts == nil {
		// Load embedded defaults
		var err error
		prompts, err = config.LoadPrompts("", "")
		if err != nil {
			return nil, fmt.Errorf("load embedded prompts: %w", err)
		}
	}

	phasedTmpl, err := template.New("phased").Parse(prompts.Phased)
	if err != nil {
		return nil, fmt.Errorf("parse phased template: %w", err)
	}

	phaselessTmpl, err := template.New("phaseless").Parse(prompts.Phaseless)
	if err != nil {
		return nil, fmt.Errorf("parse phaseless template: %w", err)
	}

	reviewFirstTmpl, err := template.New("review_first").Parse(prompts.ReviewFirst)
	if err != nil {
		return nil, fmt.Errorf("parse review_first template: %w", err)
	}

	reviewSecondTmpl, err := template.New("review_second").Parse(prompts.ReviewSecond)
	if err != nil {
		return nil, fmt.Errorf("parse review_second template: %w", err)
	}

	planCreateTmpl, err := template.New("plan_create").Parse(prompts.PlanCreate)
	if err != nil {
		return nil, fmt.Errorf("parse plan_create template: %w", err)
	}

	return &Builder{
		phasedTmpl:       phasedTmpl,
		phaselessTmpl:    phaselessTmpl,
		reviewFirstTmpl:  reviewFirstTmpl,
		reviewSecondTmpl: reviewSecondTmpl,
		planCreateTmpl:   planCreateTmpl,
	}, nil
}

// Data contains the data for rendering prompt templates.
type Data struct {
	ID               string
	Title            string
	RawContent       string
	Notes            string
	CurrentPhase     string // Formatted phase name (e.g., "**Phase 1**" or "All phases complete")
	CurrentPhaseName string // Raw phase name for status block (e.g., "Phase 1" or "null")
}

// ReviewFixData contains the data for rendering review fix prompts.
type ReviewFixData struct {
	BaseBranch     string
	Iteration      int
	FilesList      string
	IssuesMarkdown string
	AutoCommit     bool
}

// PlanCreateData contains the data for rendering plan creation prompts.
type PlanCreateData struct {
	Description     string // User's description of what they want to accomplish
	PreviousAnswers string // Formatted list of previous Q&A exchanges
}

// Build creates a prompt from a work item and optional progress notes.
func (b *Builder) Build(w *domain.WorkItem, notes []string) (string, error) {
	data := Data{
		ID:         w.ID,
		Title:      w.Title,
		RawContent: w.RawContent,
		Notes:      formatNotes(notes),
	}

	// Use phaseless template when there are no phases
	if !w.HasPhases() {
		return b.render(b.phaselessTmpl, data)
	}

	// Use phased template when phases exist
	currentPhase := w.CurrentPhase()
	if currentPhase != nil {
		data.CurrentPhase = currentPhase.Name
		data.CurrentPhaseName = currentPhase.Name
	} else {
		data.CurrentPhase = "All phases complete"
		data.CurrentPhaseName = protocol.NullPhase
	}

	return b.render(b.phasedTmpl, data)
}

// BuildReviewFirst creates a prompt for comprehensive review phase.
func (b *Builder) BuildReviewFirst(baseBranch string, filesChanged []string, issuesMarkdown string, iteration int, autoCommit bool) (string, error) {
	data := ReviewFixData{
		BaseBranch:     baseBranch,
		Iteration:      iteration,
		FilesList:      formatFilesList(filesChanged),
		IssuesMarkdown: issuesMarkdown,
		AutoCommit:     autoCommit,
	}
	return b.render(b.reviewFirstTmpl, data)
}

// BuildReviewSecond creates a prompt for critical/major issues review phase (multi-phase system).
func (b *Builder) BuildReviewSecond(baseBranch string, filesChanged []string, issuesMarkdown string, iteration int, autoCommit bool) (string, error) {
	data := ReviewFixData{
		BaseBranch:     baseBranch,
		Iteration:      iteration,
		FilesList:      formatFilesList(filesChanged),
		IssuesMarkdown: issuesMarkdown,
		AutoCommit:     autoCommit,
	}
	return b.render(b.reviewSecondTmpl, data)
}

// BuildPlanCreate creates a prompt for interactive plan creation.
func (b *Builder) BuildPlanCreate(description string, previousAnswers []QA) (string, error) {
	data := PlanCreateData{
		Description:     description,
		PreviousAnswers: formatQA(previousAnswers),
	}
	return b.render(b.planCreateTmpl, data)
}

// QA represents a question-answer pair from previous interactions.
type QA struct {
	Question string
	Answer   string
}

func formatQA(qa []QA) string {
	if len(qa) == 0 {
		return ""
	}
	var lines []string
	for _, pair := range qa {
		lines = append(lines, fmt.Sprintf("- **Q:** %s\n  **A:** %s", pair.Question, pair.Answer))
	}
	return strings.Join(lines, "\n")
}

func (b *Builder) render(tmpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatNotes(notes []string) string {
	if len(notes) == 0 {
		return "(No previous notes)"
	}
	noteLines := make([]string, 0, len(notes))
	for _, note := range notes {
		noteLines = append(noteLines, fmt.Sprintf("- %s", note))
	}
	return strings.Join(noteLines, "\n")
}

func formatFilesList(files []string) string {
	if len(files) == 0 {
		return "(no files)"
	}
	return "  - " + strings.Join(files, "\n  - ")
}

// BuildPhaseList creates a formatted list of phases with checkboxes.
func BuildPhaseList(phases []domain.Phase) string {
	lines := make([]string, 0, len(phases))
	for _, p := range phases {
		checkbox := "[ ]"
		if p.Completed {
			checkbox = "[x]"
		}
		lines = append(lines, fmt.Sprintf("- %s %s", checkbox, p.Name))
	}
	return strings.Join(lines, "\n")
}

// defaultBuilder is a package-level builder using embedded defaults.
// It is lazily initialized on first use via defaultBuilderOnce.
var (
	defaultBuilder     *Builder
	defaultBuilderOnce sync.Once
)

// Build creates a prompt using the default builder (embedded templates).
// This is a convenience function for backward compatibility.
func Build(w *domain.WorkItem, notes []string) string {
	defaultBuilderOnce.Do(func() {
		var err error
		defaultBuilder, err = NewBuilder(nil)
		if err != nil {
			defaultBuilder = nil
		}
	})
	if defaultBuilder == nil {
		return fmt.Sprintf("Work item %s: %s\n\n%s", w.ID, w.Title, w.RawContent)
	}
	result, err := defaultBuilder.Build(w, notes)
	if err != nil {
		return fmt.Sprintf("Work item %s: %s\n\n%s", w.ID, w.Title, w.RawContent)
	}
	return result
}
