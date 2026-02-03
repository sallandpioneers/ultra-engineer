package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GiteaProvider implements Provider using the tea CLI
type GiteaProvider struct {
	url   string
	token string
}

// NewGiteaProvider creates a new Gitea provider
func NewGiteaProvider(url, token string) *GiteaProvider {
	return &GiteaProvider{url: url, token: token}
}

func (g *GiteaProvider) Name() string {
	return "gitea"
}

// teaCmd creates a tea command with common flags
func (g *GiteaProvider) teaCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "tea", args...)
	return cmd
}

// runTea executes a tea command and returns stdout
func (g *GiteaProvider) runTea(ctx context.Context, args ...string) ([]byte, error) {
	cmd := g.teaCmd(ctx, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("tea command failed: %s: %s", err, string(exitErr.Stderr))
		}
		return nil, err
	}
	return out, nil
}

// teaIssue represents tea's JSON output for issues
type teaIssue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Author    teaUser   `json:"user"`
	Labels    []teaLabel `json:"labels"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type teaUser struct {
	Login string `json:"login"`
}

type teaLabel struct {
	Name string `json:"name"`
}

type teaComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	User      teaUser   `json:"user"`
	CreatedAt time.Time `json:"created_at"`
}

type teaPR struct {
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

func (g *GiteaProvider) GetIssue(ctx context.Context, repo string, number int) (*Issue, error) {
	out, err := g.runTea(ctx, "issues", "view", strconv.Itoa(number), "--repo", repo, "--output", "json")
	if err != nil {
		return nil, err
	}

	var ti teaIssue
	if err := json.Unmarshal(out, &ti); err != nil {
		return nil, fmt.Errorf("failed to parse issue: %w", err)
	}

	labels := make([]string, len(ti.Labels))
	for i, l := range ti.Labels {
		labels[i] = l.Name
	}

	return &Issue{
		Number:    ti.Number,
		Title:     ti.Title,
		Body:      ti.Body,
		Labels:    labels,
		State:     ti.State,
		Author:    ti.Author.Login,
		CreatedAt: ti.CreatedAt,
		UpdatedAt: ti.UpdatedAt,
	}, nil
}

func (g *GiteaProvider) ListIssuesWithLabel(ctx context.Context, repo string, label string) ([]*Issue, error) {
	out, err := g.runTea(ctx, "issues", "list", "--repo", repo, "--labels", label, "--state", "open", "--output", "json")
	if err != nil {
		return nil, err
	}

	var issues []teaIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("failed to parse issues: %w", err)
	}

	result := make([]*Issue, len(issues))
	for i, ti := range issues {
		labels := make([]string, len(ti.Labels))
		for j, l := range ti.Labels {
			labels[j] = l.Name
		}
		result[i] = &Issue{
			Number:    ti.Number,
			Title:     ti.Title,
			Body:      ti.Body,
			Labels:    labels,
			State:     ti.State,
			Author:    ti.Author.Login,
			CreatedAt: ti.CreatedAt,
			UpdatedAt: ti.UpdatedAt,
		}
	}

	return result, nil
}

func (g *GiteaProvider) GetComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	out, err := g.runTea(ctx, "issues", "comments", strconv.Itoa(number), "--repo", repo, "--output", "json")
	if err != nil {
		return nil, err
	}

	var comments []teaComment
	if err := json.Unmarshal(out, &comments); err != nil {
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
	_, err := g.runTea(ctx, "issues", "comment", strconv.Itoa(number), "--repo", repo, "--body", body)
	return err
}

func (g *GiteaProvider) UpdateIssueBody(ctx context.Context, repo string, number int, body string) error {
	_, err := g.runTea(ctx, "issues", "edit", strconv.Itoa(number), "--repo", repo, "--body", body)
	return err
}

func (g *GiteaProvider) AddLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := g.runTea(ctx, "issues", "labels", "add", strconv.Itoa(number), "--repo", repo, label)
	return err
}

func (g *GiteaProvider) RemoveLabel(ctx context.Context, repo string, number int, label string) error {
	_, err := g.runTea(ctx, "issues", "labels", "remove", strconv.Itoa(number), "--repo", repo, label)
	return err
}

func (g *GiteaProvider) CreatePR(ctx context.Context, repo string, pr PRCreate) (*PR, error) {
	args := []string{"pulls", "create", "--repo", repo, "--title", pr.Title, "--body", pr.Body, "--head", pr.Head, "--base", pr.Base, "--output", "json"}
	out, err := g.runTea(ctx, args...)
	if err != nil {
		return nil, err
	}

	var tp teaPR
	if err := json.Unmarshal(out, &tp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    tp.Number,
		Title:     tp.Title,
		Body:      tp.Body,
		State:     tp.State,
		Mergeable: tp.Mergeable,
		HTMLURL:   tp.HTMLURL,
		HeadRef:   tp.Head.Ref,
		BaseRef:   tp.Base.Ref,
	}, nil
}

func (g *GiteaProvider) GetPR(ctx context.Context, repo string, number int) (*PR, error) {
	out, err := g.runTea(ctx, "pulls", "view", strconv.Itoa(number), "--repo", repo, "--output", "json")
	if err != nil {
		return nil, err
	}

	var tp teaPR
	if err := json.Unmarshal(out, &tp); err != nil {
		return nil, fmt.Errorf("failed to parse PR: %w", err)
	}

	return &PR{
		Number:    tp.Number,
		Title:     tp.Title,
		Body:      tp.Body,
		State:     tp.State,
		Mergeable: tp.Mergeable,
		HTMLURL:   tp.HTMLURL,
		HeadRef:   tp.Head.Ref,
		BaseRef:   tp.Base.Ref,
	}, nil
}

func (g *GiteaProvider) GetPRComments(ctx context.Context, repo string, number int) ([]*Comment, error) {
	out, err := g.runTea(ctx, "pulls", "comments", strconv.Itoa(number), "--repo", repo, "--output", "json")
	if err != nil {
		return nil, err
	}

	var comments []teaComment
	if err := json.Unmarshal(out, &comments); err != nil {
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
	_, err := g.runTea(ctx, "pulls", "merge", strconv.Itoa(number), "--repo", repo)
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
	// tea doesn't have a clone command, use git directly
	// Construct the clone URL from the repo
	cloneURL := fmt.Sprintf("%s/%s.git", strings.TrimSuffix(g.url, "/"), repo)
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, dest)
	return cmd.Run()
}

func (g *GiteaProvider) GetDefaultBranch(ctx context.Context, repo string) (string, error) {
	out, err := g.runTea(ctx, "repos", "view", repo, "--output", "json")
	if err != nil {
		return "", err
	}

	var repoInfo struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.Unmarshal(out, &repoInfo); err != nil {
		return "", fmt.Errorf("failed to parse repo info: %w", err)
	}

	if repoInfo.DefaultBranch == "" {
		return "main", nil
	}
	return repoInfo.DefaultBranch, nil
}
