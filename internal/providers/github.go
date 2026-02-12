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

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/retry"
)

// GitHubProvider implements Provider using the gh CLI
// Note: Authentication is handled by the gh CLI (via GH_TOKEN env var or gh auth login)
type GitHubProvider struct {
	retryOpts *retry.Options
}

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

// NewGitHubProviderWithRetry creates a new GitHub provider with retry support
func NewGitHubProviderWithRetry(token string, retryConfig config.RetryConfig) *GitHubProvider {
	if token != "" {
		os.Setenv("GH_TOKEN", token)
	}
	opts := retry.DefaultOptions(retryConfig)
	opts.Classifier = retry.ClassifyHTTPError
	return &GitHubProvider{
		retryOpts: &opts,
	}
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
	// If retry is configured, use retry logic
	if g.retryOpts != nil {
		return retry.DoWithResult(ctx, *g.retryOpts, func() ([]byte, error) {
			return g.runGHOnce(ctx, args...)
		})
	}
	return g.runGHOnce(ctx, args...)
}

// runGHOnce executes a single gh command
func (g *GitHubProvider) runGHOnce(ctx context.Context, args ...string) ([]byte, error) {
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
	Number           int    `json:"number"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	State            string `json:"state"`
	MergeStateStatus string `json:"mergeStateStatus"`
	URL              string `json:"url"`
	HeadRefName      string `json:"headRefName"`
	BaseRefName      string `json:"baseRefName"`
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

func (g *GitHubProvider) CreateComment(ctx context.Context, repo string, number int, body string) (int64, error) {
	// Use gh api to create a comment and get the ID back
	endpoint := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	out, err := g.runGH(ctx, "api", endpoint, "-X", "POST", "-f", "body="+body)
	if err != nil {
		return 0, err
	}

	// Parse the response to get the comment ID
	var response struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(out, &response); err != nil {
		return 0, fmt.Errorf("failed to parse comment response: %w", err)
	}
	return response.ID, nil
}

func (g *GitHubProvider) UpdateComment(ctx context.Context, repo string, commentID int64, body string) error {
	endpoint := fmt.Sprintf("/repos/%s/issues/comments/%d", repo, commentID)
	_, err := g.runGH(ctx, "api", endpoint, "-X", "PATCH", "-f", "body="+body)
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

// ghReviewComment represents the REST API response for PR review comments (inline code comments)
type ghReviewComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      ghUser    `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (g *GitHubProvider) GetPRReviewComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	// Use gh api to fetch inline review comments from the REST API
	// Endpoint: repos/{owner}/{repo}/pulls/{pull_number}/comments
	out, err := g.runGH(ctx, "api", fmt.Sprintf("repos/%s/pulls/%d/comments", repo, number))
	if err != nil {
		return nil, err
	}

	var comments []ghReviewComment
	if err := json.Unmarshal(out, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse review comments: %w", err)
	}

	result := make([]*Comment, len(comments))
	for i, c := range comments {
		result[i] = &Comment{
			ID:        c.ID,
			Body:      c.Body,
			Author:    c.User.Login,
			CreatedAt: c.CreatedAt,
		}
	}

	return result, nil
}

func (g *GitHubProvider) MergePR(ctx context.Context, repo string, number int) error {
	_, err := g.runGH(ctx, "pr", "merge", strconv.Itoa(number), "--repo", repo, "--merge", "--delete-branch")
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "not allowed") || strings.Contains(errStr, "merge not allowed") ||
			strings.Contains(errStr, "required status check") || strings.Contains(errStr, "review is required") {
			return fmt.Errorf("%w: %v", ErrMergeNotAllowed, err)
		}
	}
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

// ghCheck represents the JSON output from gh pr checks
type ghCheck struct {
	Name        string `json:"name"`
	State       string `json:"state"`      // PENDING, SUCCESS, FAILURE, etc.
	Status      string `json:"status"`     // queued, in_progress, completed
	Conclusion  string `json:"conclusion"` // success, failure, cancelled, skipped, etc.
	DetailsURL  string `json:"detailsUrl"`
	Description string `json:"description"`
	WorkflowID  int64  `json:"workflowId"`
}

