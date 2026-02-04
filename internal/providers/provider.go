package providers

import (
	"context"
	"time"
)

// Issue represents an issue from any provider
type Issue struct {
	Number      int
	Title       string
	Body        string
	Labels      []string
	State       string
	Author      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CommentsURL string
}

// Comment represents a comment on an issue or PR
type Comment struct {
	ID        int64
	Body      string
	Author    string
	CreatedAt time.Time
}

// PR represents a pull request
type PR struct {
	Number    int
	Title     string
	Body      string
	State     string
	Mergeable bool
	HTMLURL   string
	HeadRef   string
	BaseRef   string
}

// PRCreate contains fields for creating a PR
type PRCreate struct {
	Title   string
	Body    string
	Head    string
	Base    string
	IssueID int // Link to issue if supported
}

// Provider defines the interface for git providers
type Provider interface {
	// Issue operations
	GetIssue(ctx context.Context, repo string, number int) (*Issue, error)
	ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error)
	GetComments(ctx context.Context, repo string, number int) ([]*Comment, error)
	CreateComment(ctx context.Context, repo string, number int, body string) (int64, error)
	UpdateComment(ctx context.Context, repo string, commentID int64, body string) error
	UpdateIssueBody(ctx context.Context, repo string, number int, body string) error
	ReactToComment(ctx context.Context, repo string, commentID int64, reaction string) error

	// Label operations
	AddLabel(ctx context.Context, repo string, number int, label string) error
	RemoveLabel(ctx context.Context, repo string, number int, label string) error

	// PR operations
	CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error)
	GetPR(ctx context.Context, repo string, number int) (*PR, error)
	GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error)
	GetPRReviewComments(ctx context.Context, repo string, number int) ([]*Comment, error)
	MergePR(ctx context.Context, repo string, number int) error
	IsMergeable(ctx context.Context, repo string, number int) (bool, error)

	// Repository operations
	Clone(ctx context.Context, repo string, dest string) error
	GetDefaultBranch(ctx context.Context, repo string) (string, error)

	// Authorization
	IsCollaborator(ctx context.Context, repo, username string) (bool, error)

	// Provider info
	Name() string
}

// CIStatus represents the status of a CI check
type CIStatus string

const (
	CIStatusPending CIStatus = "pending"
	CIStatusSuccess CIStatus = "success"
	CIStatusFailure CIStatus = "failure"
	CIStatusUnknown CIStatus = "unknown"
)

// CICheck represents a single CI check run
type CICheck struct {
	ID         int64    // Check run ID (for fetching logs)
	Name       string   // Check name
	Status     CIStatus // Check status
	Conclusion string   // Conclusion (success, failure, cancelled, etc.)
	DetailsURL string   // URL to view details
	Output     string   // Summary output (may be truncated)
}

// CIResult represents the combined CI status for a PR
type CIResult struct {
	OverallStatus CIStatus  // Combined status
	Checks        []CICheck // Individual checks
}

// CIProvider is an optional interface for CI operations
// Use type assertion: if ciProvider, ok := provider.(CIProvider); ok { ... }
type CIProvider interface {
	// GetCIStatus returns the CI status for a PR
	GetCIStatus(ctx context.Context, repo string, prNumber int) (*CIResult, error)

	// GetCILogs retrieves logs for a specific check run
	GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error)
}
