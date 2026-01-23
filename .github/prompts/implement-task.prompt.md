---
agent: "agent"
name: implement-task
description: Implement a task from the plans directory following TDD
tools: ['execute/runInTerminal', 'read', 'edit/createFile', 'edit/editFiles', 'search', 'agent']
---

# Implement Task

Implement a Chaperone task following TDD principles and the Design Specification.

## Usage

```
/implement-task .github/plans/phase-1-poc/05-context-parsing.task.md
```

Or provide the task path in the chat input.

## Instructions

### Step 1: Read the Task File

Read the task file provided as input (e.g., `.github/plans/phase-1-poc/NN-name.task.md`).

Extract:
- **Objective** - What needs to be done
- **Design Spec Reference** - Which sections to read
- **Dependencies** - Verify these are complete first
- **Acceptance Criteria** - What defines "done"
- **Files to Create** - Target locations

### Step 2: Verify Dependencies

Check that all dependency tasks are marked as completed:
- Read each dependency task file
- If any are incomplete, STOP and report which dependencies need to be done first

### Step 3: Read Design Spec

Use a subagent to extract relevant implementation details:

```
Spawn subagent to read docs/DESIGN-SPECIFICATION.md sections [from task].
Return:
1. Interface definitions (exact code)
2. Struct fields required
3. Behavior requirements
4. Security constraints
5. Error handling patterns
```

### Step 4: TDD - Write Tests First

1. Create test file(s) based on acceptance criteria
2. Follow patterns from `.github/instructions/go-testing.instructions.md`
3. Run tests - they MUST fail initially (red phase)

```bash
go test ./path/to/package/... -v
```

### Step 5: Implement

1. Create implementation following the Design Spec
2. Apply conventions from `.github/copilot-instructions.md`
3. Apply security rules from `.github/instructions/go-security.instructions.md`
4. Apply error handling from `.github/instructions/go-errors.instructions.md`

### Step 6: Verify

1. Run tests - must PASS (green phase)
2. Run linter: `make lint` (requires `golangci-lint` - run `make tools` to install if missing)
3. Check for errors: use get_errors tool

### Step 7: Update Task Status

1. Update the task file:
   - Change status from `[ ] Not Started` to `[x] Completed`
   - Mark ALL acceptance criteria checkboxes as `[x]` (including tests/lint items)
2. Update `.github/plans/phase-N-name/_index.md` progress table

## Output Format

```markdown
## Task Completed ✓

**Task:** [task file path]
**Objective:** [one-line summary]

### Files Created/Modified
- `path/file.go` - [description]
- `path/file_test.go` - [tests added]

### Test Results
- X tests passing
- All acceptance criteria met

### Next Suggested Task
Based on the dependency graph, the next task to work on is:
`NN-next-task.task.md` - [brief description]
```

## If Blocked

If implementation cannot proceed:
1. Note the blocker in the task file under a new "## Blockers" section
2. Update status to `[!] Blocked: [reason]`
3. Suggest alternative tasks that can proceed
