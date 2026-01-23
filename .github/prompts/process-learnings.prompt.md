---
agent: "agent"
description: Process captured learnings and convert them into workflow updates
tools: ['read/readFile', 'edit/createFile', 'edit/editFiles', 'search']
---

# Process Learnings

Review recent learnings captured in `.github/learnings/` and propose updates to the workflow.

## Context

You are processing learnings for the **Chaperone** project - a Go-based egress proxy.
The learnings directory contains observations about code style, patterns, or workflow that need to be incorporated.

## Instructions

1. **Read all unprocessed learnings** in `.github/learnings/` (ignore `archived/` and `README.md`)

2. **For each learning, determine the action:**
   - **Instructions Update**: Modify `.github/copilot-instructions.md` or create modular instruction file
   - **Prompt Update**: Modify or create `.github/prompts/*.prompt.md`
   - **Skill Update**: Modify or create `.github/skills/*/SKILL.md`
   - **ADR Needed**: Create entry in docs or note in Design Spec

3. **Group related learnings** - Multiple learnings about Go error handling → single coherent update

4. **Apply changes** following these rules:
   - Keep instructions concise and actionable
   - Include concrete examples (good vs bad)
   - Reference Design Spec sections when relevant
   - Maintain existing structure/formatting

5. **Move processed learnings** to `.github/learnings/archived/`

6. **Summarize changes** made for the user

## Variables

- `{{learnings_path}}` - Path to learnings directory (default: `.github/learnings/`)

## Output

Provide a summary:
```
## Learnings Processed

### [Learning Title]
- **Action**: Updated `.github/instructions/go-errors.md`
- **Change**: Added preference for `errors.Join` pattern
- **Moved to**: `archived/2026-01-23-error-handling.md`
```
