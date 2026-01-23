---
mode: agent
description: Verify if proposed work fits within the current development phase
tools: ['read_file']
---

# Check Phase Scope

Verify whether a proposed task or feature fits within the current development phase.

## Context

Chaperone follows a phased roadmap:
- **Phase 1 (PoC)**: Core skeleton, context parsing, plugin mechanism, mTLS, Docker
- **Phase 2 (MVP)**: Config, router, error handling, security, observability, deployment
- **Phase 3 (GA)**: Cert rotation, metrics, caching, performance, profiling
- **Phase 4 (Future)**: Mode B, Helm, OpenTelemetry

Work should generally stay within the current phase to maintain focus.

## Variables

- `{{task}}` - Description of the proposed work
- `{{current_phase}}` - Which phase we're in (default: read from ROADMAP.md)

## Instructions

1. **Read the roadmap** at `docs/ROADMAP.md`

2. **Identify current phase** (look for the phase being actively worked on)

3. **Analyze the proposed task:**
   - Does it match any item in the current phase?
   - Does it require features from a later phase?
   - Is it foundational work that enables current phase items?

4. **Provide verdict:**

### If IN SCOPE:
```
## ✅ In Scope for Phase {{N}}

**Task:** {{task}}
**Matches:** [specific roadmap item]

Proceed with implementation.
```

### If OUT OF SCOPE:
```
## ⚠️ Out of Scope for Phase {{N}}

**Task:** {{task}}
**Belongs to:** Phase {{M}} - {{phase_name}}
**Reason:** {{why it's out of scope}}

**Options:**
1. Defer to Phase {{M}}
2. Implement minimal version for current phase (describe what's minimal)
3. Justify why it's needed now (architectural dependency)

**Recommendation:** [your suggestion]
```

### If FOUNDATIONAL:
```
## 🔧 Foundational Work

**Task:** {{task}}
**Enables:** [which current phase items this supports]

This is preparatory work that supports current phase goals. Proceed.
```

## Examples

**In Scope (Phase 1):**
- "Implement TransactionContext struct" → Matches "Context Parsing"
- "Write httptest for mTLS" → Matches "mTLS Verification"

**Out of Scope (Phase 1):**
- "Add Prometheus metrics endpoint" → Phase 3 item
- "Implement config.yaml loading" → Phase 2 item

**Foundational:**
- "Set up project structure with internal/" → Enables all Phase 1 items
