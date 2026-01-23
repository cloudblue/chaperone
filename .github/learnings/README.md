# Learnings Directory

This directory captures observations, preferences, and insights discovered during development.

## How to Use

1. **Capture quickly**: Create `YYYY-MM-DD-topic.md` when you notice something
2. **Process periodically**: Run the `process-learnings` prompt to convert into updates
3. **Archive processed**: Move to `archived/` after processing

## Learning Template

````markdown
# Learning: [Brief Title]

**Date:** YYYY-MM-DD
**Severity:** low | medium | high (how much does this affect workflow?)

## Observation
What did you notice? Be specific.

## Context  
- File/task where this came up
- What were you trying to do?

## Preferred Approach
What should we do instead?

## Example
```go
// Bad (what we want to avoid)
...

// Good (what we prefer)
...
```

## Affects (check all that apply)
- [ ] .github/copilot-instructions.md (general)
- [ ] .github/instructions/go-*.md (Go-specific)
- [ ] .github/prompts/*.prompt.md (specific prompt)
- [ ] .github/skills/* (skill update needed)
- [ ] ADR needed (architectural decision)
- [ ] Design Spec addendum
````

## Categories

| Category | Examples |
|----------|----------|
| `code-style` | Formatting, naming, patterns |
| `error-handling` | Error wrapping, sentinel errors |
| `testing` | Test structure, mocking approaches |
| `security` | New redaction rules, validation |
| `architecture` | Component boundaries, interfaces |
| `tooling` | Build, lint, CI adjustments |
| `workflow` | Prompt/skill effectiveness |
