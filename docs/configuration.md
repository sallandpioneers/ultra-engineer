# Configuration Guide

Ultra Engineer is configured via a YAML file (default: `config.yaml`).

## Quick Start

```yaml
provider: github
trigger_label: ai-implement

github:
  token: ${GITHUB_TOKEN}

claude:
  command: claude
  timeout: 30m
```

## Configuration Reference

### Core Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `provider` | string | `gitea` | Git provider: `gitea`, `github`, or `gitlab` |
| `poll_interval` | duration | `60s` | How often to poll for new issues |
| `trigger_label` | string | `ai-implement` | Label that triggers processing |
| `log_file` | string | (none) | Optional path to log file |

### Provider Configuration

#### Gitea

```yaml
gitea:
  url: https://gitea.example.com
  token: ${GITEA_TOKEN}
```

| Setting | Type | Required | Description |
|---------|------|----------|-------------|
| `url` | string | Yes | Gitea instance URL |
| `token` | string | Yes | API access token |

The Gitea provider uses direct HTTP API calls with a 30-second timeout.

#### GitHub

```yaml
github:
  token: ${GITHUB_TOKEN}
```

| Setting | Type | Required | Description |
|---------|------|----------|-------------|
| `token` | string | No* | GitHub personal access token |

*The GitHub provider uses the `gh` CLI, which can use its own authentication. The token is passed via `GH_TOKEN` environment variable if provided.

#### GitLab

```yaml
gitlab:
  url: https://gitlab.example.com
  token: ${GITLAB_TOKEN}
```

| Setting | Type | Required | Description |
|---------|------|----------|-------------|
| `url` | string | Yes | GitLab instance URL |
| `token` | string | Yes | API access token |

**Note**: GitLab support is config-ready but not yet implemented.

### Claude Settings

```yaml
claude:
  command: claude
  timeout: 30m
  review_cycles: 5
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `command` | string | `claude` | Path to Claude CLI binary |
| `timeout` | duration | `30m` | Timeout per Claude invocation |
| `review_cycles` | int | `5` | Number of review iterations |

### Retry Settings

```yaml
retry:
  max_attempts: 3
  backoff_base: 10s
  rate_limit_retry: 5m
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max_attempts` | int | `3` | Maximum retries for transient errors |
| `backoff_base` | duration | `10s` | Initial backoff duration |
| `rate_limit_retry` | duration | `5m` | Retry interval when rate limited |

### Default Settings

```yaml
defaults:
  base_branch: main
  auto_merge: true
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `base_branch` | string | `main` | Default branch for PRs |
| `auto_merge` | bool | `true` | Auto-merge when provider says mergeable |

### Concurrency Settings

```yaml
concurrency:
  max_per_repo: 5
  max_total: 5
  dependency_detection: auto
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `max_per_repo` | int | `5` | Maximum concurrent issues per repository |
| `max_total` | int | `5` | Maximum total concurrent issues |
| `dependency_detection` | string | `auto` | Dependency detection mode |

**Dependency Detection Modes**:
- `auto`: Parse issue text for dependency patterns
- `manual`: Only respect explicit dependency labels
- `disabled`: Ignore all dependencies

**Dependency Patterns** (case-insensitive):
- `depends on #N`
- `after #N`
- `requires #N`
- `blocked by #N`
- `waiting for #N` / `waiting on #N`

**Manual Overrides**:
- Add `no-dependencies` label to skip detection
- Include `/no-deps` in issue body

### Progress Reporting

```yaml
progress:
  enabled: true
  debounce_interval: 60s
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `enabled` | bool | `true` | Enable progress comments |
| `debounce_interval` | duration | `60s` | Minimum time between updates |

Critical milestones (phase transitions, errors) force immediate updates regardless of debounce.

### CI Monitoring

```yaml
ci:
  poll_interval: 30s
  timeout: 30m
  max_fix_attempts: 3
  wait_for_ci: false
```

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| `poll_interval` | duration | `30s` | How often to poll CI status |
| `timeout` | duration | `30m` | Maximum time to wait for CI |
| `max_fix_attempts` | int | `3` | Maximum attempts to fix CI failures |
| `wait_for_ci` | bool | `false` | Whether to wait for CI (opt-in) |

## Environment Variables

Configuration values can reference environment variables using `${VAR_NAME}` syntax:

```yaml
gitea:
  token: ${GITEA_TOKEN}

github:
  token: ${GITHUB_TOKEN}
```

Variables are expanded at load time. Missing variables result in empty strings.

## Complete Example

```yaml
# Git provider configuration
provider: github
poll_interval: 60s
trigger_label: ai-implement
log_file: /var/log/ultra-engineer.log

# Provider-specific settings
gitea:
  url: https://gitea.example.com
  token: ${GITEA_TOKEN}

github:
  token: ${GITHUB_TOKEN}

gitlab:
  url: https://gitlab.example.com
  token: ${GITLAB_TOKEN}

# Claude Code CLI settings
claude:
  command: /usr/local/bin/claude
  timeout: 30m
  review_cycles: 5

# Retry behavior for transient errors
retry:
  max_attempts: 3
  backoff_base: 10s
  rate_limit_retry: 5m

# Repository defaults
defaults:
  base_branch: main
  auto_merge: true

# Concurrent issue processing
concurrency:
  max_per_repo: 3
  max_total: 10
  dependency_detection: auto

# Progress reporting
progress:
  enabled: true
  debounce_interval: 60s

# CI monitoring (opt-in)
ci:
  poll_interval: 30s
  timeout: 30m
  max_fix_attempts: 3
  wait_for_ci: true
```

## Configuration Loading

Configuration is loaded from:
1. Path specified by `-c` / `--config` flag
2. Default: `config.yaml` in current directory

The configuration is validated at load time. Missing required fields or invalid values result in an error.

## Example Configurations

### Minimal GitHub Setup

```yaml
provider: github
github:
  token: ${GITHUB_TOKEN}
```

### Gitea with CI Monitoring

```yaml
provider: gitea
gitea:
  url: https://git.company.com
  token: ${GITEA_TOKEN}

ci:
  wait_for_ci: true
  max_fix_attempts: 2
```

### High Concurrency Setup

```yaml
provider: github
github:
  token: ${GITHUB_TOKEN}

concurrency:
  max_per_repo: 10
  max_total: 50
  dependency_detection: auto

claude:
  timeout: 45m
```

### Conservative Setup (Low Resource Usage)

```yaml
provider: github
github:
  token: ${GITHUB_TOKEN}

poll_interval: 5m

concurrency:
  max_per_repo: 1
  max_total: 2

claude:
  review_cycles: 3
  timeout: 15m
```
