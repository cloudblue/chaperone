# Meta-Workflow: Self-Improving Development System

This document describes how the Chaperone project's AI-assisted workflow operates and improves itself.

## Bootstrapping a New Session

**Entry Point:** `AGENTS.md` at project root

When starting a fresh chat session:
```
@workspace /prompt bootstrap
```

This loads project context, checks current phase, and reports ready status.

## Overview

```
┌──────────────────────────────────────────────────────────────────┐
│                    BOOTSTRAP SESSION                             │
│              Read AGENTS.md → Load context → Check phase         │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                    IMPLEMENTATION WORK                           │
│                (prompts, skills, subagents)                      │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼ Notice something
┌──────────────────────────────────────────────────────────────────┐
│                    CAPTURE LEARNING                              │
│              .github/learnings/YYYY-MM-DD-topic.md               │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼ Periodically
┌──────────────────────────────────────────────────────────────────┐
│                   PROCESS LEARNINGS                              │
│              Update instructions, prompts, skills                │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                  IMPROVED WORKFLOW                               │
└──────────────────────────────────────────────────────────────────┘
```

## Quick Reference

### Bootstrap (New Session)
```
@workspace /prompt bootstrap
```
Loads context, checks phase, reports ready status.

### Check Phase Scope (Before Starting Work)
```
@workspace /prompt check-phase-scope
```
Variables: `task` - Verifies work fits current development phase.

### Capture a Learning (Fast)
```
@workspace /prompt capture-learning
```
Variables: `topic`, `observation`, `preference`

### Process Learnings (Batch)
```
@workspace /prompt process-learnings
```
Converts captured learnings into instruction/prompt updates.

### Update Go Instructions (Specific)
```
@workspace /prompt update-go-instructions
```
Variables: `category` (errors, testing, security), `pattern`, `example_good`, `example_bad`

### Refine a Prompt
```
@workspace /prompt refine-prompt
```
Variables: `prompt_file`, `problem`, `desired`

### Create a New Skill
```
@workspace /prompt create-skill
```
Variables: `skill_name`, `description`, `trigger`, `steps`

## File Structure

```
.github/
├── copilot-instructions.md      # Main project context
├── instructions/                 # Modular, topic-specific
│   ├── go-errors.instructions.md
│   ├── go-testing.instructions.md
│   └── go-security.instructions.md
├── prompts/                      # Reproducible tasks
│   ├── bootstrap.prompt.md           # Session initialization
│   ├── check-phase-scope.prompt.md   # Verify work fits phase
│   ├── capture-learning.prompt.md
│   ├── process-learnings.prompt.md
│   ├── update-go-instructions.prompt.md
│   ├── refine-prompt.prompt.md
│   └── create-skill.prompt.md
├── learnings/                    # Captured observations
│   ├── README.md
│   └── archived/                 # Processed learnings
└── skills/                       # Complex workflows
    └── [future skills]
```

## Workflow Patterns

### Pattern 0: Starting Fresh Session
1. Open new Copilot chat
2. Run `@workspace /prompt bootstrap`
3. Get context loaded, phase status, ready to work

### Pattern 1: Quick Style Fix
1. You notice: "I prefer X over Y in Go"
2. Quick capture: Run `capture-learning` with the observation
3. Later: Run `process-learnings` to update instructions

### Pattern 2: Prompt Not Working Well
1. You notice: "This prompt produces too verbose output"
2. Run `refine-prompt` with the problem and desired behavior
3. Prompt is updated with refinement

### Pattern 3: New Complex Workflow Needed
1. You realize: "Implementing SDK interfaces always follows these 5 steps"
2. Run `create-skill` to package the workflow
3. Future work uses the skill

### Pattern 4: Architecture Decision
1. You decide: "We'll use approach X for caching"
2. Capture as learning with `affects: ADR needed`
3. Processing creates/updates ADR documentation

### Pattern 5: Check Before Starting
1. You're about to start a new feature
2. Run `check-phase-scope` to verify it fits current phase
3. Get guidance on whether to proceed or defer

### Pattern 6: Research with Subagent
1. You need to research Design Spec or gather context
2. Spawn a subagent with focused task (avoids context pollution)
3. Subagent returns summary, you continue with clean context

## Types of Updates

| Trigger | Action | Updated File |
|---------|--------|--------------|
| Code style preference | Update Go instructions | `instructions/go-*.md` |
| New security rule | Update security instructions | `instructions/go-security.md` |
| Prompt produces bad output | Refine prompt | `prompts/*.prompt.md` |
| Complex multi-step task | Create skill | `skills/*/SKILL.md` |
| Architecture decision | Create/update ADR | `copilot-instructions.md` or docs |
| New convention discovered | Update main instructions | `copilot-instructions.md` |

## Critical Rules

### Rule 1: DRY Documentation (Don't Repeat Yourself)

**Never copy content between files. Point to authoritative sources instead.**

| Information Type | Authoritative Source | Other files should... |
|------------------|---------------------|----------------------|
| Architecture, ADRs | `docs/DESIGN-SPECIFICATION.md` | Reference "See Section X" |
| Phase tasks, progress | `docs/ROADMAP.md` | Say "Read ROADMAP.md" |
| Go conventions | `.github/copilot-instructions.md` | Say "See copilot-instructions" |
| Security rules | `.github/instructions/go-security.instructions.md` | Reference it |

**Why:** Copied content gets out of sync. Let agents autodiscover from sources.

**If you find duplication:** Create a learning to fix it.

### Rule 2: Subagents for Focused Context

Use `runSubagent` when:
- Task requires reading large documents (Design Spec sections)
- Research that would pollute current conversation
- Implementation tasks that need isolated focus
- You want a fresh context window for a specific task

**Don't use subagents for:**
- Quick questions
- Simple file edits
- Tasks that need current conversation context

## Best Practices

1. **Capture learnings immediately** - Don't wait, just jot it down
2. **Process learnings weekly** - Or before major new work
3. **Be specific in preferences** - Include code examples
4. **Don't over-engineer** - Simple prompt > complex skill for occasional tasks
5. **Version control everything** - Instructions, prompts, skills are code
6. **Refine iteratively** - Better to improve existing than create new
7. **Point, don't copy** - Reference authoritative sources, don't duplicate content
8. **Use subagents for research** - Keep main context clean

## Relationship to Design Spec

The Design Specification is the **source of truth** for architecture.
Instructions/prompts/skills are about **how we implement** that spec:

- Design Spec: "What to build" (ADRs, interfaces, behavior)
- Instructions: "How to write code" (conventions, patterns)
- Prompts: "How to invoke tasks" (reproducible, parameterized)
- Skills: "How to do complex workflows" (multi-step expertise)
