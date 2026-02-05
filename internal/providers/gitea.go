package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/retry"
)

// GiteaProvider implements Provider using Gitea API directly
type GiteaProvider struct {
	baseURL   string
	token     string
	client    *http.Client
	retryOpts *retry.Options
}

// NewGiteaProvider creates a new Gitea provider
func NewGiteaProvider(url, token string) *GiteaProvider {
	return &GiteaProvider{
		baseURL: strings.TrimSuffix(url, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewGiteaProviderWithRetry creates a new Gitea provider with retry support
func NewGiteaProviderWithRetry(url, token string, retryConfig config.RetryConfig) *GiteaProvider {
	opts := retry.DefaultOptions(retryConfig)
	opts.Classifier = retry.ClassifyHTTPError
	return &GiteaProvider{
		baseURL:   strings.TrimSuffix(url, "/"),
		token:     token,
		client:    &http.Client{Timeout: 30 * time.Second},
		retryOpts: &opts,
	}
}

func (g *GiteaProvider) Name() string {
	return "gitea"
}

// doRequest performs an HTTP request to the Gitea API
func (g *GiteaProvider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	// If retry is configured, use retry logic
	if g.retryOpts != nil {
		return retry.DoWithResult(ctx, *g.retryOpts, func() ([]byte, error) {
			return g.doRequestOnce(ctx, method, path, body)
		})
	}
	return g.doRequestOnce(ctx, method, path, body)
}

// doRequestOnce performs a single HTTP request to the Gitea API
func (g *GiteaProvider) doRequestOnce(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	url := g.baseURL + "/api/v1" + path

	var reqBody io.Reader
	if body != nil {
		jsonBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Gitea API structs
type giteaIssue struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Body      string       `json:"body"`
	State     string       `json:"state"`
	User      giteaUser    `json:"user"`
	Labels    []giteaLabel `json:"labels"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type giteaUser struct {
	Login string `json:"login"`
}

type giteaLabel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type giteaComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      giteaUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

type giteaPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	Mergeable bool   `json:"mergeable"`
	HTMLURL   string `json:"html_url"`
	Head      struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
}

type giteaRepo struct {
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
}

func (g *GiteaProvider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	path := fmt.Sprintf("/repos/%s/issues/%d", repo, number)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var gi giteaIssue
	if err := json.Unmarshal(data, &gi); err != nil {
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
		Author:    gi.User.Login,
		CreatedAt: gi.CreatedAt,
		UpdatedAt: gi.UpdatedAt,
	}, nil
}

func (g *GiteaProvider) ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error) {
	path := fmt.Sprintf("/repos/%s/issues?state=open&labels=%s", repo, url.QueryEscape(label))
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var issues []giteaIssue
	if err := json.Unmarshal(data, &issues); err != nil {
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
			Author:    gi.User.Login,
			CreatedAt: gi.CreatedAt,
			UpdatedAt: gi.UpdatedAt,
		}
	}

	return result, nil
}

func (g *GiteaProvider) GetComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var comments []giteaComment
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
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

func (g *GiteaProvider) CreateComment(ctx context.Context, repo string, number int, body string) (int64, error) {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	data, err := g.doRequest(ctx, "POST", path, map[string]string{"body": body})
	if err != nil {
		return 0, err
	}

	var comment giteaComment
	if err := json.Unmarshal(data, &comment); err != nil {
		return 0, fmt.Errorf("failed to parse comment response: %w", err)
	}
	return comment.ID, nil
}

func (g *GiteaProvider) UpdateComment(ctx context.Context, repo string, commentID int64, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/comments/%d", repo, commentID)
	_, err := g.doRequest(ctx, "PATCH", path, map[string]string{"body": body})
	return err
}

func (g *GiteaProvider) UpdateIssueBody(ctx context.Context, repo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d", repo, number)
	_, err := g.doRequest(ctx, "PATCH", path, map[string]string{"body": body})
	return err
}

func (g *GiteaProvider) ReactToComment(ctx context.Context, repo string, commentID int64, reaction string) error {
	path := fmt.Sprintf("/repos/%s/issues/comments/%d/reactions", repo, commentID)
	_, err := g.doRequest(ctx, "POST", path, map[string]string{"content": reaction})
	return err
}

func (g *GiteaProvider) AddLabel(ctx context.Context, repo string, number int, label string) error {
	// First get the label ID
	labelID, err := g.getLabelID(ctx, repo, label)
	if err != nil {
		// Try to create the label
		labelID, err = g.createLabel(ctx, repo, label)
		if err != nil {
			return fmt.Errorf("failed to get or create label: %w", err)
		}
	}

	path := fmt.Sprintf("/repos/%s/issues/%d/labels", repo, number)
	_, err = g.doRequest(ctx, "POST", path, map[string][]int64{"labels": {labelID}})
	return err
}

func (g *GiteaProvider) getLabelID(ctx context.Context, repo string, labelName string) (int64, error) {
	path := fmt.Sprintf("/repos/%s/labels", repo)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return 0, err
	}

	var labels []giteaLabel
	if err := json.Unmarshal(data, &labels); err != nil {
		return 0, err
	}

	for _, l := range labels {
		if l.Name == labelName {
			return l.ID, nil
		}
	}

	return 0, fmt.Errorf("label not found: %s", labelName)
}

func (g *GiteaProvider) createLabel(ctx context.Context, repo string, labelName string) (int64, error) {
	path := fmt.Sprintf("/repos/%s/labels", repo)
	data, err := g.doRequest(ctx, "POST", path, map[string]string{
		"name":  labelName,
		"color": "#0052cc",
	})
	if err != nil {
		return 0, err
	}

	var label giteaLabel
	if err := json.Unmarshal(data, &label); err != nil {
		return 0, err
	}

	return label.ID, nil
}

func (g *GiteaProvider) RemoveLabel(ctx context.Context, repo string, number int, label string) error {
	labelID, err := g.getLabelID(ctx, repo, label)
	if err != nil {
		return nil // Label doesn't exist, nothing to remove
	}

	path := fmt.Sprintf("/repos/%s/issues/%d/labels/%d", repo, number, labelID)
	_, err = g.doRequest(ctx, "DELETE", path, nil)
	return err
}

func (g *GiteaProvider) CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error) {
	path := fmt.Sprintf("/repos/%s/pulls", repo)
	data, err := g.doRequest(ctx, "POST", path, map[string]interface{}{
		"title": pr.Title,
		"body":  pr.Body,
		"head":  pr.Head,
		"base":  pr.Base,
	})
	if err != nil {
		return nil, err
	}

	var gp giteaPR
	if err := json.Unmarshal(data, &gp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    gp.Number,
		Title:     gp.Title,
		Body:      gp.Body,
		State:     gp.State,
		Mergeable: gp.Mergeable,
		HTMLURL:   gp.HTMLURL,
		HeadRef:   gp.Head.Ref,
		BaseRef:   gp.Base.Ref,
	}, nil
}

func (g *GiteaProvider) GetPR(ctx context.Context, repo string, number int) (*PR, error) {
	path := fmt.Sprintf("/repos/%s/pulls/%d", repo, number)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var gp giteaPR
	if err := json.Unmarshal(data, &gp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    gp.Number,
		Title:     gp.Title,
		Body:      gp.Body,
		State:     gp.State,
		Mergeable: gp.Mergeable,
		HTMLURL:   gp.HTMLURL,
		HeadRef:   gp.Head.Ref,
		BaseRef:   gp.Base.Ref,
	}, nil
}

func (g *GiteaProvider) GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	// Gitea uses the same endpoint for PR comments as issue comments
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var comments []giteaComment
	if err := json.Unmarshal(data, &comments); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
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

// giteaReview represents a review from Gitea's API
type giteaReview struct {
	ID int64 `json:"id"`
}

// giteaReviewComment represents a review comment from Gitea's API
type giteaReviewComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      giteaUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

func (g *GiteaProvider) GetPRReviewComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	// Gitea's API structure: first list all reviews, then fetch comments from each review
	// Endpoint: /repos/{owner}/{repo}/pulls/{index}/reviews
	reviewsPath := fmt.Sprintf("/repos/%s/pulls/%d/reviews", repo, number)
	reviewsData, err := g.doRequest(ctx, "GET", reviewsPath, nil)
	if err != nil {
		return nil, err
	}

	var reviews []giteaReview
	if err := json.Unmarshal(reviewsData, &reviews); err != nil {
		return nil, fmt.Errorf("failed to parse reviews: %w", err)
	}

	// Fetch comments from each review
	var allComments []*Comment
	for _, review := range reviews {
		commentsPath := fmt.Sprintf("/repos/%s/pulls/%d/reviews/%d/comments", repo, number, review.ID)
		commentsData, err := g.doRequest(ctx, "GET", commentsPath, nil)
		if err != nil {
			// Log error but continue to process other reviews
			continue
		}

		var reviewComments []giteaReviewComment
		if err := json.Unmarshal(commentsData, &reviewComments); err != nil {
			continue
		}

		for _, rc := range reviewComments {
			allComments = append(allComments, &Comment{
				ID:        rc.ID,
				Body:      rc.Body,
				Author:    rc.User.Login,
				CreatedAt: rc.CreatedAt,
			})
		}
	}

	return allComments, nil
}

func (g *GiteaProvider) MergePR(ctx context.Context, repo string, number int) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d/merge", repo, number)
	_, err := g.doRequest(ctx, "POST", path, map[string]string{
		"do": "squash", // Use squash to avoid duplicate commits
	})
	return err
}

func (g *GiteaProvider) IsMergeable(ctx context.Context, repo string, number int) (bool, error) {
	pr, err := g.GetPR(ctx, repo, number)
	if err != nil {
		return false, err
	}
	return pr.Mergeable, nil
}

func (g *GiteaProvider) Clone(ctx context.Context, repo string, dest string) error {
	// Get repo info to get clone URL
	path := fmt.Sprintf("/repos/%s", repo)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return err
	}

	var repoInfo giteaRepo
	if err := json.Unmarshal(data, &repoInfo); err != nil {
		return fmt.Errorf("failed to parse repo info: %w", err)
	}

	// Inject token into clone URL for authentication
	cloneURL := repoInfo.CloneURL
	cloneURL = strings.Replace(cloneURL, "https://", fmt.Sprintf("https://oauth2:%s@", g.token), 1)

	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, dest)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Sanitize output to remove any token that might be in error messages
		sanitizedOutput := strings.ReplaceAll(string(output), g.token, "[REDACTED]")
		return fmt.Errorf("git clone failed: %w: %s", err, sanitizedOutput)
	}
	return nil
}

func (g *GiteaProvider) GetDefaultBranch(ctx context.Context, repo string) (string, error) {
	path := fmt.Sprintf("/repos/%s", repo)
	data, err := g.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return "", err
	}

	var repoInfo giteaRepo
	if err := json.Unmarshal(data, &repoInfo); err != nil {
		return "", fmt.Errorf("failed to parse repo info: %w", err)
	}

	if repoInfo.DefaultBranch == "" {
		return "main", nil
	}
	return repoInfo.DefaultBranch, nil
}

// giteaCommitStatus represents a commit status from Gitea's API
type giteaCommitStatus struct {
	ID          int64  `json:"id"`
	State       string `json:"status"` // pending, success, error, failure, warning
	Context     string `json:"context"`
	Description string `json:"description"`
	TargetURL   string `json:"target_url"`
}

// giteaCombinedStatus represents combined commit status from Gitea
type giteaCombinedStatus struct {
	State    string              `json:"state"` // pending, success, error, failure
	Statuses []giteaCommitStatus `json:"statuses"`
}

// giteaActionRun represents a Gitea Actions workflow run
type giteaActionRun struct {
	ID         int64  `json:"id"`
	WorkflowID string `json:"workflow_id"`
	Status     string `json:"status"`     // waiting, running, completed
	Conclusion string `json:"conclusion"` // success, failure, cancelled, etc.
	HTMLURL    string `json:"html_url"`
}

// GetCIStatus implements CIProvider for Gitea
func (g *GiteaProvider) GetCIStatus(ctx context.Context, repo string, prNumber int) (*CIResult, error) {
	// Get the PR details including head SHA
	prPath := fmt.Sprintf("/repos/%s/pulls/%d", repo, prNumber)
	prData, err := g.doRequest(ctx, "GET", prPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR: %w", err)
	}

	var prDetails struct {
		Head struct {
			SHA string `json:"sha"`
			Ref string `json:"ref"`
		} `json:"head"`
	}
	if err := json.Unmarshal(prData, &prDetails); err != nil {
		return nil, fmt.Errorf("failed to parse PR details: %w", err)
	}

	// Get combined commit status
	statusPath := fmt.Sprintf("/repos/%s/commits/%s/status", repo, prDetails.Head.SHA)
	statusData, err := g.doRequest(ctx, "GET", statusPath, nil)
	if err != nil {
		// If no status found, return unknown
		return &CIResult{
			OverallStatus: CIStatusUnknown,
			Checks:        []CICheck{},
		}, nil
	}

	var combined giteaCombinedStatus
	if err := json.Unmarshal(statusData, &combined); err != nil {
		return nil, fmt.Errorf("failed to parse commit status: %w", err)
	}

	result := &CIResult{
		Checks: make([]CICheck, 0, len(combined.Statuses)),
	}

	for _, s := range combined.Statuses {
		check := CICheck{
			ID:         s.ID,
			Name:       s.Context,
			Conclusion: s.State,
			DetailsURL: s.TargetURL,
			Output:     s.Description,
		}

		switch strings.ToLower(s.State) {
		case "pending":
			check.Status = CIStatusPending
		case "success":
			check.Status = CIStatusSuccess
		case "error", "failure":
			check.Status = CIStatusFailure
		default:
			check.Status = CIStatusUnknown
		}

		result.Checks = append(result.Checks, check)
	}

	// Map overall state
	switch strings.ToLower(combined.State) {
	case "pending":
		result.OverallStatus = CIStatusPending
	case "success":
		result.OverallStatus = CIStatusSuccess
	case "error", "failure":
		result.OverallStatus = CIStatusFailure
	default:
		if len(result.Checks) == 0 {
			result.OverallStatus = CIStatusUnknown
		} else {
			result.OverallStatus = CIStatusPending
		}
	}

	// Also try to get Gitea Actions runs if available
	g.enrichWithActionRuns(ctx, repo, prDetails.Head.Ref, result)

	return result, nil
}

// enrichWithActionRuns adds Gitea Actions runs to the CI result
func (g *GiteaProvider) enrichWithActionRuns(ctx context.Context, repo, branch string, result *CIResult) {
	// Try to fetch Gitea Actions runs for the branch
	actionsPath := fmt.Sprintf("/repos/%s/actions/runs?branch=%s", repo, url.QueryEscape(branch))
	data, err := g.doRequest(ctx, "GET", actionsPath, nil)
	if err != nil {
		// Actions might not be enabled, silently skip
		return
	}

	var response struct {
		Runs []giteaActionRun `json:"workflow_runs"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return
	}

	for _, run := range response.Runs {
		check := CICheck{
			ID:         run.ID,
			Name:       run.WorkflowID,
			Conclusion: run.Conclusion,
			DetailsURL: run.HTMLURL,
		}

		switch strings.ToLower(run.Status) {
		case "waiting", "running", "queued":
			check.Status = CIStatusPending
			if result.OverallStatus == CIStatusSuccess {
				result.OverallStatus = CIStatusPending
			}
		case "completed":
			switch strings.ToLower(run.Conclusion) {
			case "success":
				check.Status = CIStatusSuccess
			case "failure", "timed_out":
				check.Status = CIStatusFailure
				result.OverallStatus = CIStatusFailure
			case "cancelled", "skipped":
				check.Status = CIStatusSuccess // Non-blocking
			default:
				check.Status = CIStatusUnknown
			}
		}

		result.Checks = append(result.Checks, check)
	}
}

// IsCollaborator checks if a user is a collaborator on the repository
func (g *GiteaProvider) IsCollaborator(ctx context.Context, repo, username string) (bool, error) {
	// If retry is configured, use retry logic
	if g.retryOpts != nil {
		return retry.DoWithResult(ctx, *g.retryOpts, func() (bool, error) {
			return g.checkCollaboratorOnce(ctx, repo, username)
		})
	}
	return g.checkCollaboratorOnce(ctx, repo, username)
}

// checkCollaboratorOnce checks collaborator status with custom 200/404 handling
func (g *GiteaProvider) checkCollaboratorOnce(ctx context.Context, repo, username string) (bool, error) {
	url := g.baseURL + "/api/v1/repos/" + repo + "/collaborators/" + username

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+g.token)
	req.Header.Set("Accept", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		// 200/204 = user IS a collaborator
		return true, nil
	case http.StatusNotFound:
		// 404 = user is NOT a collaborator (expected, not an error)
		return false, nil
	default:
		// Other errors (403, 500, etc.) - return error
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}
}

// GetCILogs retrieves logs for a Gitea CI run
func (g *GiteaProvider) GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error) {
	// Try Gitea Actions logs first
	logsPath := fmt.Sprintf("/repos/%s/actions/runs/%d/logs", repo, checkRunID)
	data, err := g.doRequest(ctx, "GET", logsPath, nil)
	if err == nil {
		return string(data), nil
	}

	// If Actions logs not available, return a message directing to the URL
	return fmt.Sprintf("Logs not directly available. Check run ID: %d\nView details in Gitea web interface.", checkRunID), nil
}
