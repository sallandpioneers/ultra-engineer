package claude

import (
	"bufio"
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

// Response represents a response from Claude Code
type Response struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// RunOptions configures a Claude Code run
type RunOptions struct {
	WorkDir     string
	SessionID   string
	Prompt      string
	AllowedTools []string // Tools to allow without prompting
}

// Run executes Claude Code with the given prompt
func (c *Client) Run(ctx context.Context, opts RunOptions) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{
		"--print",
		"--output-format", "stream-json",
	}

	if opts.SessionID != "" {
		args = append(args, "--resume", opts.SessionID)
	}

	for _, tool := range opts.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	args = append(args, "--prompt", opts.Prompt)

	cmd := exec.CommandContext(ctx, c.command, args...)
	cmd.Dir = opts.WorkDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start claude: %w", err)
	}

	// Read and parse streaming JSON output
	var result strings.Builder
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var resp Response
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			// Not JSON, might be raw output
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		switch resp.Type {
		case "assistant":
			result.WriteString(resp.Content)
		case "result":
			result.WriteString(resp.Content)
		case "error":
			return "", fmt.Errorf("claude error: %s", resp.Error)
		}
	}

	// Read stderr for any errors
	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("claude timed out after %v", c.timeout)
		}
		return "", fmt.Errorf("claude failed: %w: %s", err, string(stderrBytes))
	}

	return result.String(), nil
}

// RunInteractive runs Claude in a way that allows it to use tools
// and waits for it to complete its task
func (c *Client) RunInteractive(ctx context.Context, opts RunOptions) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
	}

	if opts.SessionID != "" {
		args = append(args, "--resume", opts.SessionID)
	}

	for _, tool := range opts.AllowedTools {
		args = append(args, "--allowedTools", tool)
	}

	args = append(args, "--prompt", opts.Prompt)

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

	// Read and parse streaming JSON output
	var result strings.Builder
	var sessionID string
	scanner := bufio.NewScanner(stdout)

	// Increase buffer size for large outputs
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var resp map[string]interface{}
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Extract session ID if present
		if sid, ok := resp["session_id"].(string); ok {
			sessionID = sid
		}

		// Handle different message types
		switch resp["type"] {
		case "assistant":
			if content, ok := resp["content"].(string); ok {
				result.WriteString(content)
			}
		case "result":
			if content, ok := resp["content"].(string); ok {
				result.WriteString(content)
			}
		case "error":
			if errMsg, ok := resp["error"].(string); ok {
				return "", sessionID, fmt.Errorf("claude error: %s", errMsg)
			}
		}
	}

	stderrBytes, _ := io.ReadAll(stderr)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", sessionID, fmt.Errorf("claude timed out after %v", c.timeout)
		}
		return "", sessionID, fmt.Errorf("claude failed: %w: %s", err, string(stderrBytes))
	}

	return result.String(), sessionID, nil
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
