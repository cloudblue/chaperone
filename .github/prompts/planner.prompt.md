---
agent: "agent"
name: planner
description: Generate implementation tasks for a phase by analyzing ROADMAP and Design Spec
tools: ['vscode', 'execute', 'read', 'edit', 'search', 'web', 'agent', 'todo']
---

# Implementation Planner

Generate granular implementation tasks for a Chaperone development phase.

## Usage

```
/planner 1
```

Where the argument is the phase number (1, 2, 3, or 4).

## Instructions

### Step 1: Read Source Documents

Read the authoritative sources:

1. `docs/ROADMAP.md` - Get the task list for the requested phase
2. `docs/explanation/DESIGN-SPECIFICATION.md` - Get implementation details

### Step 2: Analyze Phase Requirements

Use a subagent to deeply analyze the Design Spec:

```
Subagent task: "Analyze docs/explanation/DESIGN-SPECIFICATION.md for Phase {{phase}} implementation.

For each item in Phase {{phase}} of the ROADMAP, identify:
1. Which Design Spec sections are relevant
2. What interfaces/structs need to be created
3. What dependencies exist (both explicit and implicit)
4. What security considerations apply (Section 5.3)
5. What testing is required (Section 9)

Also identify any MISSING tasks that are prerequisites but not listed in ROADMAP.

Return a structured list of tasks with dependencies."
```

### Step 3: Create Task Files

For each identified task, create `.github/plans/phase-{{phase}}-<name>/NN-<task-name>.task.md`:

**Task File Template:**

```markdown
# Task: [Task Name]

**Status:** [ ] Not Started
**Priority:** [P0/P1/P2]
**Estimated Effort:** [S/M/L/XL]

## Objective

[Clear, single-sentence goal]

## Design Spec Reference

- **Primary:** Section X.Y - [Title]
- **Related:** Section A.B - [Title]

## Dependencies

- [ ] `NN-previous-task.task.md` - [why needed]
- [ ] External: [any external dependency]

## Acceptance Criteria

- [ ] [Specific, testable criterion]
- [ ] [Another criterion]
- [ ] Tests pass: `go test ./...`
- [ ] Lint passes: `make lint`

## Implementation Hints

### Suggested Approach

1. [Step 1]
2. [Step 2]

### Key Code Locations

- `internal/...` - [what goes here]
- `sdk/...` - [if applicable]

### Gotchas

- [Common mistake to avoid]

## Files to Create/Modify

- [ ] `path/to/file.go` - [purpose]
- [ ] `path/to/file_test.go` - [test coverage]

## Testing Strategy

- Unit tests: [what to test]
- Integration tests: [if applicable]
- Compliance tests: [if SDK-related]
```

### Step 4: Create Phase Index

Create `.github/plans/phase-{{phase}}-<name>/_index.md`:

```markdown
# Phase {{phase}}: [Phase Name]

**Goal:** [From ROADMAP]
**Status:** In Progress | Not Started | Completed

## Task Overview

| # | Task | Status | Priority | Dependencies |
|---|------|--------|----------|--------------|
| 01 | [Name] | [ ] | P0 | - |
| 02 | [Name] | [ ] | P0 | 01 |
| ... | ... | ... | ... | ... |

## Dependency Graph

```
01-core-skeleton
    └── 02-context-parsing
        ├── 03-context-hashing
        └── 04-plugin-interface
```

## Notes

- [Any phase-level notes]
```

### Step 5: Handle Regeneration

If `{{regenerate}}` is true:

1. Read existing `_index.md` to find completed tasks `[x]`
2. Preserve completed task files
3. Update/add new tasks
4. Mark removed ROADMAP items as `[?] Review needed`

## Output

After generating, report:

```
## Phase {{phase}} Plan Generated

**Tasks created:** X
**Dependencies identified:** Y
**Implicit tasks added:** Z (tasks not in ROADMAP but needed)

### Task List
1. NN-name.task.md - [brief description]
2. ...

### Dependency Warnings
- [Any circular or unclear dependencies]

### Ready to Start
First task with no blockers: `NN-name.task.md`
```

## Quality Checks

Before completing, verify:

- [ ] Each task has clear acceptance criteria
- [ ] Dependencies form a DAG (no cycles)
- [ ] All Design Spec sections are referenced
- [ ] Security considerations are addressed
- [ ] Testing strategy is defined for each task
