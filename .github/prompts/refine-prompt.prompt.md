---
mode: agent
description: Improve an existing prompt based on observed results
tools: ['read_file', 'replace_string_in_file', 'semantic_search']
---

# Refine Prompt

Improve an existing prompt file based on observed results or feedback.

## Context

Prompts in `.github/prompts/` define reproducible tasks. When a prompt produces suboptimal results, we refine it.

## Variables

- `{{prompt_file}}` - Which prompt to refine (e.g., `implement-feature.prompt.md`)
- `{{problem}}` - What was wrong with the output?
- `{{desired}}` - What should happen instead?
- `{{example}}` - Specific example of the issue (optional)

## Instructions

1. **Read the current prompt** at `.github/prompts/{{prompt_file}}`

2. **Analyze the problem:**
   - Is the instruction unclear?
   - Missing context?
   - Wrong tools specified?
   - Missing constraints?
   - Output format issues?

3. **Apply refinement** following prompt engineering best practices:
   - Be more specific if output was too vague
   - Add constraints if output violated rules
   - Add examples if the pattern wasn't understood
   - Adjust tools list if wrong tools were used
   - Add "DO NOT" rules for common mistakes

4. **Preserve working parts** - Don't rewrite what works

5. **Add refinement note** at bottom:
   ```markdown
   <!-- Refinement History
   - YYYY-MM-DD: [brief description of change]
   -->
   ```

## Output

Show the diff of changes made and explain the refinement rationale.
