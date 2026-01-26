# Learning: bootstrap-task-tracking

**Date:** 2026-01-26
**Severity:** high
**Processed:** 2026-01-26

## Observation
The bootstrap prompt reports phase progress by reading tasks directly from `ROADMAP.md`. However, the actual implementation workflow uses a three-tier hierarchy:

1. **ROADMAP.md** - High-level deliverables per phase
2. **Plans** (generated via `planner` prompt) - Detailed task breakdown stored in `.github/plans/`
3. **Implementation** (via `implement-task` prompt) - Executes individual tasks from plans

The ROADMAP tasks are coarse-grained milestones, while the plan files contain the granular, trackable tasks with completion status (`[x]` / `[ ]`).

## Context  
During bootstrap session initialization. The prompt showed "0/6 tasks complete" based on ROADMAP checkboxes, but actual progress should be tracked from the generated plan files in `.github/plans/`.

## Preferred Approach
The bootstrap prompt should:
1. Check for existing plan files in `.github/plans/` directory
2. If plans exist for current phase, read task status from those files
3. Report progress based on plan tasks, not ROADMAP milestones
4. If no plans exist, suggest running `planner` prompt first

Example flow:
```
1. Read ROADMAP.md → Identify current phase
2. Check .github/plans/phase-1-*.md → Get actual tasks
3. Count completed ([x]) vs total tasks
4. Report: "Phase 1 Progress: 3/12 tasks complete"
```

## Resolution
Updated `.github/prompts/bootstrap.prompt.md`:
- Added Step 3 to read task progress from `.github/plans/` directory
- Changed workflow to check plan files before reporting progress
- Added fallback suggestion to run `/planner` if no plans exist
- Renumbered steps (now 6 steps instead of 5)
