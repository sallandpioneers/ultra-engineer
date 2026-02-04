# ADR-0005: Concurrent Processing Model

## Status

Accepted

## Context

Ultra Engineer may need to process multiple issues simultaneously:

- Multiple issues in a single repository
- Issues across multiple repositories
- Issues with dependencies on each other

Challenges:
- **Resource constraints**: Claude invocations are expensive (time, API costs)
- **Dependencies**: Some issues depend on others being completed first
- **Conflicts**: Concurrent changes to the same files can conflict
- **Rate limits**: Git providers have API rate limits

## Decision

We will implement a WorkerPool with configurable concurrency and dependency detection.

**Concurrency Configuration**:
- `max_per_repo`: Maximum concurrent issues per repository (default: 5)
- `max_total`: Maximum total concurrent issues (default: 5)
- `dependency_detection`: Mode for detecting dependencies ("auto", "manual", "disabled")

**Dependency Detection Modes**:
- `auto`: Parse issue text for dependency patterns
- `manual`: Only respect explicit dependency labels
- `disabled`: Ignore all dependencies

**Dependency Patterns**: Automatically detected phrases (case-insensitive):
- "depends on #123"
- "after #123"
- "requires #123"
- "blocked by #123"
- "waiting for #123" / "waiting on #123"

**Manual Overrides**:
- `no-dependencies` label: Skip dependency detection for an issue
- `/no-deps` in issue body: Same effect

**Cycle Detection**: Before processing, detect cycles in dependency graph using DFS. If a cycle is found, mark involved issues as failed with a descriptive error.

**Blocking Behavior**:
- Issues with unresolved dependencies are skipped during polling
- When a dependency completes, dependent issues become eligible
- State tracks both `DependsOn` (declared) and `BlockedBy` (currently blocking)

**WorkerPool Implementation**:
- Goroutine pool with semaphore for concurrency control
- Per-repo semaphores to enforce `max_per_repo`
- Global semaphore for `max_total`
- Graceful shutdown on context cancellation

## Consequences

### Positive

- **Efficient**: Multiple issues processed in parallel
- **Safe**: Dependencies prevent conflicting changes
- **Configurable**: Concurrency tuned per deployment
- **Automatic**: No manual dependency management needed (in auto mode)
- **Fail-safe**: Cycles detected early before wasted work

### Negative

- **Complexity**: Concurrency adds implementation complexity
- **Potential deadlocks**: Careful implementation needed
- **Resource usage**: High concurrency increases load
- **False positives**: Auto-detection might find spurious dependencies

### Neutral

- Dependency detection is best-effort
- Manual mode gives users full control
- Statistics tracked for monitoring

## Alternatives Considered

### Sequential processing only

Process one issue at a time.

Rejected because:
- Too slow for repositories with many issues
- Underutilizes available resources
- Poor user experience waiting for queue

### Unlimited concurrency

Process all eligible issues simultaneously.

Rejected because:
- Could overwhelm resources
- API rate limits would be hit
- No control over resource usage

### External job queue (Redis, RabbitMQ)

Use a dedicated job queue for work distribution.

Rejected because:
- Adds infrastructure dependency
- Overkill for our use case
- In-process concurrency is sufficient

### Dependency detection via AI

Use Claude to analyze dependencies.

Considered for future enhancement, but rejected for initial version because:
- Expensive (API calls for every issue)
- Adds latency
- Regex patterns work well for common cases
