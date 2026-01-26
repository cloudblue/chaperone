# Chaperone - AI Agent Entry Point

> **For AI Agents**: Start here to understand this project and how to work with it.

## Quick Context

**Chaperone** is a high-performance Go egress proxy that injects credentials into outgoing API requests.
It uses a plugin architecture (static recompilation) for Distributor customization.

## Essential Files to Read

Before starting any work, read these files in order:

1. **Architecture & Spec**: `docs/DESIGN-SPECIFICATION.md` - Source of truth for what to build
2. **Current Phase**: `docs/ROADMAP.md` - We're in **Phase 1 (PoC)**
3. **Coding Conventions**: `.github/copilot-instructions.md` - How to write code
4. **Workflow System**: `.github/META-WORKFLOW.md` - How the self-improving workflow works

## Project Structure

```
chaperone/
├── docs/                    # 📚 Authoritative documentation
│   ├── DESIGN-SPECIFICATION.md  # Architecture, ADRs, interfaces
│   └── ROADMAP.md               # Phased delivery plan
│
├── sdk/                     # 📦 Plugin SDK (separate Go module)
│   ├── go.mod               # github.com/cloudblue/chaperone/sdk
│   ├── plugin.go            # Plugin, CredentialProvider, etc.
│   └── compliance/          # Contract test kit
│
├── plugins/reference/       # 🔌 Default plugin implementation
│
├── internal/                # 🔒 Private implementation (proxy core)
│
├── cmd/chaperone/           # 🚀 Main binary entry point
│
└── .github/                 # 🤖 AI Workflow System
    ├── copilot-instructions.md   # Coding conventions
    ├── META-WORKFLOW.md          # Workflow documentation
    ├── instructions/             # Modular coding rules
    ├── prompts/                  # Reproducible task templates
    ├── learnings/                # Captured observations
    └── skills/                   # Complex workflows
```

## Current Phase

**Read `docs/ROADMAP.md` for current phase details and task status.**

The roadmap defines 4 phases: PoC → MVP → GA → Future.
Work should fit within the current phase. Use `check-phase-scope` prompt if unsure.

## Available Prompts

**Prompts are the primary workflow entry points.** When asked to perform an action, always check if a dedicated prompt exists first.

All prompts are invoked with `/prompt-name` in Copilot Chat.

| Prompt | Purpose |
|--------|---------|
| `/bootstrap` | Initialize session with full context |
| `/check-phase-scope` | Verify work fits current phase |
| `/planner` | Generate tasks for a phase from ROADMAP |
| `/implement-task` | Implement a specific task file |
| `/code-quality-review` | Independent review of staged changes before merge |
| `/fix-review-issues` | Fix issues identified by code-quality-review |
| `/capture-learning` | Quick capture an observation |
| `/process-learnings` | Convert learnings to workflow updates |
| `/update-go-instructions` | Update Go coding conventions |
| `/refine-prompt` | Improve a prompt that's not working well |
| `/create-skill` | Create a new multi-step workflow skill |

## Key Conventions

- **Language**: Go 1.25+
- **Error Handling**: Always wrap with context (`fmt.Errorf("...: %w", err)`)
- **Logging**: `log/slog` (structured JSON)
- **Testing**: TDD, table-driven tests, compliance suite for plugins
- **Security**: Never log credentials, use memguard for secrets
- **Commits**: Conventional commits (`feat:`, `fix:`, `security:`, etc.)

**Full conventions**: See `.github/copilot-instructions.md`

## How to Bootstrap a Session

When starting a new chat session:

1. **Read this file** (AGENTS.md) - You're doing it now ✓
2. **Check current phase** in `docs/ROADMAP.md`
3. **Review recent learnings** in `.github/learnings/` (if any unprocessed)
4. **Understand the task** in context of Design Spec

Or simply run:
```
/bootstrap
```

### Common Actions → Prompts

| When asked to... | Use this prompt |
|------------------|----------------|
| Implement a task | `/implement-task` |
| Plan work for a phase | `/planner` |
| Capture an observation | `/capture-learning` |
| Check if work fits phase | `/check-phase-scope` |

## Architecture Decisions (ADRs)

**See `docs/DESIGN-SPECIFICATION.md` Section 4** for all ADRs:
- ADR-001: Static Recompilation for plugins
- ADR-002: Go as language
- ADR-003: Hybrid caching (Fast/Slow path)
- ADR-004: Split modules (SDK vs Core)
- ADR-005: Configurable naming

## Security Reminders

**See `.github/instructions/go-security.instructions.md`** for full security rules.

Key points:
- Never log credentials
- TLS 1.3 minimum
- Validate all inputs against allow-list
- Strip credentials from responses
