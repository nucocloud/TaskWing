# Repository Guidelines

## Project Structure & Module Organization

- `main.go` is the entrypoint; CLI commands live in `cmd/` (Cobra-based).
- Core implementation is under `internal/` (for example `internal/app`, `internal/mcp`, `internal/memory`, `internal/planner`).
- Unit tests are colocated with code as `*_test.go`; integration tests are in `tests/integration/`.
- Documentation is in `docs/`; automation scripts are in `scripts/`; CI pipelines are in `.github/workflows/`.
- Generated/local artifacts such as `.taskwing/`, `test-results/`, and the `taskwing` binary are gitignored.

## Build, Test, and Development Commands

- `make dev-setup`: prepares local tooling, runs `go mod tidy`, and generates code.
- `make build`: builds the local CLI binary (`./taskwing`).
- `make test`: runs unit, integration, and MCP test targets.
- `make test-quick`: fast local checks during iteration.
- `make lint`: runs formatting and static analysis (`go fmt`, `go vet`, `staticcheck`, optional `golangci-lint`).
- `go test ./...`: baseline CI-style test run.


## Coding Style & Naming Conventions

- Target Go `1.24.x` (see `go.mod` and CI workflow).
- Follow standard Go style: gofmt-managed formatting (tabs), idiomatic naming, lowercase package names.
- Keep CLI wiring in `cmd/` and business logic in `internal/...`.
- Use descriptive lowercase file names; tests must use `_test.go`.
- Do not commit local secrets or machine-specific files (`.env`, local binaries, temp outputs).

## Testing Guidelines

- Prefer table-driven tests for logic-heavy code and use `t.Run(...)` for subcases.
- Name tests with `TestXxx` and keep assertions focused on observable behavior.
- Run `make test-quick` before small commits; run `make test` before opening a PR.
- Add/update integration tests in `tests/integration/` for end-to-end CLI/MCP behavior.
- Use `make coverage` to inspect coverage and avoid reducing coverage in touched packages.

## Commit & Pull Request Guidelines

- Use Conventional Commit style: `feat(scope): ...`, `fix: ...`, `test: ...`, `chore: ...`.
- Keep commit subjects concise, imperative, and focused on what changed.
- PRs should include: purpose, linked issue (if applicable), test evidence, and docs updates for user-facing changes.
- Ensure CI passes (`lint`, `test`, docs consistency) before requesting review.
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
