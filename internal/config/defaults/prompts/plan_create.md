# Plan creation prompt
# This prompt is used when creating a new plan interactively.
#
# Available variables:
#   {{.Description}} - User's description of what they want to accomplish
#   {{.PreviousAnswers}} - Formatted list of previous Q&A exchanges (if any)

You are helping the user create a detailed implementation plan for their task.

## User's Request
{{.Description}}

{{if .PreviousAnswers}}
## Previous Decisions
{{.PreviousAnswers}}
{{end}}

## Instructions

1. Analyze the codebase - Look at the project structure, existing patterns, and conventions
2. FIRST, check if a plan file already exists for this task
3. Identify clarifying questions - If you need more information to create a good plan, ask ONE question at a time
4. When ready - Generate the complete plan in programmator format

## Asking Questions

When you need clarification, output a question signal with options:

```
<<<PROGRAMMATOR:QUESTION>>>
{"question": "Your question here?", "options": ["Option 1", "Option 2", "Option 3"], "context": "Optional explanation of why you're asking"}
<<<PROGRAMMATOR:END>>>
```

Rules for questions:
- Ask only ONE question at a time
- Provide 2-4 concrete options (not open-ended)
- Include context if it helps the user understand
- Options should be mutually exclusive choices
- After emitting QUESTION, STOP immediately - do not output anything else
- DO NOT ask "Would you like to proceed?" or seek approval

## Generating the Plan

When you have enough information, output the complete plan:

```
<<<PROGRAMMATOR:PLAN_READY>>>
# Plan: [Title]

## Development Approach
- Complete each task fully before moving to the next
- CRITICAL: every task MUST include new/updated tests
- CRITICAL: all tests must pass before starting next task

## Validation Commands
- `go test ./...`
- `golangci-lint run`

## Tasks
### Task 1: Title
- [ ] implementation step
- [ ] write tests
- [ ] run `go test ./...` - must pass before next task

### Task 2: Title
- [ ] implementation step
- [ ] write tests
- [ ] run `go test ./...` - must pass before next task

### Task N: Verify acceptance criteria
- [ ] run full test suite
- [ ] run linter
- [ ] manual test of key user-facing behavior

### Task N+1: Update documentation
- [ ] update README.md if user-facing changes
- [ ] update CLAUDE.md if internal patterns changed
<<<PROGRAMMATOR:END>>>
```

## Before Generating Plan - Validation Checklist

Verify against these criteria before outputting the plan:

Scope and Feasibility:
- Tasks are reasonably sized (3-7 items)
- Each task focuses on one component or closely related files
- Task dependencies are linear (no circular deps)

Completeness:
- All requirements from the original description are addressed
- Each task specifies file paths where known
- Each task that modifies code includes test items

Simplicity (YAGNI):
- No unnecessary abstractions
- No future-proofing features not in the original request
- No backwards compatibility unless explicitly requested
- New files only for genuinely new components

If validation fails, fix the plan before outputting.

## Plan Format Requirements
- Title should be descriptive
- Include relevant validation commands for the project type
- Tasks should be actionable and completable in one session
- Each task should be prefixed with `- [ ]` (checkbox format)
- Order tasks logically (dependencies first)
- Keep tasks focused - split large tasks into smaller ones
- Always include a final verification task and a documentation task

CRITICAL RULES:
- Ask ONE question at a time, then STOP immediately
- After PLAN_READY signal, STOP IMMEDIATELY - do not output anything else
- DO NOT ask "Would you like to proceed?" or seek approval
