# TaskWing Context for Gemini

## Project Overview

**TaskWing** is a local-first AI knowledge layer for development. It extracts architectural decisions, patterns, and constraints from your codebase into local SQLite and makes them queryable by AI assistants (like Gemini, Claude, Cursor) via the Model Context Protocol (MCP).

**Core Value Proposition:**

- **Auto-extraction:** Uses LLM inference to extract architecture from code.
- **Semantic Search:** Query decisions and trade-offs.
- **MCP Integration:** Exposes knowledge to AI agents.

## Tech Stack

- **Language:** Go 1.24+
- **CLI Framework:** Cobra
- **Database:** SQLite (modernc.org/sqlite) - _Single source of truth_
- **LLM Orchestration:** CloudWeGo Eino (OpenAI, Anthropic, Gemini, Bedrock, Ollama support)
- **Frontend (Dashboard):**
  - React 19
  - Vite 7
  - Tailwind CSS 4
  - Shadcn/UI
  - Bun (likely runtime/package manager)

## Architecture

The system is composed of a CLI tool with an embedded MCP server and a web dashboard.

### Core Layers (`internal/`)

- **Memory (`internal/memory`):** Repository pattern. Encapsulates SQLite (Source of Truth) and Markdown (Snapshot).
- **Bootstrap (`internal/bootstrap`):** Analyzes codebases.
  - `scanner.go`: Heuristic analysis (fast, basic).
  - `llm_analyzer.go`: Deep analysis using LLMs.
- **Knowledge (`internal/knowledge`):** `KnowledgeService` centralizes intelligence (RAG, Embeddings, Search).
- **LLM (`internal/llm`):** Interface for AI providers via Eino (Factory pattern).

### Storage Model (`.taskwing/memory/`)

1.  **`memory.db`**: SQLite database. **The canonical source of truth.**
2.  **`index.json`**: Cached index for fast retrieval.
3.  **`features/*.md`**: Human-readable snapshots (generated via Repository). Do not edit manually.

### Directory Structure

```
/
├── cmd/                  # CLI entry points (root, bootstrap, mcp_server, etc.)
├── internal/             # Private application code
│   ├── agents/           # Specialized agents (code, doc, git_deps)
│   ├── bootstrap/        # Codebase analysis logic
│   ├── knowledge/        # Vector search & classification
│   ├── llm/              # LLM client factories
│   ├── memory/           # SQLite storage implementation
│   └── ui/               # TUI components (Bubble Tea)
├── dashboard/            # React/Vite web frontend
├── docs/                 # Documentation (MCP, Roadmap, etc.)
└── Makefile              # Build & Test automation
```

## Key Commands

### Backend / CLI

| Command          | Description                            |
| :--------------- | :------------------------------------- |
| `make build`     | Build the `taskwing` binary            |
| `make test`      | Run all tests (Unit, Integration, MCP) |
| `make test-unit` | Run only unit tests                    |
| `make test-mcp`  | Run MCP protocol tests                 |
| `make lint`      | Run formatters and `golangci-lint`     |
| `make dev-setup` | Install dev dependencies               |

### CLI Commands

| Command              | Description                               |
| :------------------- | :---------------------------------------- |
| `taskwing bootstrap` | Initialize project memory                 |
| `taskwing plan`      | Manage development plans                  |
| `taskwing task`      | Manage execution tasks                    |
| `taskwing start`     | Start API/watch/dashboard services        |

### Frontend (`dashboard/`)

| Command                   | Description                   |
| :------------------------ | :---------------------------- |
| `bun dev` / `npm run dev` | Start Vite development server |
| `bun build`               | Build for production          |

## Development Conventions

1.  **Source of Truth:** Always treat SQLite as the source of truth. The `Repository` handles synchronization.
2.  **Global Flags:** CLI commands should respect global flags like `--json`, `--verbose`, `--preview`.
3.  **Testing:**
    - Use `make test-quick` for rapid iteration.
    - Ensure MCP tests pass if modifying server logic.
    - **New:** Unit tests for `internal/knowledge` and `internal/memory` are required.
4.  **Style:** Follow standard Go idioms. Use `make lint` to enforce.
5.  **LLM Integration:** Use the `internal/llm` client factory to support multiple providers (OpenAI, Anthropic, Gemini, Bedrock, Ollama) agnostic of the specific API.

## MCP Integration

TaskWing exposes an `ask` tool. When working on this feature:

- Ensure responses stay within token budgets (500-1000 tokens).
- Test with `taskwing mcp` locally or use `make test-mcp`.

### Autonomous Task Execution (Hooks)

TaskWing integrates with Claude Code's hook system for autonomous plan execution:

```bash
taskwing hook session-init      # Initialize session tracking (SessionStart hook)
taskwing hook continue-check    # Check if should continue to next task (Stop hook)
taskwing hook session-end       # Cleanup session (SessionEnd hook)
taskwing hook status            # View current session state
```

**Circuit breakers** prevent runaway execution:

- `--max-tasks=5` - Stop after N tasks for human review
- `--max-minutes=30` - Stop after N minutes

