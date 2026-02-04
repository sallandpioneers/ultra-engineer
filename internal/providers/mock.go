package providers

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MockProvider is a mock implementation of Provider for testing
type MockProvider struct {
	mu sync.RWMutex

	// Issue storage
	Issues   map[string]map[int]*Issue   // repo -> issueNum -> issue
	Comments map[string]map[int][]*Comment // repo -> issueNum -> comments

	// PR storage
	PRs map[string]map[int]*PR // repo -> prNum -> pr

	// Tracking calls for assertions
	CreatedComments []MockComment
	UpdatedComments []MockCommentUpdate
	AddedLabels     []MockLabel
	RemovedLabels   []MockLabel
	Reactions       []MockReaction

	// Configurable behavior
	DefaultBranch string
	CloneError    error
	MergeError    error
}

// MockComment tracks created comments
type MockComment struct {
	ID        int64
	Repo      string
	IssueNum  int
	Body      string
	CreatedAt time.Time
}

// MockCommentUpdate tracks comment updates
type MockCommentUpdate struct {
	Repo      string
	CommentID int64
	Body      string
}

// MockLabel tracks label operations
type MockLabel struct {
	Repo     string
	IssueNum int
	Label    string
}

// MockReaction tracks reactions
type MockReaction struct {
	Repo      string
	CommentID int64
	Reaction  string
}

// NewMockProvider creates a new mock provider
func NewMockProvider() *MockProvider {
	return &MockProvider{
		Issues:        make(map[string]map[int]*Issue),
		Comments:      make(map[string]map[int][]*Comment),
		PRs:           make(map[string]map[int]*PR),
		DefaultBranch: "main",
	}
}

// AddIssue adds an issue to the mock
func (m *MockProvider) AddIssue(repo string, issue *Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Issues[repo] == nil {
		m.Issues[repo] = make(map[int]*Issue)
	}
	m.Issues[repo][issue.Number] = issue
}

// GetIssue implements Provider
func (m *MockProvider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if repoIssues, ok := m.Issues[repo]; ok {
		if issue, ok := repoIssues[number]; ok {
			return issue, nil
		}
	}
	return nil, fmt.Errorf("issue not found: %s#%d", repo, number)
}

// ListIssuesWithLabel implements Provider
func (m *MockProvider) ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Issue
	if repoIssues, ok := m.Issues[repo]; ok {
		for _, issue := range repoIssues {
			for _, l := range issue.Labels {
				if l == label {
					result = append(result, issue)
					break
				}
			}
		}
	}
	return result, nil
}

// GetComments implements Provider
func (m *MockProvider) GetComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if repoComments, ok := m.Comments[repo]; ok {
		if comments, ok := repoComments[number]; ok {
			return comments, nil
		}
	}
	return []*Comment{}, nil
}

// CreateComment implements Provider
func (m *MockProvider) CreateComment(ctx context.Context, repo string, number int, body string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Comments[repo] == nil {
		m.Comments[repo] = make(map[int][]*Comment)
	}

	commentID := int64(len(m.CreatedComments) + 1)
	comment := &Comment{
		ID:        commentID,
		Body:      body,
		Author:    "ultra-engineer[bot]",
		CreatedAt: time.Now(),
	}

	m.Comments[repo][number] = append(m.Comments[repo][number], comment)
	m.CreatedComments = append(m.CreatedComments, MockComment{
		ID:        commentID,
		Repo:      repo,
		IssueNum:  number,
		Body:      body,
		CreatedAt: comment.CreatedAt,
	})

	return commentID, nil
}

// UpdateComment implements Provider
func (m *MockProvider) UpdateComment(ctx context.Context, repo string, commentID int64, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Update the comment in storage
	for _, issueComments := range m.Comments[repo] {
		for _, comment := range issueComments {
			if comment.ID == commentID {
				comment.Body = body
				break
			}
		}
	}

	m.UpdatedComments = append(m.UpdatedComments, MockCommentUpdate{
		Repo:      repo,
		CommentID: commentID,
		Body:      body,
	})

	return nil
}

// UpdateIssueBody implements Provider
func (m *MockProvider) UpdateIssueBody(ctx context.Context, repo string, number int, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if repoIssues, ok := m.Issues[repo]; ok {
		if issue, ok := repoIssues[number]; ok {
			issue.Body = body
			return nil
		}
	}
	return fmt.Errorf("issue not found")
}

