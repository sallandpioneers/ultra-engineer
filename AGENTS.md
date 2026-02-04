# AI Agent Context for Ultra Engineer

This document provides comprehensive context for AI assistants working on the Ultra Engineer codebase.

## Project Overview

Ultra Engineer is a Go-based orchestration system that automates GitHub/Gitea issue implementation using Claude Code CLI. It monitors repositories for issues with a trigger label, guides them through a multi-phase workflow (Q&A, planning, implementation, review), creates pull requests, and optionally monitors CI.

**Key Characteristics:**
- Stateless operation: all state persisted in issue body HTML comments
- Label-driven phase tracking
- Provider abstraction for GitHub/Gitea
- Concurrent issue processing with dependency detection

## Directory Structure

```
.
├── cmd/ultra-engineer/          # CLI entry points
│   ├── main.go                  # Root command, global flags
│   ├── daemon.go                # Continuous polling daemon
│   ├── run.go                   # Single issue processing
│   ├── status.go                # Status display
│   └── abort.go                 # Abort processing
│
├── internal/
│   ├── config/                  # YAML configuration loading
│   ├── state/                   # Workflow state serialization
│   ├── providers/               # Git provider implementations
│   │   ├── provider.go          # Interface definitions
│   │   ├── github.go            # GitHub (via gh CLI)
│   │   ├── gitea.go             # Gitea (HTTP API)
│   │   └── mock.go              # Testing mock
│   ├── orchestrator/            # Core orchestration
│   │   ├── orchestrator.go      # Main orchestrator
│   │   ├── dependency.go        # Dependency detection
│   │   ├── concurrent.go        # Concurrent processing
│   │   └── polling.go           # Issue polling
│   ├── workflow/                # Phase implementations
│   │   ├── qa.go                # Q&A phase
│   │   ├── planning.go          # Planning phase
│   │   ├── implementation.go    # Code implementation
│   │   ├── review.go            # Review cycles
│   │   ├── pr.go                # PR management
│   │   └── ci.go                # CI monitoring
│   ├── claude/                  # Claude CLI integration
│   │   ├── claude.go            # Client
│   │   └── prompts.go           # System prompts
│   ├── retry/                   # Retry with exponential backoff
│   ├── progress/                # Progress reporting
│   ├── sandbox/                 # Working directory isolation
│   └── greeting/                # Initial Q&A generation
│
├── docs/                        # Documentation
├── config.example.yaml          # Example configuration
└── Makefile                     # Build targets
```

## Key Concepts

### Workflow Phases

Issues progress through 8 phases tracked via labels:

| Phase | Label | Description |
|-------|-------|-------------|
| `new` | (none) | Initial state |
| `questions` | `phase:questions` | Clarifying Q&A |
| `planning` | `phase:planning` | Implementation planning |
| `approval` | `phase:approval` | Awaiting plan approval |
| `implementing` | `phase:implementing` | Writing code |
| `review` | `phase:review` | Review cycles |
| `completed` | `phase:completed` | Successfully done |
| `failed` | `phase:failed` | Failed or aborted |

### State Persistence

State is stored as JSON in HTML comments within the issue body:

```html
<!-- ultra-engineer-state
{"session_id":"abc123","current_phase":"implementing",...}
-->
```

Key state fields: `SessionID`, `CurrentPhase`, `QAHistory`, `QARound`, `PlanVersion`, `ReviewIteration`, `PRNumber`, `BranchName`, `DependsOn`, `BlockedBy`, `CIFixAttempts`, `LastCIStatus`.

### Provider Interface

The `Provider` interface (18 methods) abstracts Git operations:
- **Issue ops**: GetIssue, ListIssuesWithLabel, GetComments, CreateComment, UpdateComment, UpdateIssueBody, ReactToComment
- **Label ops**: AddLabel, RemoveLabel
- **PR ops**: CreatePR, GetPR, GetPRComments, GetPRReviewComments, MergePR, IsMergeable
- **Repo ops**: Clone, GetDefaultBranch
- **Info**: Name

Optional `CIProvider` interface adds: GetCIStatus, GetCILogs.

### Dependency Detection

Issues can declare dependencies using patterns:
- `depends on #123`
- `after #123`
- `requires #123`
- `blocked by #123`
- `waiting for #123` / `waiting on #123`

Override with `no-dependencies` label or `/no-deps` in issue body.

## Common Tasks

### Build and Test

```bash
make build          # Build binary
make test           # Run tests
make lint           # Run linter
make all            # Build + test + lint
```

### Run Locally

```bash
# Copy and configure
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# Run daemon
./ultra-engineer daemon --repo owner/repo

# Process single issue
./ultra-engineer run --repo owner/repo --issue 123
```

### Adding a New Provider

1. Create `internal/providers/newprovider.go`
2. Implement the `Provider` interface (18 methods)
3. Optionally implement `CIProvider` for CI support
4. Add constructor and update `createProvider()` in `cmd/ultra-engineer/main.go`

### Adding a New Workflow Phase

1. Create phase handler in `internal/workflow/`
2. Add phase constant to `internal/state/state.go`
3. Update orchestrator to handle the new phase
4. Document the phase label in README

## Code Conventions

- **Error handling**: Return errors, don't panic. Use `fmt.Errorf` with `%w` for wrapping.
- **Context**: Pass `context.Context` as first parameter for cancellation support.
- **Logging**: Use the configured logger, not `fmt.Print`.
- **Testing**: Place tests in `*_test.go` files alongside implementation.
- **Comments**: Document exported functions and types.

## Configuration Reference

Key configuration sections (see `internal/config/config.go`):

| Section | Key Settings |
|---------|-------------|
| Core | `provider`, `poll_interval`, `trigger_label`, `log_file` |
| Provider | `gitea.url/token`, `github.token`, `gitlab.url/token` |
| Claude | `command`, `timeout`, `review_cycles` |
| Retry | `max_attempts`, `backoff_base`, `rate_limit_retry` |
| Defaults | `base_branch`, `auto_merge` |
| Concurrency | `max_per_repo`, `max_total`, `dependency_detection` |
| Progress | `enabled`, `debounce_interval` |
| CI | `poll_interval`, `timeout`, `max_fix_attempts`, `wait_for_ci` |

Environment variables can be referenced as `${VAR_NAME}` in YAML.

## Architecture Decisions

See `docs/adr/` for Architecture Decision Records explaining key design choices:
- State machine architecture
- Provider abstraction
- State persistence in HTML comments
- Claude CLI integration
- Concurrent processing model
- CI monitoring strategy
