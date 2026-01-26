# Documentation

This directory contains the authoritative project documentation.

## Contents

| Document | Purpose |
|----------|---------|
| [DESIGN-SPECIFICATION.md](DESIGN-SPECIFICATION.md) | **Source of Truth** - Complete technical specification, ADRs, interfaces |
| [ROADMAP.md](ROADMAP.md) | Phased delivery plan (PoC → MVP → GA → Future) |

## How These Documents Are Used

### Design Specification
- Referenced by prompts and skills for implementation guidance
- Contains Architecture Decision Records (ADRs)
- Defines all interfaces, protocols, and behaviors
- **Do not modify without team consensus**

### Roadmap
- Defines scope for each development phase
- Current focus: **Phase 1 (PoC)**
- Used by `check-phase-scope` prompt to validate work fits current phase

## Relationship to Workflow

```
docs/DESIGN-SPECIFICATION.md     →  "What to build" (architecture)
docs/ROADMAP.md                  →  "When to build" (phases)
.github/copilot-instructions.md  →  "How to build" (conventions)
.github/prompts/                 →  "How to invoke" (tasks)
.github/skills/                  →  "Complex workflows" (expertise)
```

## Modification Policy

| Document | Who Can Modify | Process |
|----------|----------------|---------|
| DESIGN-SPECIFICATION.md | Tech Lead / Architect | ADR process, PR review |
| ROADMAP.md | Product/Tech Lead | Phase completion review |
