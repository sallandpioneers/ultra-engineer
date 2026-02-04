# ADR-0003: State Persistence in HTML Comments

## Status

Accepted

## Context

Ultra Engineer needs to persist workflow state between invocations. The state includes:

- Current phase
- Session ID
- Q&A history and round number
- Plan version
- Review iteration count
- PR number and branch name
- CI fix attempts
- Dependency information
- Error messages

Requirements:
- **No external database**: Avoid operational complexity
- **Portable**: State should be visible and movable with the issue
- **Recoverable**: State should survive orchestrator restarts
- **Inspectable**: Developers should be able to debug by viewing state

## Decision

We will store state as JSON within HTML comments in the issue body.

**Format**:
```html
<!-- ultra-engineer-state
{"session_id":"abc123","current_phase":"implementing",...}
-->
```

**Location**: State is appended to the issue body (not in comments) so it:
- Survives comment deletion
- Is always present when reading the issue
- Can be updated atomically with the body

**Bot Marker**: A separate marker `<!-- ultra-engineer -->` identifies bot-generated comments for filtering.

**State Operations**:
- `Parse(body)` - Extract state from issue body
- `Serialize()` - Convert state to HTML comment
- `UpdateBody(body)` - Replace existing state or append new state
- `ContainsState(body)` - Check if body has state
- `RemoveState(body)` - Strip state from body (for display)

**Encoding**: JSON encoding ensures:
- Human-readable for debugging
- Easy to parse
- Supports complex nested structures (like QAHistory)

## Consequences

### Positive

- **Zero infrastructure**: No database, cache, or external storage needed
- **Self-contained**: All state travels with the issue
- **Debuggable**: State is visible in issue source
- **Atomic updates**: State updates are atomic with body updates
- **Platform agnostic**: Works with any platform that supports HTML comments

### Negative

- **Size limits**: Very large state could hit body size limits (mitigated by keeping state compact)
- **Visible in source**: Users editing the issue body see the state comment
- **Race conditions**: Concurrent updates to the same issue could conflict (mitigated by single-issue-per-orchestrator design)
- **No history**: Only current state is preserved (previous states are overwritten)

### Neutral

- State is invisible when viewing the rendered issue
- State format is versioned implicitly by JSON structure
- Malformed state results in starting fresh (fail-safe)

## Alternatives Considered

### Database storage (SQLite, PostgreSQL)

Store state in a database keyed by issue URL.

Rejected because:
- Requires database setup and maintenance
- State becomes decoupled from issues
- Harder to debug (need database access)
- Adds deployment complexity

### File-based storage

Store state in local JSON files.

Rejected because:
- Not portable (tied to specific machine)
- Lost if orchestrator moves to new machine
- Requires file system access

### GitHub/Gitea project custom fields

Use platform-specific metadata fields.

Rejected because:
- Not available on all platforms
- API complexity varies by platform
- Limited field types and sizes

### Comments instead of body

Store state in a special comment instead of the body.

Rejected because:
- Comments can be deleted
- Harder to find the "current" state comment
- Would need to scan all comments
