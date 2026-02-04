# CLI Commands Reference

Ultra Engineer provides a command-line interface for managing automated issue implementation.

## Global Flags

These flags are available for all commands:

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--config` | `-c` | string | `config.yaml` | Path to configuration file |
| `--verbose` | `-v` | bool | `false` | Enable verbose logging |
| `--log-file` | | string | (none) | Path to log file |

## Commands

### daemon

Continuously polls for issues with the trigger label and processes them automatically.

```bash
ultra-engineer daemon --repo owner/repo [--repo owner/repo2]
```

**Flags:**

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--repo` | string | Yes | Repository to monitor (owner/repo format). Can be specified multiple times for multiple repositories. |

**Examples:**

```bash
# Monitor a single repository
ultra-engineer daemon --repo myorg/myrepo

# Monitor multiple repositories
ultra-engineer daemon --repo myorg/repo1 --repo myorg/repo2

# With verbose logging
ultra-engineer daemon -v --repo myorg/myrepo

# Custom config file
ultra-engineer daemon -c /etc/ultra-engineer/config.yaml --repo myorg/myrepo
```

**Behavior:**
- Polls at the interval specified in configuration
- Processes issues concurrently based on concurrency settings
- Handles graceful shutdown on SIGINT/SIGTERM
- Logs progress to stdout and optional log file

### run

Process a single issue through the workflow state machine.

```bash
ultra-engineer run --repo owner/repo --issue 123
```

**Flags:**

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--repo` | string | Yes | Repository (owner/repo format) |
| `--issue` | int | Yes | Issue number to process |

**Examples:**

```bash
# Process issue #42
ultra-engineer run --repo myorg/myrepo --issue 42

# With verbose logging
ultra-engineer run -v --repo myorg/myrepo --issue 42
```

**Behavior:**
- Reads current state from issue
- Executes appropriate phase handler
- If user input is required (e.g., answering questions, approving plan), posts request and exits
- Run again after providing input to continue

**Use Cases:**
- Manual triggering for testing
- Debugging specific issues
- One-off processing without daemon

### status

Show the current status of issues being processed.

```bash
ultra-engineer status --repo owner/repo [--issue 123]
```

**Flags:**

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--repo` | string | Yes | Repository (owner/repo format) |
| `--issue` | int | No | Specific issue number (optional) |

**Examples:**

```bash
# List all issues with trigger label
ultra-engineer status --repo myorg/myrepo

# Show detailed status for specific issue
ultra-engineer status --repo myorg/myrepo --issue 42
```

**Output (without --issue):**

Lists all issues with the trigger label in table format:

```
ISSUE | TITLE                    | PHASE        | AUTHOR
------|--------------------------|--------------|--------
42    | Add user authentication  | implementing | alice
43    | Fix login bug            | review       | bob
44    | Update documentation     | questions    | carol
```

**Output (with --issue):**

Detailed status for the specified issue:

```
Issue #42: Add user authentication
  Author: alice
  State: open
  Phase: implementing
  Session ID: abc123def456

  Q&A Rounds: 2
  Plan Version: 1
  Review Iteration: 0

  PR: #87
  Branch: feat/user-auth-42

  Last Updated: 2025-01-15 10:30:00 UTC
```

### abort

Abort processing of an issue and mark it as failed.

```bash
ultra-engineer abort --repo owner/repo --issue 123
```

**Flags:**

| Flag | Type | Required | Description |
|------|------|----------|-------------|
| `--repo` | string | Yes | Repository (owner/repo format) |
| `--issue` | int | Yes | Issue number to abort |

**Examples:**

```bash
# Abort processing of issue #42
ultra-engineer abort --repo myorg/myrepo --issue 42
```

**Behavior:**
1. Adds `abort` label to the issue
2. Posts comment: "**Processing aborted** via CLI command."
3. Adds `phase:failed` label
4. Removes trigger label (best-effort)

**Use Cases:**
- Stop runaway processing
- Cancel work on deprioritized issues
- Reset for a fresh start

### version

Print version information.

```bash
ultra-engineer version
```

**Output:**

```
ultra-engineer v0.1.0
```

## Exit Codes

| Code | Description |
|------|-------------|
| 0 | Success |
| 1 | General error |
| 2 | Configuration error |
| 3 | Provider error |

## Logging

Logs are written to:
1. Standard output (always)
2. Log file (if `--log-file` is specified or configured)

Log format:
```
[ultra-engineer] 2025/01/15 10:30:00 Processing issue #42 in myorg/myrepo
```

With `--verbose`:
```
[ultra-engineer] main.go:123: 2025/01/15 10:30:00 Processing issue #42 in myorg/myrepo
```

## Signal Handling

The `daemon` command handles these signals:

| Signal | Behavior |
|--------|----------|
| SIGINT (Ctrl+C) | Graceful shutdown - finishes current operations |
| SIGTERM | Graceful shutdown - finishes current operations |

## Examples

### Development Workflow

```bash
# Start with verbose logging
ultra-engineer daemon -v --repo myuser/myproject

# In another terminal, check status
ultra-engineer status --repo myuser/myproject

# Abort a problematic issue
ultra-engineer abort --repo myuser/myproject --issue 42
```

### Production Deployment

```bash
# Run with custom config and log file
ultra-engineer daemon \
  -c /etc/ultra-engineer/config.yaml \
  --log-file /var/log/ultra-engineer/daemon.log \
  --repo company/repo1 \
  --repo company/repo2
```

### Testing a Single Issue

```bash
# Process just one issue (useful for testing)
ultra-engineer run -v --repo myuser/myproject --issue 123

# Check its status
ultra-engineer status --repo myuser/myproject --issue 123
```
