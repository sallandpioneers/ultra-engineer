# ADR-0001: State Machine Architecture

## Status

Accepted

## Context

Ultra Engineer needs to manage complex, multi-phase workflows for implementing GitHub/Gitea issues. The workflow involves several distinct phases:

1. Initial Q&A to gather requirements
2. Planning the implementation
3. User approval of the plan
4. Code implementation
5. Review cycles
6. Completion or failure

Each phase may require user interaction (answering questions, approving plans), and the system must be able to pause and resume at any point. The system also needs to be:

- **Resumable**: Processing can be interrupted and resumed later
- **Observable**: Users should be able to see the current state
- **Recoverable**: Failures should be handled gracefully
- **Stateless**: The orchestrator should not require persistent local storage

## Decision

We will implement a finite state machine with label-driven phase tracking.

**Phase States**: Eight distinct phases represented as string constants:
- `new` - Initial state
- `questions` - Clarifying Q&A
- `planning` - Implementation planning
- `approval` - Awaiting user approval
- `implementing` - Active code writing
- `review` - Review cycles
- `completed` - Successfully finished
- `failed` - Failed or aborted

**Label-Driven Tracking**: Each phase (except `new`) corresponds to a label (e.g., `phase:questions`, `phase:implementing`). The orchestrator:
1. Reads current phase from issue labels
2. Processes the appropriate phase handler
3. Updates labels when transitioning to a new phase

**State Transitions**: Transitions are explicit and controlled by the orchestrator. Invalid transitions are prevented by the state machine logic.

## Consequences

### Positive

- **Clear visibility**: Users can see workflow progress via labels in the issue list
- **Resumable**: Phase can be determined from labels at any time
- **Simple**: Each phase has a single responsibility
- **Extensible**: New phases can be added by defining a new state and transition rules

### Negative

- **Label pollution**: Issues accumulate phase labels (mitigated by removing old phase labels on transition)
- **Limited parallelism**: Only one phase active at a time per issue

### Neutral

- Phase handlers are separate files in `internal/workflow/`
- The orchestrator is responsible for all state transitions

## Alternatives Considered

### Event-driven architecture

Using an event queue to drive transitions.

Rejected because:
- Added complexity for our use case
- Would require additional infrastructure (message queue)
- Harder to observe current state

### Database-backed state

Storing state in a database instead of labels/comments.

Rejected because:
- Requires external database dependency
- Users can't easily inspect state
- Adds operational complexity

### Single "processing" label with comment-based state only

Using just one label and relying entirely on comments for state.

Rejected because:
- Less visible in issue lists
- Harder to filter issues by phase
- Less intuitive for users
