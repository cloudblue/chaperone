---
mode: agent
description: Update Go-specific coding instructions based on a preference or pattern
tools: ['read_file', 'create_file', 'replace_string_in_file', 'file_search']
---

# Update Go Instructions

Update or create Go-specific coding instructions for the Chaperone project.

## Context

Chaperone is a Go project following strict conventions. Go-specific instructions can be:
1. In `.github/copilot-instructions.md` (main file)
2. In `.github/instructions/go-*.instructions.md` (modular, auto-included)

## Variables

- `{{category}}` - What aspect of Go? (errors, testing, concurrency, logging, style)
- `{{pattern}}` - The pattern or convention to add/update
- `{{example_good}}` - Example of correct code
- `{{example_bad}}` - Example of what to avoid (optional)
- `{{rationale}}` - Why this pattern? (optional)

## Instructions

1. **Check if modular instruction file exists** for this category:
   - `.github/instructions/go-{{category}}.instructions.md`

2. **If exists**: Update the file with the new pattern
   - Add to appropriate section
   - Include examples
   - Don't duplicate existing content

3. **If doesn't exist**: Create the modular file with structure:
   ```markdown
   ---
   applyTo: "**/*.go"
   ---
   # Go {{Category}} Conventions

   ## Patterns

   ### {{pattern}}
   
   {{rationale}}

   ```go
   // ✅ Correct
   {{example_good}}

   // ❌ Avoid
   {{example_bad}}
   ```
   ```

4. **Verify consistency** with main `copilot-instructions.md` - no contradictions

## Output

Confirm what was updated and where.
