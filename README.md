# Ultra Engineer

Orchestrates Claude Code CLI instances to automatically implement issues from GitHub and Gitea.

## Features

- **Q&A Phase**: Ask clarifying questions about issues
- **Planning**: Create and review implementation plans
- **Implementation**: Write code with automated review cycles
- **PR Management**: Create PRs, fix CI failures, and auto-merge

## Build

```bash
go build -o ultra-engineer ./cmd/ultra-engineer
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and configure:

```yaml
provider: gitea  # or github

poll_interval: 60s
trigger_label: ai-implement

gitea:
  url: https://gitea.example.com
  token: ${GITEA_TOKEN}

github:
  token: ${GITHUB_TOKEN}

claude:
  command: claude
  timeout: 30m
  review_cycles: 5

retry:
  max_attempts: 3
  backoff_base: 10s
  rate_limit_retry: 5m

defaults:
  base_branch: main
  auto_merge: true
```

### Default Values

| Setting | Default |
|---------|---------|
| provider | gitea |
| poll_interval | 60s |
| trigger_label | ai-implement |
| claude.command | claude |
| claude.timeout | 30m |
| claude.review_cycles | 5 |
| retry.max_attempts | 3 |
| retry.backoff_base | 10s |
| retry.rate_limit_retry | 5m |
| defaults.base_branch | main |
| defaults.auto_merge | true |

## Commands

### daemon

Continuously polls for issues with the trigger label and processes them.

```bash
ultra-engineer daemon --repo owner/repo
```

### run

Processes a single issue through the workflow state machine. If user input is required (e.g., answering questions, approving plan), it posts the request and exits.

```bash
ultra-engineer run --repo owner/repo --issue 123
```

### status

Shows status of issues being processed.

```bash
ultra-engineer status --repo owner/repo
ultra-engineer status --repo owner/repo --issue 123
```

### abort

Aborts processing of an issue and marks it as failed.

```bash
ultra-engineer abort --repo owner/repo --issue 123
```

### version

Prints version information.

```bash
ultra-engineer version
```

## Global Flags

- `-c, --config` - Path to config file (default: `config.yaml`)
- `-v, --verbose` - Enable verbose logging

## Workflow Phases

Issues progress through these phases, tracked via labels:

| Phase | Label | Description |
|-------|-------|-------------|
| new | (none) | Initial state, no processing started |
| questions | `phase:questions` | Clarifying Q&A phase |
| planning | `phase:planning` | Implementation planning |
| approval | `phase:approval` | Awaiting user approval of plan |
| implementing | `phase:implementing` | Active code implementation |
| review | `phase:review` | Review cycles and refinement |
| completed | `phase:completed` | Successfully completed |
| failed | `phase:failed` | Failed or aborted |

## Supported Providers

- **gitea** - Full support via Gitea API
- **github** - Full support via `gh` CLI
