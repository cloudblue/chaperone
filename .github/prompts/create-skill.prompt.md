---
agent: "agent"
description: Create a new Agent Skill for the Chaperone project
tools: ['read/readFile', 'edit/createFile', 'search']
---

# Create Chaperone Skill

Create a new Agent Skill specific to the Chaperone project.

## Context

Skills in `.github/skills/` encode domain expertise and multi-step workflows.
Chaperone skills should align with the project's architecture (Design Spec) and conventions.

## Variables

- `{{skill_name}}` - Kebab-case name (e.g., `implement-sdk-interface`, `add-security-control`)
- `{{description}}` - What does this skill help accomplish?
- `{{trigger}}` - When should this skill be activated? (keywords, scenarios)
- `{{steps}}` - High-level steps the skill should guide through

## Instructions

1. **Validate the skill is needed:**
   - Is this a multi-step workflow? (If single-step, a prompt might suffice)
   - Will this be reused? (If one-time, just do it directly)
   - Does it require domain expertise? (If generic, check if skill exists globally)

2. **Create the skill directory and SKILL.md:**
   
   Location: `.github/skills/{{skill_name}}/SKILL.md`

3. **Use this Chaperone-specific template:**

```markdown
---
name: {{skill_name}}
description: {{description}}
metadata:
  project: chaperone
  design-spec-sections: [list relevant sections from docs/explanation/DESIGN-SPECIFICATION.md]
---

# {{Skill Title}}

## Overview
{{What this skill accomplishes}}

## When to Use This Skill
{{trigger}}

## Prerequisites
- [ ] Design Specification available
- [ ] SDK module exists at `sdk/`
- [ ] [Other prerequisites]

## Workflow

### Step 1: [Name]
[Instructions]

### Step 2: [Name]
[Instructions]

...

## Chaperone-Specific Rules

- All code must follow `.github/copilot-instructions.md`
- Reference Design Spec sections: [list]
- Security requirements from Section 5.3 apply
- TDD: Write tests first

## Validation Checklist

- [ ] Code compiles (`go build ./...`)
- [ ] Tests pass (`make test`)
- [ ] Lint passes (`make lint`)
- [ ] Follows Design Spec
- [ ] Security controls implemented

## Related Resources

- [DESIGN-SPECIFICATION.md](../../docs/explanation/DESIGN-SPECIFICATION.md)
- [ROADMAP.md](../../docs/ROADMAP.md)
- [SDK README](../../sdk/README.md)
```

4. **Add supporting files if needed:**
   - `references/` - Additional documentation
   - `templates/` - Code templates
   - `scripts/` - Helper scripts

## Output

Confirm skill was created and provide usage example:
```
Created: .github/skills/{{skill_name}}/SKILL.md

To use: "Use the {{skill_name}} skill to [task]"
```
