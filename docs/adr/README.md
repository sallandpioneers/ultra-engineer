# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for Ultra Engineer.

ADRs document significant architectural decisions made during the project's development, including the context, decision, and consequences of each choice.

## Index

| ADR | Title | Status |
|-----|-------|--------|
| [0001](0001-state-machine-architecture.md) | State Machine Architecture | Accepted |
| [0002](0002-provider-abstraction.md) | Provider Abstraction | Accepted |
| [0003](0003-state-persistence.md) | State Persistence in HTML Comments | Accepted |
| [0004](0004-claude-cli-integration.md) | Claude CLI Integration | Accepted |
| [0005](0005-concurrent-processing-model.md) | Concurrent Processing Model | Accepted |
| [0006](0006-ci-monitoring-strategy.md) | CI Monitoring Strategy | Accepted |

## ADR Template

When creating a new ADR, use the following template:

```markdown
# ADR-NNNN: Title

## Status

[Proposed | Accepted | Deprecated | Superseded by ADR-XXXX]

## Context

[Describe the forces at play, including technological, political, social, and project-specific constraints. This section should describe the problem that required a decision.]

## Decision

[Describe the decision that was made. Use active voice: "We will..."]

## Consequences

### Positive

- [List positive consequences]

### Negative

- [List negative consequences]

### Neutral

- [List neutral observations]

## Alternatives Considered

[Describe alternatives that were considered and why they were rejected.]
```

## Guidelines

1. **One decision per ADR**: Keep each ADR focused on a single architectural decision.
2. **Immutable once accepted**: Don't modify accepted ADRs. Instead, create a new ADR that supersedes the old one.
3. **Focus on "why"**: The most valuable part of an ADR is understanding why a decision was made.
4. **Include alternatives**: Document what was considered and rejected to prevent revisiting the same discussions.
