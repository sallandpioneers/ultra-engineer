# Contributing to Ultra Engineer

Thank you for your interest in contributing to Ultra Engineer! This guide will help you get started.

## Development Setup

### Prerequisites

- Go 1.23 or later
- Git
- Claude Code CLI (`claude`)
- GitHub CLI (`gh`) - for GitHub provider testing

### Clone and Build

```bash
git clone https://github.com/your-org/ultra-engineer.git
cd ultra-engineer
make build
```

### Running Tests

```bash
make test
```

### Running Linter

```bash
make lint
```

### Full Build Pipeline

```bash
make all  # Runs build, test, and lint
```

## Project Structure

```
.
├── cmd/ultra-engineer/     # CLI entry points
├── internal/               # Internal packages
│   ├── config/             # Configuration loading
│   ├── state/              # Workflow state management
│   ├── providers/          # Git provider implementations
│   ├── orchestrator/       # Core orchestration logic
│   ├── workflow/           # Phase implementations
│   ├── claude/             # Claude CLI integration
│   ├── retry/              # Retry logic
│   ├── progress/           # Progress reporting
│   └── sandbox/            # Working directory isolation
├── docs/                   # Documentation
└── Makefile                # Build targets
```

## Code Style

### Go Conventions

- Follow [Effective Go](https://golang.org/doc/effective_go) guidelines
- Use `gofmt` for formatting (enforced by linter)
- Use `golangci-lint` for static analysis

### Error Handling

- Return errors, don't panic
- Wrap errors with context using `fmt.Errorf` and `%w`
- Handle all errors explicitly

```go
result, err := someFunction()
if err != nil {
    return fmt.Errorf("failed to do something: %w", err)
}
```

### Context Usage

- Pass `context.Context` as the first parameter
- Use context for cancellation and timeouts
- Don't store context in structs

```go
func (p *Provider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
    // Use ctx for cancellation
}
```

### Logging

- Use the configured logger, not `fmt.Print`
- Include relevant context in log messages
- Use appropriate log levels

### Testing

- Place tests in `*_test.go` files alongside implementation
- Use table-driven tests for multiple cases
- Mock external dependencies

```go
func TestParseIssueReferences(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected []int
    }{
        {"simple dependency", "depends on #123", []int{123}},
        {"multiple", "depends on #1 and #2", []int{1, 2}},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### Comments

- Document all exported functions, types, and constants
- Use complete sentences
- Focus on "why" rather than "what"

```go
// GetIssue retrieves issue details from the Git provider.
// It returns ErrNotFound if the issue doesn't exist.
func (p *Provider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error)
```

## Making Changes

### 1. Create a Branch

```bash
git checkout -b feat/your-feature
# or
git checkout -b fix/bug-description
```

Branch naming conventions:
- `feat/` - New features
- `fix/` - Bug fixes
- `refactor/` - Code refactoring
- `docs/` - Documentation updates
- `test/` - Test additions or fixes

### 2. Make Your Changes

- Keep changes focused and atomic
- Update tests for new functionality
- Update documentation if needed

### 3. Run Tests and Linter

```bash
make all
```

### 4. Commit Your Changes

Use conventional commits:

```
feat(provider): add GitLab support

Implement the Provider interface for GitLab using the API.
Includes support for issues, PRs, and labels.

Closes #42
```

Types:
- `feat` - New feature
- `fix` - Bug fix
- `refactor` - Code change that neither fixes nor adds
- `docs` - Documentation only
- `test` - Adding or fixing tests
- `chore` - Maintenance tasks

### 5. Submit a Pull Request

- Fill out the PR template
- Link related issues
- Request review from maintainers

## Adding a New Provider

1. Create `internal/providers/newprovider.go`
2. Implement the `Provider` interface (18 methods)
3. Optionally implement `CIProvider` for CI support
4. Add configuration to `internal/config/config.go`
5. Update `createProvider()` in `cmd/ultra-engineer/main.go`
6. Add tests in `internal/providers/newprovider_test.go`
7. Document in `docs/providers.md`

See [Provider Documentation](docs/providers.md) for interface details.

## Adding a New Workflow Phase

1. Add phase constant to `internal/state/state.go`
2. Create phase handler in `internal/workflow/`
3. Update orchestrator to handle the phase
4. Add tests for the new phase
5. Document in `docs/workflow.md`
6. Update README phase table

## Testing with Mock Provider

Use the mock provider for unit tests:

```go
mock := providers.NewMockProvider()
mock.SetIssue(repo, 42, &providers.Issue{
    Number: 42,
    Title:  "Test Issue",
    Body:   "Description",
})

// Use mock in tests
orchestrator := NewOrchestrator(mock, ...)
```

## Architecture Decision Records

For significant architectural changes, create an ADR:

1. Copy the template from `docs/adr/README.md`
2. Name it `docs/adr/NNNN-short-title.md`
3. Document context, decision, and consequences
4. Update the ADR index

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones
- Join discussions in existing issues

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help others learn and grow

Thank you for contributing!
