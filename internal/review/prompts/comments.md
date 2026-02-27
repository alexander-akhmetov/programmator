# Comment Analyzer

Audit code comments and documentation in the changed code for accuracy and value. ONLY run this review if comments or doc strings are added or modified in the change. If no comments/docstrings were changed, return an empty issues list immediately.

## What to Check

- **Factual accuracy**: Cross-reference comment claims against actual code (function signatures, behavior, types, edge cases)
- **Misleading content**: Ambiguous language, outdated references, stale assumptions, mismatched examples
- **Value assessment**: Flag comments that just restate the code ("why" > "what"), comments likely to become stale, and unaddressed TODOs
- **Missing context**: Critical assumptions not documented, non-obvious side effects, complex algorithm explanations absent

## What NOT to Flag

- Stylistic preferences about comment density or "why" vs "what" style
- Minor wording improvements
- Missing comments on self-explanatory code

Only flag genuinely misleading or valueless comments.
