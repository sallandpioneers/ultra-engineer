# ADR-0006: CI Monitoring Strategy

## Status

Accepted

## Context

After Ultra Engineer creates a pull request, CI pipelines typically run to validate the changes. Several scenarios need handling:

1. **CI passes**: PR can be merged
2. **CI fails**: Changes need fixing
3. **CI is slow**: Long-running pipelines
4. **No CI**: Some repositories don't have CI

Considerations:
- Not all users want automated CI handling
- CI systems vary widely (GitHub Actions, Jenkins, GitLab CI, etc.)
- Fixing CI failures requires understanding the failure
- Multiple fix attempts could waste resources

## Decision

We will implement opt-in CI waiting with polling and automated fix attempts.

**Opt-in Design**: CI monitoring is disabled by default (`wait_for_ci: false`). Users must explicitly enable it, acknowledging the implications.

**CI Configuration**:
- `poll_interval`: How often to check CI status (default: 30s)
- `timeout`: Maximum time to wait for CI (default: 30m)
- `max_fix_attempts`: Maximum attempts to fix CI failures (default: 3)
- `wait_for_ci`: Enable/disable CI waiting (default: false)

**CI Status Types**:
- `pending`: CI is still running
- `success`: All checks passed
- `failure`: One or more checks failed
- `unknown`: CI status couldn't be determined

**CIResult Structure**:
- `OverallStatus`: Combined status of all checks
- `Checks`: List of individual check results
  - `ID`: Check run ID (for fetching logs)
  - `Name`: Check name
  - `Status`: Current status
  - `Conclusion`: Final result (success, failure, cancelled)
  - `DetailsURL`: Link to check details
  - `Output`: Summary output

**Fix Attempt Flow**:
1. Detect CI failure
2. Fetch CI logs using `GetCILogs`
3. Pass logs to Claude for analysis
4. Claude attempts to fix the issue
5. Commit and push changes
6. Wait for CI to run again
7. Repeat up to `max_fix_attempts` times

**Provider Integration**: CI support is optional via the `CIProvider` interface:
- `GetCIStatus`: Get current CI status for a PR
- `GetCILogs`: Get logs for a specific check run

Providers that don't implement `CIProvider` simply skip CI waiting.

**State Tracking**:
- `CIFixAttempts`: Number of fix attempts made
- `LastCIStatus`: Last observed CI status
- `CIWaitStartTime`: When CI waiting started (for timeout)

## Consequences

### Positive

- **Opt-in**: Users choose whether to use CI monitoring
- **Automated fixes**: Can resolve common CI failures without human intervention
- **Configurable limits**: Prevent infinite fix loops
- **Provider agnostic**: Works with any provider implementing `CIProvider`
- **Transparent**: CI status visible via logs and comments

### Negative

- **Additional complexity**: CI monitoring adds significant code
- **API usage**: Polling increases API calls
- **Potential loops**: Fix attempts might make things worse
- **Provider variability**: CI integration differs significantly per platform

### Neutral

- GitHub Actions is well-supported via `gh` CLI
- Gitea CI support depends on configured CI system
- Fix attempts use the same Claude invocation as implementation

## Alternatives Considered

### Webhooks for CI status

Receive webhook notifications when CI completes.

Rejected because:
- Requires webhook server infrastructure
- Complicates deployment
- Not all CI systems support webhooks

### Always wait for CI

Make CI waiting mandatory.

Rejected because:
- Many small repositories don't have CI
- Some users want faster feedback
- CI can be very slow

### No automated fixes

Only wait for CI, don't attempt fixes.

Rejected because:
- Common failures (linting, formatting) are easy to fix
- Reduces human intervention needed
- Still useful even without fixes (via opt-out of max_fix_attempts: 0)

### External CI integration service

Use a dedicated CI monitoring service.

Rejected because:
- Adds external dependency
- Additional cost
- Provider APIs already expose CI status
