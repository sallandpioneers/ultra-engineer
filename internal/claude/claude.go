package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// Client wraps the Claude Code CLI
type Client struct {
	command string
	timeout time.Duration
}

// NewClient creates a new Claude Code client
func NewClient(command string, timeout time.Duration) *Client {
	return &Client{
		command: command,
		timeout: timeout,
	}
}

// JSONResponse represents the JSON output from Claude Code
type JSONResponse struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	Result    string `json:"result"`
	Error     string `json:"error,omitempty"`
	CostUSD   float64 `json:"cost_usd"`
}

// RunOptions configures a Claude Code run
type RunOptions struct {
	WorkDir      string
	SessionID    string
	Prompt       string
	AllowedTools []string // Tools to allow without prompting
}

// Run executes Claude Code with the given prompt
func (c *Client) Run(ctx context.Context, opts RunOptions) (string, error) {
	result, _, err := c.RunInteractive(ctx, opts)
	return result, err
}

// RunInteractive runs Claude in a way that allows it to use tools
// and waits for it to complete its task
func (c *Client) RunInteractive(ctx context.Context, opts RunOptions) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{
		"-p", // print mode (non-interactive)
		"--output-format", "json",
	}

	if opts.SessionID != "" {
		args = append(args, "--resume", opts.SessionID)
	}

	for _, tool := range opts.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	// Prompt must be the last positional argument
	args = append(args, opts.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)
	cmd.Dir = opts.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Read all stdout
	stdoutBytes, err := io.ReadAll(stdout)
	if err != nil {
		return "", "", fmt.Errorf("failed to read stdout: %w", err)
	}

	// Read stderr for any errors
	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", fmt.Errorf("claude timed out after %v", c.timeout)
		}
		return "", "", fmt.Errorf("claude failed: %w: %s", err, string(stderrBytes))
	}

	// Parse JSON response
	var resp JSONResponse
	if err := json.Unmarshal(stdoutBytes, &resp); err != nil {
		// If not valid JSON, return raw output
		return string(stdoutBytes), "", nil
	}

	if resp.Error != "" {
		return "", resp.SessionID, fmt.Errorf("claude error: %s", resp.Error)
	}

	return resp.Result, resp.SessionID, nil
}

// IsRateLimited checks if an error indicates rate limiting
func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "too many requests")
}
