---
name: plan-to-ticket
description: >
  Create a programmator-compatible ticket from the current plan file with phases, tasks, and acceptance criteria.
allowed-tools: "Read,Bash(ticket:*),Bash(ls:*),Bash(git rev-parse:*),Write"
---

# Plan to Ticket

Converts a Claude Code plan file into a programmator-compatible ticket with phases, tasks, and acceptance criteria.

## Instructions

1. **Find the most recent plan file**:
   ```bash
   ls -lt ~/.claude/plans/*.md | head -1
   ```

2. **Read the full plan content**

3. **Determine project context**: Use current git repo folder name, or "misc" if not in a repo

4. **Extract title**: Get the title from the plan's first `# ` heading

5. **Extract phases and tasks from the plan**:

   **Phases** (high-level stages):
   - Each major implementation step/section becomes a phase
   - Phase format: `- [ ] Phase N: Name - brief actionable description`
   - Keep phase count proportional to plan complexity (don't force 5 phases on a 2-step plan)
   - Phase names must be actionable (what to do, not just a label)
   - If the plan already has checkbox phases, reuse them as-is

   **Tasks** (concrete work items grouped by phase):
   - Break each phase into specific, self-contained changes
   - Group tasks under `### Phase N: Name` subheadings
   - Each task = one logical unit (one function, one endpoint, one component)
   - Use plan text **plus conversation context** to resolve file paths. Add a `Files:` line (e.g., `Files: Create \`path/to/new\`, Modify \`path/to/existing\``) per phase group. Include only paths that are explicit or strongly implied. Omit the `Files:` line entirely if no paths are confidently known.
   - If the plan involves code changes, include test items per task group (write tests, run tests)
   - If a phase has no granular detail in the plan, create 1-2 tasks summarizing that phase's work

6. **Generate acceptance criteria**: From the plan's goal and overview, derive 2-4 concrete "done when" conditions. Each must be verifiable (not subjective). If the plan mentions tests or verification steps, include those.

7. **Create an empty ticket** to get an ID and file path:
   ```bash
   ticket create "[project] title" --type task
   ```

8. **Write the ticket file directly** at `$TICKETS_DIR/<id>.md` with the complete frontmatter and body:

   ```markdown
   ---
   id: <id>
   status: open
   deps: []
   links: []
   created: <timestamp from created ticket>
   type: task
   priority: 2
   ---
   # [project] title

   ## Goal

   <one-sentence goal extracted from plan summary/intro>

   ## Phases

   - [ ] Phase 1: Name - description
   - [ ] Phase 2: Name - description
   ...

   ## Tasks

   ### Phase 1: Name
   Files: Create `path/to/new`, Modify `path/to/existing`
   - [ ] specific action with file reference
   - [ ] another action
   - [ ] write tests for new/changed functionality
   - [ ] run tests — must pass before next phase

   ### Phase 2: Name
   - [ ] ...

   ## Acceptance Criteria

   - Done when X
   - Done when Y

   ## Plan

   <full original plan content — INLINE EVERYTHING, never use $(cat ...) or file references>
   ```

   **CRITICAL**: The plan content must be fully inlined. NEVER use `$(cat ...)`, file path references, or any form of indirection. Copy the entire plan text directly into the ticket body.

9. **Report**: Show the created ticket ID, list the phases, and show task count per phase. Mention that the user can now run `programmator start <ticket-id>` to execute it.
