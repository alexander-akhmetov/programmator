package source

import (
	"github.com/worksonmyai/programmator/internal/domain"
	"github.com/worksonmyai/programmator/internal/plan"
	"github.com/worksonmyai/programmator/internal/protocol"
)

// PlanSource adapts plan files to the Source interface.
// It also implements Mover for plan-file relocation.
type PlanSource struct {
	filePath string
}

// Compile-time interface checks.
var (
	_ Source = (*PlanSource)(nil)
	_ Mover  = (*PlanSource)(nil)
)

// NewPlanSource creates a new PlanSource for the given file path.
func NewPlanSource(filePath string) *PlanSource {
	return &PlanSource{filePath: filePath}
}

// Get parses the plan file and returns it as a WorkItem.
// The id parameter is expected to be the file path.
func (s *PlanSource) Get(_ string) (*domain.WorkItem, error) {
	p, err := plan.ParseFile(s.filePath)
	if err != nil {
		return nil, err
	}
	return planToWorkItem(p), nil
}

// UpdatePhase marks a task as completed in the plan file.
func (s *PlanSource) UpdatePhase(_ string, phaseName string) error {
	p, err := plan.ParseFile(s.filePath)
	if err != nil {
		return err
	}

	if err := p.MarkTaskComplete(phaseName); err != nil {
		return err
	}

	return p.SaveFile()
}

// AddNote is a no-op for plan files.
// Plan files don't have a notes section like tickets.
func (s *PlanSource) AddNote(_, _ string) error {
	return nil
}

// SetStatus is a no-op for plan files.
// Plan files don't track status separately.
func (s *PlanSource) SetStatus(_, _ string) error {
	return nil
}

// Type returns "plan".
func (s *PlanSource) Type() string {
	return TypePlan
}

// FilePath returns the plan file path.
func (s *PlanSource) FilePath() string {
	return s.filePath
}

// MoveTo moves the plan file to a new directory.
// Returns the new file path.
func (s *PlanSource) MoveTo(destDir string) (string, error) {
	p, err := plan.ParseFile(s.filePath)
	if err != nil {
		return "", err
	}

	newPath, err := p.MoveTo(destDir)
	if err != nil {
		return "", err
	}

	// Update our file path reference
	s.filePath = newPath
	return newPath, nil
}

// planToWorkItem converts a plan.Plan to a domain.WorkItem.
func planToWorkItem(p *plan.Plan) *domain.WorkItem {
	phases := make([]domain.Phase, len(p.Tasks))
	for i, t := range p.Tasks {
		phases[i] = domain.Phase{
			Name:      t.Name,
			Completed: t.Completed,
		}
	}

	return &domain.WorkItem{
		ID:                 p.ID(),
		Title:              p.Title,
		Status:             protocol.WorkItemOpen, // Plans don't track status, default to open
		Phases:             phases,
		RawContent:         p.RawContent,
		ValidationCommands: p.ValidationCommands,
	}
}
