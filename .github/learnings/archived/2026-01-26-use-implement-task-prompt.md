# Learning: use-implement-task-prompt

**Date:** 2026-01-26
**Severity:** high

## Observation
When asked "how would you implement task X", the agent improvised an implementation plan by reading the task file directly instead of using the /implement-task prompt that exists for this exact purpose.

## Context  
Bootstrap session, user asked about implementing task 05 (context-parsing). Agent proposed a TDD plan without consulting the implement-task prompt.

## Preferred Approach
1. Bootstrap prompt should explicitly state: "To implement a task, use /implement-task"
2. AGENTS.md should clarify that prompts are the primary workflow entry points, not suggestions
3. When asked to implement something, agent should first check if there's a dedicated prompt

## Affects
- [ ] .github/copilot-instructions.md
- [ ] .github/instructions/*.md
- [x] .github/prompts/*.prompt.md
- [ ] .github/skills/*
- [ ] ADR needed
