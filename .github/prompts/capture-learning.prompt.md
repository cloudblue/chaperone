---
mode: agent
description: Quickly capture a learning without leaving your current task
tools: ['create_file']
---

# Capture Learning

Quickly capture an observation about code style, patterns, or workflow.

## Instructions

Create a learning file at `.github/learnings/{{date}}-{{topic}}.md` with the provided observation.

## Variables

- `{{date}}` - Today's date in YYYY-MM-DD format
- `{{topic}}` - Short kebab-case topic (e.g., `error-handling`, `test-structure`)
- `{{observation}}` - What did you notice?
- `{{context}}` - Where did this come up? (file, task)
- `{{preference}}` - What should we do instead?
- `{{severity}}` - How important? (low, medium, high) - default: medium

## Template to Create

```markdown
# Learning: {{topic}}

**Date:** {{date}}
**Severity:** {{severity}}

## Observation
{{observation}}

## Context  
{{context}}

## Preferred Approach
{{preference}}

## Affects
- [ ] .github/copilot-instructions.md
- [ ] .github/instructions/*.md
- [ ] .github/prompts/*.prompt.md
- [ ] .github/skills/*
- [ ] ADR needed
```

## After Creation

Confirm the file was created and remind user to run `process-learnings` later.
