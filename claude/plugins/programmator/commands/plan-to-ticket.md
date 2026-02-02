---
name: plan-to-ticket
description: >
  Create a programmator-compatible ticket from the current plan file with phases and full plan content.
allowed-tools: "Read,Bash(ticket:*),Bash(ls:*),Bash(git rev-parse:*),Write"
---

# Plan to Ticket

Converts a Claude Code plan file into a programmator-compatible ticket with phases.

## Instructions

1. **Find the most recent plan file**:
   ```bash
   ls -lt ~/.claude/plans/*.md | head -1
   ```

2. **Read the plan content**

3. **Determine project context**: Use current git repo folder name, or "misc" if not in a repo

4. **Extract title**: Get the title from the plan's first `# ` heading

5. **Extract phases from the plan**: Analyze the plan structure and identify the major logical steps. Map each step to a phase. Rules:
   - Each major implementation step/section becomes a phase
   - Phase format: `- [ ] Phase N: Name - brief actionable description`
   - Keep phase count proportional to plan complexity (don't force 5 phases on a 2-step plan)
   - Phase names must be actionable (what to do, not just a label)
   - If the plan already has checkbox phases, reuse them as-is

6. **Detect validation commands**: Read `CLAUDE.md` in the current project root (and any `CLAUDE.md` files it references) to find test, lint, and build commands. Look for sections like "Build and Test Commands", "Testing", "Linting", etc. Extract the relevant commands (e.g., `go test ./...`, `make lint`). If `CLAUDE.md` doesn't exist or has no such commands, fall back to scanning the plan text for mentions of test/lint/build commands. If nothing is found, omit the validation section.

7. **Build the ticket body**: Write a temp file with this structure:

   ```markdown
   ## Goal
   <one-sentence goal extracted from plan summary/intro>

   ## Validation Commands
   - `<command 1>`
   - `<command 2>`

   ## Phases
   - [ ] Phase 1: Name - description
   - [ ] Phase 2: Name - description
   ...

   ## Plan
   <full original plan content verbatim>
   ```

   Omit the Validation Commands section if no commands were found. Write this to a temp file (e.g., `/tmp/plan-ticket-body.md`).

8. **Create ticket**:
   ```bash
   ticket create "[project] title" --type task -d "$(cat /tmp/plan-ticket-body.md)"
   ```

9. **Report**: Show the created ticket ID and list the phases that were extracted. Mention that the user can now run `programmator start <ticket-id>` to execute it.