// ReactToComment implements Provider
func (m *MockProvider) ReactToComment(ctx context.Context, repo string, commentID int64, reaction string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Reactions = append(m.Reactions, MockReaction{
		Repo:      repo,
		CommentID: commentID,
		Reaction:  reaction,
	})
	return nil
}

// AddLabel implements Provider
func (m *MockProvider) AddLabel(ctx context.Context, repo string, number int, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if repoIssues, ok := m.Issues[repo]; ok {
		if issue, ok := repoIssues[number]; ok {
			// Check if label already exists
			for _, l := range issue.Labels {
				if l == label {
					return nil
				}
			}
			issue.Labels = append(issue.Labels, label)
		}
	}

	m.AddedLabels = append(m.AddedLabels, MockLabel{
		Repo:     repo,
		IssueNum: number,
		Label:    label,
	})
	return nil
}

// RemoveLabel implements Provider
func (m *MockProvider) RemoveLabel(ctx context.Context, repo string, number int, label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if repoIssues, ok := m.Issues[repo]; ok {
		if issue, ok := repoIssues[number]; ok {
			var newLabels []string
			for _, l := range issue.Labels {
				if l != label {
					newLabels = append(newLabels, l)
				}
			}
			issue.Labels = newLabels
		}
	}

	m.RemovedLabels = append(m.RemovedLabels, MockLabel{
		Repo:     repo,
		IssueNum: number,
		Label:    label,
	})
	return nil
}

// CreatePR implements Provider
func (m *MockProvider) CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.PRs[repo] == nil {
		m.PRs[repo] = make(map[int]*PR)
	}

	prNum := len(m.PRs[repo]) + 1
	newPR := &PR{
		Number:    prNum,
		Title:     pr.Title,
		Body:      pr.Body,
		State:     "open",
		Mergeable: true,
		HTMLURL:   fmt.Sprintf("https://example.com/%s/pull/%d", repo, prNum),
		HeadRef:   pr.Head,
		BaseRef:   pr.Base,
	}

	m.PRs[repo][prNum] = newPR
	return newPR, nil
}

// GetPR implements Provider
func (m *MockProvider) GetPR(ctx context.Context, repo string, number int) (*PR, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if repoPRs, ok := m.PRs[repo]; ok {
		if pr, ok := repoPRs[number]; ok {
			return pr, nil
		}
	}
	return nil, fmt.Errorf("PR not found")
}

// GetPRComments implements Provider
func (m *MockProvider) GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	return []*Comment{}, nil
}

// MergePR implements Provider
func (m *MockProvider) MergePR(ctx context.Context, repo string, number int) error {
	if m.MergeError != nil {
		return m.MergeError
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if repoPRs, ok := m.PRs[repo]; ok {
		if pr, ok := repoPRs[number]; ok {
			pr.State = "merged"
			return nil
		}
	}
	return fmt.Errorf("PR not found")
}

// IsMergeable implements Provider
func (m *MockProvider) IsMergeable(ctx context.Context, repo string, number int) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if repoPRs, ok := m.PRs[repo]; ok {
		if pr, ok := repoPRs[number]; ok {
			return pr.Mergeable, nil
		}
	}
	return false, fmt.Errorf("PR not found")
}

// Clone implements Provider
func (m *MockProvider) Clone(ctx context.Context, repo string, dest string) error {
	return m.CloneError
}

// GetDefaultBranch implements Provider
func (m *MockProvider) GetDefaultBranch(ctx context.Context, repo string) (string, error) {
	return m.DefaultBranch, nil
}

// Name implements Provider
func (m *MockProvider) Name() string {
	return "mock"
}

// Helper methods for testing

// AddComment adds a comment to an issue (simulating user comment)
func (m *MockProvider) AddComment(repo string, issueNum int, comment *Comment) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Comments[repo] == nil {
		m.Comments[repo] = make(map[int][]*Comment)
	}
	m.Comments[repo][issueNum] = append(m.Comments[repo][issueNum], comment)
}

// SetPRMergeable sets the mergeable status of a PR
func (m *MockProvider) SetPRMergeable(repo string, prNum int, mergeable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if repoPRs, ok := m.PRs[repo]; ok {
		if pr, ok := repoPRs[prNum]; ok {
			pr.Mergeable = mergeable
		}
	}
}

// Reset clears all mock state
func (m *MockProvider) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Issues = make(map[string]map[int]*Issue)
	m.Comments = make(map[string]map[int][]*Comment)
	m.PRs = make(map[string]map[int]*PR)
	m.CreatedComments = nil
	m.UpdatedComments = nil
	m.AddedLabels = nil
	m.RemovedLabels = nil
	m.Reactions = nil
}
