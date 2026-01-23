---
mode: agent
description: Bootstrap a new session with full project context
tools: ['read_file', 'list_dir', 'file_search', 'runSubagent', 'run_in_terminal']
---

# Bootstrap Session

Initialize a new chat session with full Chaperone project context.

## Instructions

Perform these steps to load project context:

### Step 1: Read Entry Point

Read `AGENTS.md` - This gives you the project overview and pointers to all key files.

### Step 2: Use Subagent for Design Spec Summary

Spawn a subagent to read and summarize the Design Spec:

```
Subagent task: Read docs/DESIGN-SPECIFICATION.md and docs/ROADMAP.md.
Return:
1. Current phase name and number
2. List of tasks in current phase with completion status
3. Key ADRs that affect implementation
4. Any blocking dependencies between tasks
```

This keeps the main context clean while getting necessary information.

### Step 3: Check for Unprocessed Learnings

List `.github/learnings/` directory. If there are unprocessed learnings (not in `archived/`):
- Note them for the user
- Suggest running `process-learnings` prompt

### Step 4: Quick Codebase State

Run `git log --oneline -5` to see recent activity.
Check what exists in `internal/` and `sdk/`.

### Step 5: Report Ready Status

Provide a summary:

```
## Session Bootstrapped ✓

**Current Phase:** Phase 1 (PoC)
**Phase Progress:** X/Y tasks complete

**Unprocessed Learnings:** [none | X learnings pending]

**Codebase State:**
- SDK: [status]
- Internal: [status]
- Reference Plugin: [status]

**Ready for:** [suggested next task based on phase]
```

## Output

After bootstrapping, ask the user what they'd like to work on, offering suggestions based on the current phase's incomplete tasks.
