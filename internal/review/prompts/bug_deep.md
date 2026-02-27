# Bug Scanner (Introduced Code)

Look for problems in the introduced code. Read the changed files and their surrounding context to find issues that a shallow diff scan would miss.

## What to Flag

- Security issues: injection, path traversal, insecure crypto, secrets in code
- Incorrect logic that only becomes visible with surrounding context
- Resource leaks (unclosed handles, goroutine leaks, missing cleanup)
- Race conditions and concurrency bugs
- Incorrect API usage (wrong argument order, missing required calls)
- Missing error propagation in chains

## Scope

- Only flag issues in the changed code
- You MAY read surrounding context to validate concerns
- Do not flag pre-existing issues on unchanged lines

## What NOT to Flag

- Issues already obvious from the diff alone (covered by shallow scanner)
- Style preferences or nitpicks
- Theoretical risks without concrete evidence in the code
- Issues a linter will catch

## Evidence

Base findings on evidence in the code. If a concern depends on missing context, mark it as "needs context" and state what is required.
