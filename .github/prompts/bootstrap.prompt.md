---
agent: "agent"
description: Bootstrap a new session with full project context
tools: ['execute', 'read/terminalLastCommand', 'read/getTaskOutput', 'read/readFile', 'search', 'web', 'agent']
---

# Bootstrap Session

Initialize a new chat session with full Chaperone project context.

## Instructions

Perform these steps to load project context:

### Step 1: Read Entry Point

Read `AGENTS.md` - This gives you the project overview and pointers to all key files.

### Step 2: Identify Current Phase

Read `docs/ROADMAP.md` to identify:
1. Current phase name and number
2. Key ADRs that affect implementation

### Step 3: Get Task Progress from Plans

**Important:** Task progress is tracked in plan files, NOT in ROADMAP.md.

The workflow hierarchy is: `ROADMAP.md` (milestones) → `Plans` (tasks) → `Implementation`

1. List `.github/plans/` directory
2. If plan files exist for current phase (e.g., `phase-1-*.md`):
   - Read the plan files
   - Count completed tasks (`[x]`) vs total tasks (`[ ]`)
   - Note any blocked or in-progress tasks
3. If NO plans exist for current phase:
   - Report "No plan generated yet"
   - Suggest running `planner` prompt first

### Step 4: Check for Unprocessed Learnings

List `.github/learnings/` directory. If there are unprocessed learnings (not in `archived/`):
- Note them for the user
- Suggest running `process-learnings` prompt

### Step 5: Quick Codebase State

Run `git log --oneline -5` to see recent activity.
Check what exists in `internal/` and `sdk/`.

### Step 6: Report Ready Status

Provide a summary:

```
## Session Bootstrapped ✓

**Current Phase:** Phase 1 (PoC)
**Phase Progress:** X/Y tasks complete (from .github/plans/)
  - [If no plans]: "No plan generated - run /planner first"

**Unprocessed Learnings:** [none | X learnings pending]

**Codebase State:**
- SDK: [status]
- Internal: [status]
- Reference Plugin: [status]

**Next Task:** [first incomplete task from plan, or suggest /planner]
```

## Output

After bootstrapping, ask the user what they'd like to work on, suggesting:
- The next incomplete task from the plan (if plans exist)
- Running `/planner` to generate tasks (if no plans exist)

## Important: Use Prompts for Actions

**Prompts are the primary workflow entry points, not suggestions.**

When the user wants to:
- **Implement a task** → Use `/implement-task`
- **Plan a phase** → Use `/planner`
- **Capture a learning** → Use `/capture-learning`

Do NOT improvise implementation plans by reading task files directly. Always use the dedicated prompt.
