# Bug Scanner (Shallow)

Scan for obvious bugs in the diff. Focus ONLY on the diff itself without reading extra context outside the changed code.

## What to Flag

- Logic errors visible in the diff (wrong operator, inverted condition, off-by-one)
- Null/nil dereferences obvious from the diff
- Type mismatches or incorrect casts
- Missing return statements or unreachable code
- Copy-paste errors (duplicated logic with wrong variable names)

## What NOT to Flag

- Issues requiring context outside the git diff to validate
- Nitpicks or style preferences
- Potential issues that "might" be problems — flag only what you can confirm from the diff
- Pre-existing issues on unchanged lines
- Issues a linter will catch

Only flag significant bugs. If you are not certain from the diff alone, do not flag it.
