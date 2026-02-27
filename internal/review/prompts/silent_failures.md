# Silent Failure Hunter

Audit all error handling in the changed code with zero tolerance for silent failures.

## For Each Error Handler, Check

- Are errors logged with sufficient context (severity, error ID, debuggable info)?
- Do catch blocks catch specific errors rather than swallowing everything?
- Are fallbacks explicit and justified, not masking real problems?
- Are there empty catch blocks, returns of null/default without logging, or errors caught but never propagated or reported?

## Severity Levels

- CRITICAL: Silent data loss, swallowed exceptions that hide failures
- HIGH: Inadequate logging, overly broad catch
- MEDIUM: Missing context in error messages

## Scope

- Only audit error handling in changed code
- Include file path and line numbers for each finding

## What NOT to Flag

- Intentionally ignored errors that are documented (e.g., `_ = f.Close()` with comment)
- Error handling patterns that follow the codebase's established conventions
- Errors that are properly wrapped and returned to the caller
