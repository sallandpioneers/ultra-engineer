# Architecture Overview

Ultra Engineer is an orchestration system that automates GitHub/Gitea issue implementation using Claude Code CLI.

## System Design

```mermaid
graph TB
    subgraph "Ultra Engineer"
        CLI[CLI Commands]
        Orch[Orchestrator]
        WF[Workflow Phases]
        Claude[Claude Client]
        Prov[Provider Interface]
    end

    subgraph "External Systems"
        GH[GitHub]
        GT[Gitea]
        CC[Claude Code CLI]
    end

    CLI --> Orch
    Orch --> WF
    WF --> Claude
    WF --> Prov
    Claude --> CC
    Prov --> GH
    Prov --> GT
```

## Component Overview

### CLI Layer (`cmd/ultra-engineer/`)

Entry points for user interaction:

- **daemon**: Continuous polling mode for automated processing
- **run**: Single issue processing for manual/testing use
- **status**: Display current processing status
- **abort**: Stop processing and mark as failed
- **version**: Show version information

### Orchestrator (`internal/orchestrator/`)

Core coordination logic:

- **orchestrator.go**: Main orchestration loop
- **polling.go**: Issue discovery and polling
- **concurrent.go**: Worker pool for parallel processing
- **dependency.go**: Dependency detection and cycle checking

### Workflow (`internal/workflow/`)

Phase-specific implementations:

- **qa.go**: Q&A phase for gathering requirements
- **planning.go**: Implementation planning
- **implementation.go**: Code writing
- **review.go**: Review cycles
- **pr.go**: Pull request management
- **ci.go**: CI status monitoring

### Providers (`internal/providers/`)

Git platform abstraction:

- **provider.go**: Interface definitions
- **github.go**: GitHub implementation (via `gh` CLI)
- **gitea.go**: Gitea implementation (HTTP API)
- **mock.go**: Testing mock

### State (`internal/state/`)

Workflow state management:

- Phase tracking via labels
- State persistence in HTML comments
- State serialization/deserialization

### Claude (`internal/claude/`)

Claude Code CLI integration:

- CLI invocation with appropriate flags
- Prompt templates for each phase
- Output parsing

## Data Flow

### Issue Processing Flow

```mermaid
sequenceDiagram
    participant U as User
    participant O as Orchestrator
    participant P as Provider
    participant C as Claude
    participant R as Repository

    U->>P: Add trigger label
    O->>P: Poll for labeled issues
    P-->>O: Issue list
    O->>P: Get issue details
    O->>C: Generate questions
    C-->>O: Questions
    O->>P: Post comment
    U->>P: Answer questions
    O->>P: Get comments
    O->>C: Create plan
    C-->>O: Plan
    O->>P: Post plan
    U->>P: Approve plan
    O->>R: Clone repo
    O->>C: Implement code
    C-->>O: Code changes
    O->>P: Create PR
    O->>C: Review code
    C-->>O: Review feedback
    O->>P: Merge PR
```

## State Machine

Issues progress through phases tracked via labels:

```mermaid
stateDiagram-v2
    [*] --> new
    new --> questions: Start processing
    questions --> planning: Questions answered
    planning --> approval: Plan created
    approval --> implementing: Plan approved
    implementing --> review: Code written
    review --> review: Review iteration
    review --> completed: Review passed

    new --> failed: Error
    questions --> failed: Error
    planning --> failed: Error
    approval --> failed: Rejected
    implementing --> failed: Error
    review --> failed: Max iterations
```

## Sandbox Management

Each issue is processed in an isolated working directory:

1. **Clone**: Repository cloned to temporary directory
2. **Branch**: New branch created for changes
3. **Work**: Claude operates within sandbox
4. **Push**: Changes pushed to remote
5. **Cleanup**: Sandbox removed after completion

This isolation prevents:
- Concurrent issues interfering with each other
- Local state pollution
- File conflicts

## Claude CLI Integration

Ultra Engineer invokes Claude Code CLI with specific configurations per phase:

| Phase | Tools Allowed | Purpose |
|-------|---------------|---------|
| Q&A | Read-only | Generate clarifying questions |
| Planning | Read-only | Create implementation plan |
| Implementation | Full access | Write code |
| Review | Full access | Review and refine |

Prompts are tailored for each phase (see `internal/claude/prompts.go`).

## Concurrency Model

```mermaid
graph TB
    subgraph "Worker Pool"
        W1[Worker 1]
        W2[Worker 2]
        W3[Worker N]
    end

    subgraph "Semaphores"
        GS[Global Semaphore<br/>max_total]
        RS1[Repo Semaphore 1<br/>max_per_repo]
        RS2[Repo Semaphore 2<br/>max_per_repo]
    end

    subgraph "Issues"
        I1[Issue 1]
        I2[Issue 2]
        I3[Issue 3]
    end

    I1 --> GS
    I2 --> GS
    I3 --> GS
    GS --> RS1
    GS --> RS2
    RS1 --> W1
    RS1 --> W2
    RS2 --> W3
```

Workers acquire semaphores before processing:
1. Global semaphore (enforces `max_total`)
2. Per-repo semaphore (enforces `max_per_repo`)

## Dependency Handling

Dependencies are detected from issue text and tracked in state:

```mermaid
graph LR
    A[Issue #1] --> B[Issue #2]
    B --> C[Issue #3]
    A --> C
```

- Issues wait for dependencies to complete
- Cycles are detected and fail immediately
- Override with `no-dependencies` label

## Error Handling

Errors are handled at multiple levels:

1. **Transient errors**: Retried with exponential backoff
2. **Rate limits**: Longer retry interval
3. **Permanent errors**: Issue marked as failed
4. **CI failures**: Automated fix attempts (if enabled)

See [ADRs](adr/) for detailed architectural decisions.
