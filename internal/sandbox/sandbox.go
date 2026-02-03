package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Sandbox represents an isolated working directory for an issue
type Sandbox struct {
	Root       string
	RepoDir    string
	IssueID    string
	BranchName string
}

// Create creates a new sandbox for processing an issue
func Create(baseDir string, repo string, issueID string) (*Sandbox, error) {
	// Create unique directory for this issue
	sandboxDir := filepath.Join(baseDir, fmt.Sprintf("issue-%s", issueID))

	if err := os.MkdirAll(sandboxDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sandbox directory: %w", err)
	}

	return &Sandbox{
		Root:    sandboxDir,
		RepoDir: filepath.Join(sandboxDir, "repo"),
		IssueID: issueID,
	}, nil
}

// Clone clones the repository into the sandbox
func (s *Sandbox) Clone(ctx context.Context, cloneURL string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", cloneURL, s.RepoDir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone repository: %w: %s", err, string(output))
	}
	return nil
}

// CreateBranch creates and checks out a new branch, or checks out existing one
func (s *Sandbox) CreateBranch(ctx context.Context, branchName string) error {
	s.BranchName = branchName

	// Try to create new branch
	cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
	cmd.Dir = s.RepoDir
	if _, err := cmd.CombinedOutput(); err != nil {
		// Branch might already exist, try checking it out
		cmd2 := exec.CommandContext(ctx, "git", "checkout", branchName)
		cmd2.Dir = s.RepoDir
		if output, err := cmd2.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to checkout branch: %w: %s", err, string(output))
		}
	}
	return nil
}

// Commit stages all changes and creates a commit
func (s *Sandbox) Commit(ctx context.Context, message string) error {
	// Check if there are changes before staging
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = s.RepoDir
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check status: %w", err)
	}

	if len(statusOutput) == 0 {
		// No changes to commit
		return nil
	}

	// Stage all changes
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = s.RepoDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage changes: %w: %s", err, string(output))
	}

	// Commit
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = s.RepoDir
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit: %w: %s", err, string(output))
	}

	return nil
}

// Push pushes the branch to origin
func (s *Sandbox) Push(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git", "push", "-u", "origin", s.BranchName)
	cmd.Dir = s.RepoDir
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push: %w: %s", err, string(output))
	}
	return nil
}

// GetCurrentBranch returns the current branch name
func (s *Sandbox) GetCurrentBranch(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "branch", "--show-current")
	cmd.Dir = s.RepoDir
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// HasChanges checks if there are uncommitted changes
func (s *Sandbox) HasChanges(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = s.RepoDir
	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check status: %w", err)
	}
	return len(output) > 0, nil
}

// Cleanup removes the sandbox directory
func (s *Sandbox) Cleanup() error {
	return os.RemoveAll(s.Root)
}

// Exists checks if the sandbox exists
func (s *Sandbox) Exists() bool {
	_, err := os.Stat(s.RepoDir)
	return err == nil
}

// RepoPath returns the full path to a file in the repo
func (s *Sandbox) RepoPath(relativePath string) string {
	return filepath.Join(s.RepoDir, relativePath)
}

// Manager handles sandbox lifecycle
type Manager struct {
	baseDir string
}

// NewManager creates a sandbox manager
func NewManager(baseDir string) *Manager {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	return &Manager{baseDir: filepath.Join(baseDir, "ultra-engineer-sandboxes")}
}

// GetOrCreate gets an existing sandbox or creates a new one
func (m *Manager) GetOrCreate(repo string, issueID string) (*Sandbox, error) {
	sandbox := &Sandbox{
		Root:    filepath.Join(m.baseDir, fmt.Sprintf("issue-%s", issueID)),
		RepoDir: filepath.Join(m.baseDir, fmt.Sprintf("issue-%s", issueID), "repo"),
		IssueID: issueID,
	}

	if sandbox.Exists() {
		return sandbox, nil
	}

	return Create(m.baseDir, repo, issueID)
}

// Get gets an existing sandbox
func (m *Manager) Get(issueID string) *Sandbox {
	return &Sandbox{
		Root:    filepath.Join(m.baseDir, fmt.Sprintf("issue-%s", issueID)),
		RepoDir: filepath.Join(m.baseDir, fmt.Sprintf("issue-%s", issueID), "repo"),
		IssueID: issueID,
	}
}

// CleanupAll removes all sandboxes
func (m *Manager) CleanupAll() error {
	return os.RemoveAll(m.baseDir)
}
