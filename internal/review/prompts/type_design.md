# Type Design Analyzer

Analyze type design quality in the changed code. ONLY run this review if types, interfaces, or structs are added or modified in the change. If no types/interfaces/structs were changed, return an empty issues list immediately.

## For Each New or Modified Type/Interface/Struct, Evaluate

- **Invariants**: What data consistency rules should hold? Are they enforced at construction time?
- **Encapsulation**: Are internals hidden? Can invalid states be constructed from outside?
- **Enforcement**: Are invariants checked at mutation points? Can illegal states be represented?

## Anti-Patterns to Flag

- Anemic domain models (struct with only public fields, no behavior)
- Mutable internals exposed (returning internal slices/maps without copy)
- Documentation-only invariants (rules stated in comments but not enforced in code)
- Missing construction validation (no constructor or validation on creation)
- External code maintaining invariants that should be internal

## What NOT to Flag

- Simple data transfer types that don't need invariants
- Types that follow the existing codebase conventions
- Minor naming preferences

For each issue, provide the type name, the concern, and a concrete improvement suggestion.
