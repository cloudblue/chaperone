---
agent: agent
name: fix-review-issues
description: Fix issues identified by code-quality-review
tools: ['execute', 'read', 'edit', 'search', 'web', 'agent', 'todo']
---

# Fix Review Issues

Address issues identified by `/code-quality-review` to achieve merge readiness.

## Usage

Run this prompt after `/code-quality-review` returns `NEEDS_FIXES` verdict.

### Step 0: Load Review and Task Context

1. Read `.github/reviews/latest-review.md` to get the review findings
2. Read `.github/reviews/current-task.txt` to get the task being fixed
3. Read the task file to understand acceptance criteria and design requirements

If files don't exist or are stale, ask user to run `/code-quality-review` first.

## Instructions

### Step 1: Parse Review Findings

From the review output, extract:
1. **Verdict** - Should be `NEEDS_FIXES` (if `NEEDS_REDESIGN`, stop and discuss with user)
2. **Critical issues** - Must fix all
3. **Warning issues** - Fix all or most
4. **File list** - Which files need changes

### Step 2: Prioritize Fixes

Order of priority:
1. 🔴 **Critical** - Security, test failures, breaking changes
2. 🟠 **Warning** - Code smells, missing tests, bugs
3. 🟡 **Suggestion** - Only if quick and non-invasive

### Step 3: Apply Fixes

For each issue:

1. **Read the relevant code** around the reported line
2. **Understand the issue** in context
3. **Apply the fix** following project conventions:
   - Error handling: `.github/instructions/go-errors.instructions.md`
   - Security: `.github/instructions/go-security.instructions.md`
   - Testing: `.github/instructions/go-testing.instructions.md`

### Step 4: Verify Fixes

After all fixes:

```bash
make lint       # Must pass
make test       # Must pass
```

Use `get_errors` tool to check for any remaining issues.

### Step 5: Report Completion

## Output Format

```markdown
## Review Issues Fixed ✓

**Task:** [task file path from current-task.txt]
**Issues Addressed:**
- 🔴 Critical: X fixed
- 🟠 Warning: Y fixed
- 🟡 Suggestion: Z addressed (optional)

### Changes Made

| File | Line | Issue | Fix Applied |
|------|------|-------|-------------|
| `path/file.go` | 45 | Error ignored | Added error handling with context |
| `path/file.go` | 78 | Missing godoc | Added documentation |

### Verification
- **Lint:** PASS
- **Tests:** PASS (X tests)

### Next Step
Stage the fixes and re-run the review:
```bash
git add -A
```
Then run: `/code-quality-review`

On PASS, the task will be marked as completed.
```

---

## Fix Patterns

### Common Error Handling Fixes

```go
// Issue: Error ignored
// Before:
result, _ := someFunction()

// After:
result, err := someFunction()
if err != nil {
    return fmt.Errorf("operation context: %w", err)
}
```

### Common Security Fixes

```go
// Issue: Credentials logged
// Before:
slog.Info("request", "headers", req.Header)

// After:
slog.Info("request", "headers", redactHeaders(req.Header))
```

### Common Test Fixes

```go
// Issue: Missing error case test
// Add a new test case to the table:
{
    name:    "invalid input returns error",
    input:   "",
    wantErr: true,
    errType: ErrInvalidInput,
},
```

---

## When to Stop

**Do NOT attempt to fix if:**

1. **Verdict was `NEEDS_REDESIGN`** - Requires architectural discussion
2. **Fix would change public API** - Needs version bump consideration
3. **Fix conflicts with Design Spec** - Needs spec clarification
4. **You're uncertain** - Ask user for guidance

In these cases, report the blocker and ask user how to proceed.

---

## After Fixing

Recommend user to:
1. Run `/code-quality-review` again to verify
2. If PASS, stage and commit
3. If still NEEDS_FIXES, iterate

The goal is to reach `PASS` verdict.
