# Modular Instructions

This directory contains topic-specific instruction files that are automatically included based on file patterns.

## How It Works

Files matching `*.instructions.md` with an `applyTo` frontmatter are automatically loaded when editing matching files.

## Structure

```yaml
---
applyTo: "**/*.go"  # Glob pattern for when to include
---
# Instructions content...
```

## Current Files

| File | Applies To | Purpose |
|------|------------|---------|
| `go-errors.instructions.md` | `**/*.go` | Error handling conventions |
| `go-testing.instructions.md` | `**/*_test.go` | Test writing conventions |
| `go-security.instructions.md` | `**/*.go` | Security-sensitive code rules |

## Adding New Instructions

Use the `update-go-instructions` prompt or create manually following the pattern.

## Relationship to copilot-instructions.md

- **Main file**: General project context, architecture, ADRs
- **Modular files**: Specific, detailed conventions per topic
- **No duplication**: Modular files extend, not repeat
