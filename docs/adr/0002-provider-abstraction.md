# ADR-0002: Provider Abstraction

## Status

Accepted

## Context

Ultra Engineer needs to support multiple Git hosting platforms:

- **GitHub**: The most popular platform, widely used in open source
- **Gitea**: Popular self-hosted alternative, used in enterprise environments
- **GitLab**: Another major platform (planned for future support)

Each platform has different APIs, authentication methods, and capabilities. We need a way to write platform-agnostic orchestration logic while supporting platform-specific implementations.

## Decision

We will define a `Provider` interface that abstracts all Git platform operations.

**Interface Design**: The `Provider` interface includes 18 methods organized by category:

**Issue Operations (7 methods)**:
- `GetIssue` - Retrieve issue details
- `ListIssuesWithLabel` - Find issues with trigger label
- `GetComments` - Get issue comments
- `CreateComment` - Post a new comment
- `UpdateComment` - Edit existing comment
- `UpdateIssueBody` - Modify issue body (for state storage)
- `ReactToComment` - Add reactions to comments

**Label Operations (2 methods)**:
- `AddLabel` - Add label to issue
- `RemoveLabel` - Remove label from issue

**PR Operations (6 methods)**:
- `CreatePR` - Create pull request
- `GetPR` - Get PR details
- `GetPRComments` - Get PR comments
- `GetPRReviewComments` - Get review comments
- `MergePR` - Merge the PR
- `IsMergeable` - Check if PR can be merged

**Repository Operations (2 methods)**:
- `Clone` - Clone repository to local directory
- `GetDefaultBranch` - Get default branch name

**Provider Info (1 method)**:
- `Name` - Return provider name for logging

**Optional CI Interface**: A separate `CIProvider` interface (2 methods) for platforms supporting CI status:
- `GetCIStatus` - Get CI status for a PR
- `GetCILogs` - Get logs for a check run

**Implementation Strategy**:
- GitHub provider uses `gh` CLI (leverages existing authentication)
- Gitea provider uses direct HTTP API calls
- Mock provider for testing

## Consequences

### Positive

- **Platform agnostic**: Core logic works with any provider
- **Testable**: Mock provider enables comprehensive testing
- **Extensible**: New providers can be added without changing orchestration code
- **Clear contract**: Interface documents required capabilities

### Negative

- **Lowest common denominator**: Some platform-specific features may not be exposed
- **Interface size**: 18 methods is a significant implementation burden for new providers
- **Potential inconsistencies**: Different providers may have subtle behavioral differences

### Neutral

- Each provider is a separate file in `internal/providers/`
- Provider selection is configuration-driven
- CI support is optional (type assertion to check for `CIProvider`)

## Alternatives Considered

### Direct API calls without abstraction

Each workflow phase calls platform APIs directly.

Rejected because:
- Code duplication across platforms
- Tight coupling to specific platforms
- Difficult to test

### Use existing Go libraries for each platform

Use `go-github`, `go-gitea-api`, etc.

Partially adopted: We use `gh` CLI for GitHub (which handles auth), but implement Gitea directly. Full library adoption was rejected because:
- Different libraries have different patterns
- Still need abstraction layer
- `gh` CLI provides better auth handling for GitHub

### Smaller interface with composition

Break the 18-method interface into smaller interfaces.

Rejected because:
- All methods are needed for the workflow
- Would complicate provider instantiation
- No clear benefit for our use case
