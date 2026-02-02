---
name: plan-to-file
description: >
  Convert the current Claude Code plan into a programmator-compatible plan file in the current project.
allowed-tools: "Read,Bash(ls:*),Write,AskUserQuestion"
---

# Plan to File

Converts a Claude Code plan file into a programmator-compatible plan file.

## Instructions

1. **Find the most recent plan file**:
   ```bash
   ls -lt ~/.claude/plans/*.md | head -1
   ```

2. **Read the plan content**

3. **Extract title**: Get the title from the plan's first `# ` heading. Strip any "Plan:" prefix if already present.

4. **Extract tasks from the plan**: Analyze the plan structure and identify the logical steps. Rules:
   - Each major implementation step/section becomes a task
   - Task format: `- [ ] Task N: Name - brief actionable description`
   - Keep task count proportional to plan complexity
   - Task names must be actionable (what to do, not just a label)
   - If the plan already has checkbox tasks, reuse them as-is

5. **Detect validation commands**: Read `CLAUDE.md` in the current project root (and any `CLAUDE.md` files it references) to find test, lint, and build commands. Look for sections like "Build and Test Commands", "Testing", "Linting", etc. Extract the relevant commands (e.g., `go test ./...`, `make lint`). If `CLAUDE.md` doesn't exist or has no such commands, fall back to scanning the plan text for mentions of test/lint/build commands. If nothing is found, omit the validation section.

6. **Build the plan file** with this structure:

   ```markdown
   # Plan: <title>

   ## Validation Commands
   - `<command 1>`
   - `<command 2>`

   ## Tasks
   - [ ] Task 1: Name - description
   - [ ] Task 2: Name - description
   ...
   ```

7. **Check output path**: If `./plan.md` already exists in the current directory, ask the user what filename to use. Otherwise default to `./plan.md`.

8. **Write the file** to the chosen path.

9. **Report**: Show the written filename, the extracted title, task count, and any validation commands found. Mention that the user can now run `programmator start ./plan.md` to execute it.
