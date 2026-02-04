# Provider Documentation

Ultra Engineer supports multiple Git hosting platforms through a provider abstraction layer.

## Supported Providers

| Provider | Status | Implementation |
|----------|--------|----------------|
| GitHub | Full support | `gh` CLI |
| Gitea | Full support | HTTP API |
| GitLab | Config-ready | Not yet implemented |

## GitHub Setup

### Requirements

1. **GitHub CLI (`gh`)**: Must be installed and accessible in PATH
2. **Authentication**: Either via `gh auth login` or `GITHUB_TOKEN`

### Installation

```bash
# macOS
brew install gh

# Ubuntu/Debian
sudo apt install gh

# Or download from https://cli.github.com/
```

### Authentication

Option 1: Interactive login
```bash
gh auth login
```

Option 2: Token in configuration
```yaml
provider: github
github:
  token: ${GITHUB_TOKEN}
```

The token is passed to `gh` via the `GH_TOKEN` environment variable.

### Required Permissions

The token or authenticated user needs:
- `repo` scope for private repositories
- `public_repo` scope for public repositories

### Configuration

```yaml
provider: github
github:
  token: ${GITHUB_TOKEN}  # Optional if using gh auth
```

## Gitea Setup

### Requirements

1. **API Token**: Generated from Gitea settings
2. **Network access**: HTTP access to Gitea instance

### Creating an API Token

1. Go to **Settings** → **Applications** → **Access Tokens**
2. Generate a new token with required permissions
3. Copy the token (shown only once)

### Required Permissions

- `read:issue` - Read issues
- `write:issue` - Create comments, update labels
- `read:repository` - Clone repositories
- `write:repository` - Push branches, create PRs

### Configuration

```yaml
provider: gitea
gitea:
  url: https://gitea.example.com
  token: ${GITEA_TOKEN}
```

### Implementation Details

- Uses direct HTTP API calls
- 30-second timeout per request
- Supports retry with exponential backoff

## GitLab Setup

**Note**: GitLab configuration is accepted but the provider is not yet implemented.

### Configuration (Future)

```yaml
provider: gitlab
gitlab:
  url: https://gitlab.example.com
  token: ${GITLAB_TOKEN}
```

## Provider Interface

The `Provider` interface defines 18 methods organized by category.

### Issue Operations (7 methods)

```go
// GetIssue retrieves issue details
GetIssue(ctx context.Context, repo string, number int) (*Issue, error)

// ListIssuesWithLabel finds issues with a specific label
ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error)

// GetComments retrieves all comments on an issue
GetComments(ctx context.Context, repo string, number int) ([]*Comment, error)

// CreateComment posts a new comment
CreateComment(ctx context.Context, repo string, number int, body string) (int64, error)

// UpdateComment edits an existing comment
UpdateComment(ctx context.Context, repo string, commentID int64, body string) error

// UpdateIssueBody modifies the issue body (used for state storage)
UpdateIssueBody(ctx context.Context, repo string, number int, body string) error

// ReactToComment adds a reaction to a comment
ReactToComment(ctx context.Context, repo string, commentID int64, reaction string) error
```

### Label Operations (2 methods)

```go
// AddLabel adds a label to an issue
AddLabel(ctx context.Context, repo string, number int, label string) error

// RemoveLabel removes a label from an issue
RemoveLabel(ctx context.Context, repo string, number int, label string) error
```

### PR Operations (6 methods)

```go
// CreatePR creates a new pull request
CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error)

// GetPR retrieves PR details
GetPR(ctx context.Context, repo string, number int) (*PR, error)

// GetPRComments retrieves comments on a PR
GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error)

// GetPRReviewComments retrieves review comments on a PR
GetPRReviewComments(ctx context.Context, repo string, number int) ([]*Comment, error)

// MergePR merges a pull request
MergePR(ctx context.Context, repo string, number int) error

// IsMergeable checks if a PR can be merged
IsMergeable(ctx context.Context, repo string, number int) (bool, error)
```

### Repository Operations (2 methods)

```go
// Clone clones a repository to a local directory
Clone(ctx context.Context, repo string, dest string) error

// GetDefaultBranch returns the repository's default branch
GetDefaultBranch(ctx context.Context, repo string) (string, error)
```

### Provider Info (1 method)

```go
// Name returns the provider name for logging
Name() string
```

## CI Provider Interface

An optional interface for providers supporting CI status:

```go
type CIProvider interface {
    // GetCIStatus retrieves CI status for a PR
    GetCIStatus(ctx context.Context, repo string, prNumber int) (*CIResult, error)

    // GetCILogs retrieves logs for a specific check run
    GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error)
}
```

### CIResult Structure

```go
type CIStatus string

const (
    CIStatusPending CIStatus = "pending"
    CIStatusSuccess CIStatus = "success"
    CIStatusFailure CIStatus = "failure"
    CIStatusUnknown CIStatus = "unknown"
)

type CICheck struct {
    ID         int64     // Check run ID (for fetching logs)
    Name       string    // Check name
    Status     CIStatus  // Current status
    Conclusion string    // success, failure, cancelled, etc.
    DetailsURL string    // URL to view details
    Output     string    // Summary output
}

type CIResult struct {
    OverallStatus CIStatus  // Combined status of all checks
    Checks        []CICheck // Individual check results
}
```

## Extending with a New Provider

### Step 1: Create Provider File

Create `internal/providers/newprovider.go`:

```go
package providers

import (
    "context"
)

type NewProvider struct {
    url   string
    token string
}

func NewNewProvider(url, token string) *NewProvider {
    return &NewProvider{url: url, token: token}
}

func (p *NewProvider) Name() string {
    return "newprovider"
}

// Implement all 18 Provider interface methods...
```

### Step 2: Implement Interface Methods

Implement all methods from the `Provider` interface. See existing implementations for reference:
- `github.go` - Uses CLI tool
- `gitea.go` - Uses HTTP API

### Step 3: Optional CI Support

To support CI monitoring, also implement `CIProvider`:

```go
func (p *NewProvider) GetCIStatus(ctx context.Context, repo string, prNumber int) (*CIResult, error) {
    // Implementation
}

func (p *NewProvider) GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error) {
    // Implementation
}
```

### Step 4: Register Provider

Update `cmd/ultra-engineer/main.go`:

```go
func createProvider(cfg *config.Config) (providers.Provider, error) {
    switch cfg.Provider {
    case "gitea":
        return providers.NewGiteaProvider(cfg.Gitea.URL, cfg.Gitea.Token), nil
    case "github":
        return providers.NewGitHubProvider(cfg.GitHub.Token), nil
    case "newprovider":  // Add your provider
        return providers.NewNewProvider(cfg.NewProvider.URL, cfg.NewProvider.Token), nil
    default:
        return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
    }
}
```

### Step 5: Add Configuration

Add configuration struct to `internal/config/config.go`:

```go
type NewProviderConfig struct {
    URL   string `yaml:"url"`
    Token string `yaml:"token"`
}

type Config struct {
    // ...
    NewProvider NewProviderConfig `yaml:"newprovider"`
}
```

## Mock Provider

A mock provider is available for testing in `internal/providers/mock.go`. It implements all interface methods with configurable responses for unit testing.
