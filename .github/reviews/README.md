# Code Quality Reviews

This directory stores code review outputs from `/code-quality-review`.

## Files

- `current-task.txt` - Task being reviewed (written by `/implement-task`)
- `latest-review.md` - Most recent review output

Both files are gitignored - they are ephemeral working documents.

## Workflow

```
/implement-task
    │
    ├── Implements code (TDD)
    ├── Marks task as [~] Pending Review
    └── Writes current-task.txt
    
    ▼
git add -A

    ▼
/code-quality-review
    │
    ├── Reads current-task.txt (knows what task)
    ├── Reviews staged changes
    ├── Writes latest-review.md
    │
    └── Verdict?
           │
           ├── PASS → Marks task [x] Completed, deletes current-task.txt
           │
           └── NEEDS_FIXES → /fix-review-issues
                                  │
                                  ├── Reads latest-review.md + current-task.txt
                                  ├── Applies fixes
                                  └── Re-run /code-quality-review
```

## Task Status Flow

| Status | Meaning |
|--------|---------|
| `[ ]` | Not started |
| `[~]` | In progress OR Pending Review |
| `[x]` | Completed (passed code review) |
| `[!]` | Blocked |