Configuration in `.claude/settings.json` enables auto-continuation through plans.

## CLI Binaries

- **`taskwing`**: Production binary installed via Homebrew (`brew install josephgoksu/tap/taskwing`)
- **`./bin/taskwing`**: Local development binary generated by [air](https://github.com/air-verse/air) for hot-reloading

Use `./bin/taskwing` during development, `taskwing` for testing production behavior.

## Release Process

**CRITICAL: Do NOT release without explicit user approval.**

### AI-Assisted Release (Preferred)

When user says "let's release", "create a release", or similar:

1. **Analyze changes** since last tag:

   ```bash
   git log $(git describe --tags --abbrev=0)..HEAD --oneline
   ```

2. **Generate release notes** summarizing:
   - New features (feat:)
   - Bug fixes (fix:)
   - Breaking changes (if any)

3. **Suggest version bump** based on changes:
   - PATCH: bug fixes, refactors, internal improvements
   - MINOR: new user-facing features
   - MAJOR: breaking changes

4. **Get user approval** before proceeding

5. **Execute release**:

   ```bash
   # Create annotated tag with release notes (no source file changes needed)
   git tag -a vX.Y.Z -m "Release notes here..."
   # Push tag to trigger CI/CD
   git push origin vX.Y.Z
   ```

   Note: Version is injected via ldflags at build time from the git tag.
   No need to edit `cmd/root.go` - GoReleaser handles versioning automatically.

### Manual Release (Standalone)

```bash
make release
```

Interactive script that prompts for version, opens editor for notes, creates tag, and pushes.

### Rules

- Never release without explicit user request
- Never bump version autonomously
- Always show release notes for approval before tagging
- GoReleaser + GitHub Actions handle the rest after tag push
<!-- TASKWING_DOCS_START -->

## TaskWing Integration

This project uses TaskWing for architectural knowledge management. You have access to TaskWing MCP tools.

### TaskWing Workflow Contract v1
1. No implementation before a clarified and approved plan/task checkpoint.
2. No completion claim without fresh verification evidence.
3. No debug fix proposal without root-cause evidence.

### MCP Tools (use directly, no skill needed)
- `ask` -- Search project knowledge before modifying unfamiliar code.
- `remember` -- Persist a decision or pattern for future sessions.
- `code` -- Find symbols, explain call graphs, analyze impact, simplify code.
- `debug` -- Diagnose issues with root-cause analysis.
- `task` with action=current -- Check current task status.

**When to use TaskWing MCP tools:**
- Before modifying unfamiliar code: call `ask` to check for relevant decisions, constraints, and patterns
- Before planning multi-step work: call `plan` with action=clarify to get a structured plan
- When asked about architecture, tech stack, or "why" questions: call `ask` with answer=true
- After making an architectural decision: call `remember` to persist it for future sessions
- To understand a symbol's role and callers: call `code` with action=explain

**Do not** grep or read files to answer architecture questions when TaskWing MCP is available. The knowledge graph has pre-extracted, verified decisions with evidence.

### Slash Commands
- /taskwing:plan - Use when you need to clarify a goal and build an approved execution plan.
- /taskwing:next - Use when you are ready to start the next approved TaskWing task with full context.
- /taskwing:done - Use when implementation is verified and you are ready to complete the current task.
- /taskwing:context - Use when you need the full project knowledge dump for complete architectural context.

### Core Commands

<!-- TASKWING_COMMANDS_START -->
- taskwing bootstrap
- taskwing ask "<query>"
- taskwing task
- taskwing mcp
- taskwing doctor
- taskwing config
- taskwing start
<!-- TASKWING_COMMANDS_END -->

### MCP Tools (Canonical Contract)

<!-- TASKWING_MCP_TOOLS_START -->
| Tool | Description |
|------|-------------|
| ask | Search project knowledge (decisions, patterns, constraints) |
| task | Unified task lifecycle (next, current, start, complete) |
| plan | Plan management (clarify, decompose, expand, generate, finalize, audit) |
| code | Code intelligence (find, search, explain, callers, impact, simplify) |
| debug | Diagnose issues systematically with AI-powered analysis |
| remember | Store knowledge in project memory |
<!-- TASKWING_MCP_TOOLS_END -->

### Autonomous Task Execution (Hooks)

TaskWing integrates with Claude Code's hook system for autonomous plan execution:

~~~bash
taskwing hook session-init      # Initialize session tracking (SessionStart hook)
taskwing hook continue-check    # Check if should continue to next task (Stop hook)
taskwing hook session-end       # Cleanup session (SessionEnd hook)
taskwing hook status            # View current session state
~~~

Circuit breakers prevent runaway execution:
- --max-tasks=5 stops after N tasks for human review.
- --max-minutes=30 stops after N minutes.

Configuration in .claude/settings.json enables auto-continuation through plans.
Hook commands prefer $CLAUDE_PROJECT_DIR/bin/taskwing and fall back to taskwing in PATH.
If Claude Code is already running, use /hooks to review or reload hook changes.

<!-- TASKWING_DOCS_END -->
