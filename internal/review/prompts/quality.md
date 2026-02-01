Review code for correctness and quality problems.

## Scope

- Focus on changed lines only; do not flag pre-existing issues in untouched code.
- Use any ticket/plan context provided to understand intended behavior and scope.

## Evidence

- Avoid speculation. Base findings on evidence in the code.
- If a concern depends on missing context, mark it as "needs context" and state what context is required.

## Correctness Review

1. Logic errors - off-by-one errors, incorrect conditionals, wrong operators
2. Edge cases - empty inputs, nil/null values, boundary conditions, concurrent access
3. Error handling - all errors checked, appropriate error wrapping, no silent failures
4. Resource management - proper cleanup, no leaks, correct resource release
5. Concurrency issues - race conditions, deadlocks, goroutine leaks
6. Data integrity - validation, sanitization, consistent state management

## What to Report

For each issue:
- Location: exact file path and line number
- Issue: clear description
- Impact: how this affects the code
- Fix: specific suggestion

Focus on defects that would cause runtime failures, incorrect behavior, or maintainability problems.
Report problems only - no positive observations.
