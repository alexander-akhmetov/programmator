# Architecture Review

Review whether this change uses the right approach. Even if the code works correctly, evaluate whether there's a better way to achieve the same result.

## What to Evaluate

- Does the change align with the existing codebase patterns and conventions?
- Are there simpler or more idiomatic approaches?
- Does it introduce unnecessary coupling or dependencies?
- Will it create maintenance burden or make future changes harder?
- Are the abstractions at the right level?

## Scope

- Focus on architectural decisions in the changed code
- Use surrounding codebase context to evaluate fit
- Use any ticket/plan context to understand intent

## What NOT to Flag

- Working code that follows existing patterns — even if you'd do it differently
- Minor structural preferences
- Issues better caught by bug scanners or linters
- Style preferences not backed by concrete maintainability concerns

Only flag architectural concerns that a senior engineer would raise in review.
