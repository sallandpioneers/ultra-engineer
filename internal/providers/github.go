package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitHubProvider implements Provider using the gh CLI
// Note: Authentication is handled by the gh CLI (via GH_TOKEN env var or gh auth login)
type GitHubProvider struct{}

// NewGitHubProvider creates a new GitHub provider
// The token parameter is kept for interface consistency but authentication
// is handled by the gh CLI itself
func NewGitHubProvider(token string) *GitHubProvider {
	// If a token is provided, set it as an environment variable for gh CLI
	// This allows explicit token configuration while still using gh CLI
	// Note: This is not thread-safe, but provider creation should happen
	// once during startup, not concurrently
	if token != "" {
		os.Setenv("GH_TOKEN", token)
	}
	return &GitHubProvider{}
}

func (g *GitHubProvider) Name() string {
	return "github"
}

// ghCmd creates a gh command with common flags
func (g *GitHubProvider) ghCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gh", args...)
	return cmd
}

// runGH executes a gh command and returns stdout
func (g *GitHubProvider) runGH(ctx context.Context, args ...string) ([]byte, error) {
	cmd := g.ghCmd(ctx, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("gh command failed: %s: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}

// ghIssue represents gh's JSON output for issues
type ghIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Author    ghUser    `json:"author"`
	Labels    []ghLabel `json:"labels"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghLabel struct {
	Name string `json:"name"`
}

type ghComment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Author    ghUser    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

type ghPR struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	State          string `json:"state"`
	MergeStateStatus string `json:"mergeStateStatus"`
	URL            string `json:"url"`
	HeadRefName    string `json:"headRefName"`
	BaseRefName    string `json:"baseRefName"`
}

func (g *GitHubProvider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	out, err := g.runGH(ctx, "issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,body,state,author,labels,createdAt,updatedAt")
	if err != nil {
		return nil, err
	}

	var gi ghIssue
	if err := json.Unmarshal(out, &gi); err != nil {
		return nil, fmt.Errorf("failed to parse issue: %w", err)
	}

	labels := make([]string, len(gi.Labels))
	for i, l := range gi.Labels {
		labels[i] = l.Name
	}

	return &Issue{
		Number:    gi.Number,
		Title:     gi.Title,
		Body:      gi.Body,
		Labels:    labels,
		State:     gi.State,
		Author:    gi.Author.Login,
		CreatedAt: gi.CreatedAt,
		UpdatedAt: gi.UpdatedAt,
	}, nil
}

func (g *GitHubProvider) ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error) {
	out, err := g.runGH(ctx, "issue", "list", "--repo", repo, "--label", label, "--state", "open", "--json", "number,title,body,state,author,labels,createdAt,updatedAt")
	if err != nil {
		return nil, err
	}

	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	result := make([]*Issue, len(issues))
	for i, gi := range issues {
		labels := make([]string, len(gi.Labels))
		for j, l := range gi.Labels {
			labels[j] = l.Name
		}
		result[i] = &Issue{
			Number:    gi.Number,
			Title:     gi.Title,
			Body:      gi.Body,
			Labels:    labels,
			State:     gi.State,
			Author:    gi.Author.Login,
			CreatedAt: gi.CreatedAt,
			UpdatedAt: gi.UpdatedAt,
		}
	}

	return result, nil
}

func (g *GitHubProvider) GetComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	out, err := g.runGH(ctx, "issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "comments", "--jq", ".comments")
	if err != nil {
		return nil, err
	}

	var comments []ghComment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
	}

	result := make([]*Comment, len(comments))
	for i, c := range comments {
		// GitHub's gh CLI returns GraphQL node IDs (strings like "IC_kwDOOTmGh85y...")
		// We need a stable numeric ID for comparison. Use a hash of the node ID.
		var id int64
		if _, err := fmt.Sscanf(c.ID, "%d", &id); err != nil || id == 0 {
			// Generate a stable hash from the node ID
			id = hashNodeID(c.ID)
		}
		result[i] = &Comment{
			ID:        id,
			Body:      c.Body,
			Author:    c.Author.Login,
			CreatedAt: c.CreatedAt,
		}
	}

	return result, nil
}

func (g *GitHubProvider) CreateComment(ctx context.Context, repo string, number int, body string) error {
	_, err := g.runGH(ctx, "issue", "comment", strconv.Itoa(number), "--repo", repo, "--body", body)
	return err
}

func (g *GitHubProvider) UpdateIssueBody(ctx context.Context, repo string, number int, body string) error {
	_, err := g.runGH(ctx, "issue", "edit", strconv.Itoa(number), "--repo", repo, "--body", body)
	return err
}

func (g *GitHubProvider) ReactToComment(ctx context.Context, repo string, commentID int64, reaction string) error {
	// Use gh api to add a reaction to a comment
	endpoint := fmt.Sprintf("/repos/%s/issues/comments/%d/reactions", repo, commentID)
	_, err := g.runGH(ctx, "api", endpoint, "-X", "POST", "-f", "content="+reaction)
	return err
}

func (g *GitHubProvider) AddLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := g.runGH(ctx, "issue", "edit", strconv.Itoa(number), "--repo", repo, "--add-label", label)
	return err
}

func (g *GitHubProvider) RemoveLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := g.runGH(ctx, "issue", "edit", strconv.Itoa(number), "--repo", repo, "--remove-label", label)
	return err
}

func (g *GitHubProvider) CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error) {
	args := []string{"pr", "create", "--repo", repo, "--title", pr.Title, "--body", pr.Body, "--head", pr.Head, "--base", pr.Base}
	_, err := g.runGH(ctx, args...)
	if err != nil {
		return nil, err
	}

	// Get the PR we just created
	out, err := g.runGH(ctx, "pr", "view", pr.Head, "--repo", repo, "--json", "number,title,body,state,mergeStateStatus,url,headRefName,baseRefName")
	if err != nil {
		return nil, err
	}

	var gp ghPR
	if err := json.Unmarshal(out, &gp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    gp.Number,
		Title:     gp.Title,
		Body:      gp.Body,
		State:     gp.State,
		Mergeable: gp.MergeStateStatus == "CLEAN" || gp.MergeStateStatus == "MERGEABLE",
		HTMLURL:   gp.URL,
		HeadRef:   gp.HeadRefName,
		BaseRef:   gp.BaseRefName,
	}, nil
}

func (g *GitHubProvider) GetPR(ctx context.Context, repo string, number int) (*PR, error) {
	out, err := g.runGH(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,body,state,mergeStateStatus,url,headRefName,baseRefName")
	if err != nil {
		return nil, err
	}

	var gp ghPR
	if err := json.Unmarshal(out, &gp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    gp.Number,
		Title:     gp.Title,
		Body:      gp.Body,
		State:     gp.State,
		Mergeable: gp.MergeStateStatus == "CLEAN" || gp.MergeStateStatus == "MERGEABLE",
		HTMLURL:   gp.URL,
		HeadRef:   gp.HeadRefName,
		BaseRef:   gp.BaseRefName,
	}, nil
}

func (g *GitHubProvider) GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	out, err := g.runGH(ctx, "pr", "view", strconv.Itoa(number), "--repo", repo, "--json", "comments", "--jq", ".comments")
	if err != nil {
		return nil, err
	}

	var comments []ghComment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
	}

	result := make([]*Comment, len(comments))
	for i, c := range comments {
		// GitHub's gh CLI returns GraphQL node IDs (strings like "IC_kwDOOTmGh85y...")
		// We need a stable numeric ID for comparison. Use a hash of the node ID.
		var id int64
		if _, err := fmt.Sscanf(c.ID, "%d", &id); err != nil || id == 0 {
			// Generate a stable hash from the node ID
			id = hashNodeID(c.ID)
		}
		result[i] = &Comment{
			ID:        id,
			Body:      c.Body,
			Author:    c.Author.Login,
			CreatedAt: c.CreatedAt,
		}
	}

	return result, nil
}

func (g *GitHubProvider) MergePR(ctx context.Context, repo string, number int) error {
	_, err := g.runGH(ctx, "pr", "merge", strconv.Itoa(number), "--repo", repo, "--merge", "--delete-branch")
	return err
}

func (g *GitHubProvider) IsMergeable(ctx context.Context, repo string, number int) (bool, error) {
	pr, err := g.GetPR(ctx, repo, number)
	if err != nil {
		return false, err
	}
	return pr.Mergeable, nil
}

func (g *GitHubProvider) Clone(ctx context.Context, repo string, dest string) error {
	cmd := g.ghCmd(ctx, "repo", "clone", repo, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w: %s", err, string(output))
	}
	return nil
}

func (g *GitHubProvider) GetDefaultBranch(ctx context.Context, repo string) (string, error) {
	out, err := g.runGH(ctx, "repo", "view", repo, "--json", "defaultBranchRef", "--jq", ".defaultBranchRef.name")
	if err != nil {
		return "", err
	}

	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "main", nil
	}
	return branch, nil
}

// hashNodeID generates a stable int64 hash from a GitHub node ID string
func hashNodeID(nodeID string) int64 {
	// Use FNV-1a hash algorithm for stable hashing
	var hash uint64 = 14695981039346656037 // FNV offset basis
	for i := 0; i < len(nodeID); i++ {
		hash ^= uint64(nodeID[i])
		hash *= 1099511628211 // FNV prime
	}
	// Convert to int64, keeping it positive for consistency
	return int64(hash & 0x7FFFFFFFFFFFFFFF)
}
