# Ultra Engineer

Orchestrates Claude Code CLI instances to automatically implement issues from GitHub and Gitea.

## Features

- **Q&A Phase**: Ask clarifying questions about issues
- **Planning**: Create and review implementation plans
- **Implementation**: Write code with automated review cycles
- **PR Management**: Create PRs, fix CI failures, and auto-merge
- **Concurrent Processing**: Handle multiple issues with dependency detection
- **CI Integration**: Optional CI monitoring with automated fix attempts

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System design and component interactions |
| [Workflow](docs/workflow.md) | Detailed phase documentation |
| [Configuration](docs/configuration.md) | Complete configuration reference |
| [Providers](docs/providers.md) | GitHub, Gitea, and provider extension guide |
| [CLI Reference](docs/cli.md) | Command-line interface documentation |
| [Troubleshooting](docs/troubleshooting.md) | Common issues and solutions |
| [Contributing](CONTRIBUTING.md) | Development setup and contribution guide |
| [ADRs](docs/adr/) | Architecture Decision Records |
| [AI Agents](AGENTS.md) | Context for AI-assisted development |

## Quick Start

### Build

```bash
go build -o ultra-engineer ./cmd/ultra-engineer
```

### Configure

Copy `config.example.yaml` to `config.yaml`:

```yaml
provider: github  # or gitea
trigger_label: ai-implement

github:
  token: ${GITHUB_TOKEN}

claude:
  command: claude
  timeout: 30m
```

See [Configuration Guide](docs/configuration.md) for all options.

### Run

```bash
# Continuous daemon mode
ultra-engineer daemon --repo owner/repo

# Single issue processing
ultra-engineer run --repo owner/repo --issue 123
```

## Commands

| Command | Description |
|---------|-------------|
| `daemon` | Continuously poll and process issues |
| `run` | Process a single issue |
| `status` | Show processing status |
| `abort` | Stop processing an issue |
| `version` | Print version info |

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

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | `-c` | Path to config file (default: `config.yaml`) |
| `--verbose` | `-v` | Enable verbose logging |
| `--log-file` | | Path to log file |

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

See [Workflow Documentation](docs/workflow.md) for detailed phase descriptions.

## Supported Providers

| Provider | Status | Implementation |
|----------|--------|----------------|
| GitHub | Full support | `gh` CLI |
| Gitea | Full support | HTTP API |
| GitLab | Config-ready | Not yet implemented |

See [Provider Documentation](docs/providers.md) for setup instructions.

## License

[MIT License](LICENSE)
