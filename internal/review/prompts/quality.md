Review code for bugs, security issues, and quality problems.

## Evidence

- Avoid speculation. Report only issues you can verify from the code.
- If a concern depends on missing context, mark it as "needs context" and state what context is required.

## Correctness Review

1. Logic errors - off-by-one errors, incorrect conditionals, wrong operators
2. Edge cases - empty inputs, nil/null values, boundary conditions, concurrent access
3. Error handling - all errors checked, appropriate error wrapping, no silent failures
4. Resource management - proper cleanup, no leaks, correct resource release
5. Concurrency issues - race conditions, deadlocks, goroutine leaks
6. Data integrity - validation, sanitization, consistent state management

## Security Analysis

1. Input validation - all user inputs validated and sanitized
2. Authentication/authorization - proper checks in place
3. Injection vulnerabilities - SQL, command, path traversal
4. Secret exposure - no hardcoded credentials or keys
5. Information disclosure - error messages, logs, debug info

## Simplicity Assessment

1. Direct solutions first - if simple approach works, don't use complex pattern
2. No enterprise patterns for simple problems
3. Question every abstraction - must solve real problem
4. No scope creep - changes solve only the stated problem
5. No premature optimization

## What to Report

For each issue:
- Location: exact file path and line number
- Issue: clear description
- Impact: how this affects the code
- Fix: specific suggestion

Focus on defects that would cause runtime failures, security vulnerabilities, or maintainability problems.
Report problems only - no positive observations.