// GetCIStatus implements CIProvider for GitHub
func (g *GitHubProvider) GetCIStatus(ctx context.Context, repo string, prNumber int) (*CIResult, error) {
	out, err := g.runGH(ctx, "pr", "checks", strconv.Itoa(prNumber), "--repo", repo, "--json", "name,state,status,conclusion,detailsUrl,description,workflowId")
	if err != nil {
		return nil, fmt.Errorf("failed to get PR checks: %w", err)
	}

	var checks []ghCheck
	if err := json.Unmarshal(out, &checks); err != nil {
		return nil, fmt.Errorf("failed to parse PR checks: %w", err)
	}

	result := &CIResult{
		OverallStatus: CIStatusSuccess, // Default to success if no checks
		Checks:        make([]CICheck, 0, len(checks)),
	}

	hasPending := false
	hasFailure := false

	for _, c := range checks {
		check := CICheck{
			ID:         c.WorkflowID,
			Name:       c.Name,
			Conclusion: c.Conclusion,
			DetailsURL: c.DetailsURL,
			Output:     c.Description,
		}

		// Map GitHub state/conclusion to CIStatus
		switch strings.ToUpper(c.State) {
		case "PENDING":
			check.Status = CIStatusPending
			hasPending = true
		case "SUCCESS":
			check.Status = CIStatusSuccess
		case "FAILURE", "ERROR":
			check.Status = CIStatusFailure
			hasFailure = true
		default:
			// Check conclusion for completed checks
			switch strings.ToLower(c.Conclusion) {
			case "success":
				check.Status = CIStatusSuccess
			case "failure", "timed_out", "action_required":
				check.Status = CIStatusFailure
				hasFailure = true
			case "cancelled", "skipped", "neutral":
				check.Status = CIStatusSuccess // Treat as non-blocking
			default:
				check.Status = CIStatusPending
				hasPending = true
			}
		}

		result.Checks = append(result.Checks, check)
	}

	// Determine overall status
	if hasFailure {
		result.OverallStatus = CIStatusFailure
	} else if hasPending {
		result.OverallStatus = CIStatusPending
	} else if len(result.Checks) == 0 {
		result.OverallStatus = CIStatusUnknown
	}

	return result, nil
}

// IsCollaborator checks if a user is a collaborator on the repository
func (g *GitHubProvider) IsCollaborator(ctx context.Context, repo, username string) (bool, error) {
	// Use gh api to check collaborator permission
	// Endpoint: repos/{owner}/{repo}/collaborators/{username}/permission
	endpoint := fmt.Sprintf("repos/%s/collaborators/%s/permission", repo, username)
	out, err := g.runGH(ctx, "api", endpoint)
	if err != nil {
		// GitHub returns 404 for users who are not collaborators at all
		if strings.Contains(err.Error(), "404") {
			return false, nil
		}
		// For other errors (network, auth, etc.), fail closed
		return false, err
	}

	// Parse the permission response
	type permissionResponse struct {
		Permission string `json:"permission"`
	}
	var resp permissionResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return false, fmt.Errorf("failed to parse permission response: %w", err)
	}

	// Consider user authorized if they have admin, maintain, write, or triage permissions
	// Users with only "read" permission should NOT be authorized
	switch resp.Permission {
	case "admin", "maintain", "write", "triage":
		return true, nil
	default:
		return false, nil
	}
}

// GetCILogs retrieves logs for a GitHub Actions workflow run
func (g *GitHubProvider) GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error) {
	// Use gh api to fetch the check run details with output
	endpoint := fmt.Sprintf("/repos/%s/check-runs/%d", repo, checkRunID)
	out, err := g.runGH(ctx, "api", endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to get check run: %w", err)
	}

	var checkRun struct {
		Output struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
			Text    string `json:"text"`
		} `json:"output"`
		Conclusion string `json:"conclusion"`
		HTMLURL    string `json:"html_url"`
	}

	if err := json.Unmarshal(out, &checkRun); err != nil {
		return "", fmt.Errorf("failed to parse check run: %w", err)
	}

	// Combine output fields for context
	var logs strings.Builder
	if checkRun.Output.Title != "" {
		logs.WriteString("Title: ")
		logs.WriteString(checkRun.Output.Title)
		logs.WriteString("\n\n")
	}
	if checkRun.Output.Summary != "" {
		logs.WriteString("Summary:\n")
		logs.WriteString(checkRun.Output.Summary)
		logs.WriteString("\n\n")
	}
	if checkRun.Output.Text != "" {
		logs.WriteString("Details:\n")
		logs.WriteString(checkRun.Output.Text)
	}

	// If no output available, provide basic info
	if logs.Len() == 0 {
		logs.WriteString(fmt.Sprintf("Check concluded with: %s\n", checkRun.Conclusion))
		logs.WriteString(fmt.Sprintf("Details: %s\n", checkRun.HTMLURL))
	}

	return logs.String(), nil
}
