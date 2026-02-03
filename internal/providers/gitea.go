package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// GiteaProvider implements Provider using Gitea API directly
type GiteaProvider struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewGiteaProvider creates a new Gitea provider
func NewGiteaProvider(url, token string) *GiteaProvider {
	return &GiteaProvider{
		baseURL: strings.TrimSuffix(url, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GiteaProvider) Name() string {
	return "gitea"
}

// doRequest performs an HTTP request to the Gitea API
func (g *GiteaProvider) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
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
	path := fmt.Sprintf("/repos/%s/issues?state=open&labels=%s", repo, label)
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

func (g *GiteaProvider) CreateComment(ctx context.Context, repo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", repo, number)
	_, err := g.doRequest(ctx, "POST", path, map[string]string{"body": body})
	return err
}

func (g *GiteaProvider) UpdateIssueBody(ctx context.Context, repo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d", repo, number)
	_, err := g.doRequest(ctx, "PATCH", path, map[string]string{"body": body})
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

func (g *GiteaProvider) MergePR(ctx context.Context, repo string, number int) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d/merge", repo, number)
	_, err := g.doRequest(ctx, "POST", path, map[string]string{
		"do": "merge",
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
		return fmt.Errorf("git clone failed: %w: %s", err, string(output))
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
