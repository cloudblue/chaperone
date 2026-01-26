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

### Step 7: Mark Pending Review

1. Update the task file:
   - Change status from `[ ] Not Started` to `[~] Pending Review`
   - Mark implementation acceptance criteria as `[x]` (but NOT the "task completed" box)
2. Update `.github/plans/phase-N-name/_index.md` progress table to `[~]`
3. Write task path to `.github/reviews/current-task.txt` for review tracking:
   ```bash
   echo ".github/plans/phase-1-poc/NN-task-name.task.md" > .github/reviews/current-task.txt
   ```

**Note:** Task will be marked `[x] Completed` by `/code-quality-review` when it passes.

## Output Format

```markdown
## Implementation Ready for Review

**Task:** [task file path]
**Status:** Pending Review
**Objective:** [one-line summary]

### Files Created/Modified
- `path/file.go` - [description]
- `path/file_test.go` - [tests added]

### Test Results
- X tests passing
- All acceptance criteria met

### Next Step: Code Review
Stage your changes and run the independent review:
```bash
git add -A
```
Then run: `/code-quality-review`

The review will:
- Validate code quality
- Mark task as `[x] Completed` if PASS
- Or identify issues to fix with `/fix-review-issues`
```

## If Blocked

If implementation cannot proceed:
1. Note the blocker in the task file under a new "## Blockers" section
2. Update status to `[!] Blocked: [reason]`
3. Suggest alternative tasks that can proceed
