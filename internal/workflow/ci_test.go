package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
)

// mockCIProvider implements providers.CIProvider for testing
type mockCIProvider struct {
	statusFunc func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error)
	logsFunc   func(ctx context.Context, repo string, checkRunID int64) (string, error)
}

func (m *mockCIProvider) GetCIStatus(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
	if m.statusFunc != nil {
		return m.statusFunc(ctx, repo, prNumber)
	}
	return &providers.CIResult{OverallStatus: providers.CIStatusSuccess}, nil
}

func (m *mockCIProvider) GetCILogs(ctx context.Context, repo string, checkRunID int64) (string, error) {
	if m.logsFunc != nil {
		return m.logsFunc(ctx, repo, checkRunID)
	}
	return "test logs", nil
}

func TestCIMonitor_WaitForCI_Success(t *testing.T) {
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			return &providers.CIResult{
				OverallStatus: providers.CIStatusSuccess,
				Checks: []providers.CICheck{
					{Name: "build", Status: providers.CIStatusSuccess},
				},
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	result, err := monitor.WaitForCI(ctx, "owner/repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != providers.CIStatusSuccess {
		t.Errorf("expected success, got %v", result.Status)
	}
	if result.TimedOut {
		t.Error("expected no timeout")
	}
}

func TestCIMonitor_WaitForCI_Failure(t *testing.T) {
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			return &providers.CIResult{
				OverallStatus: providers.CIStatusFailure,
				Checks: []providers.CICheck{
					{Name: "build", Status: providers.CIStatusSuccess},
					{Name: "test", Status: providers.CIStatusFailure, Conclusion: "failure"},
				},
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	result, err := monitor.WaitForCI(ctx, "owner/repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != providers.CIStatusFailure {
		t.Errorf("expected failure, got %v", result.Status)
	}
	if len(result.FailedChecks) != 1 {
		t.Errorf("expected 1 failed check, got %d", len(result.FailedChecks))
	}
	if result.FailedChecks[0].Name != "test" {
		t.Errorf("expected 'test' check, got %q", result.FailedChecks[0].Name)
	}
}

func TestCIMonitor_WaitForCI_Timeout(t *testing.T) {
	callCount := 0
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			callCount++
			return &providers.CIResult{
				OverallStatus: providers.CIStatusPending,
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 50*time.Millisecond)
	ctx := context.Background()

	result, err := monitor.WaitForCI(ctx, "owner/repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.TimedOut {
		t.Error("expected timeout")
	}
	if callCount == 0 {
		t.Error("expected at least one poll")
	}
}

func TestCIMonitor_WaitForCI_ContextCancellation(t *testing.T) {
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			return &providers.CIResult{
				OverallStatus: providers.CIStatusPending,
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	_, err := monitor.WaitForCI(ctx, "owner/repo", 123)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestCIMonitor_WaitForCI_Unknown(t *testing.T) {
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			return &providers.CIResult{
				OverallStatus: providers.CIStatusUnknown,
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	result, err := monitor.WaitForCI(ctx, "owner/repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Unknown status (no CI) should be treated as success
	if result.Status != providers.CIStatusSuccess {
		t.Errorf("expected success for unknown status, got %v", result.Status)
	}
}

func TestCIMonitor_GetFailureLogs(t *testing.T) {
	provider := &mockCIProvider{
		logsFunc: func(ctx context.Context, repo string, checkRunID int64) (string, error) {
			return "Error: test failed\nat line 42", nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	checks := []providers.CICheck{
		{ID: 1, Name: "test", Status: providers.CIStatusFailure},
		{ID: 2, Name: "lint", Status: providers.CIStatusFailure},
	}

	logs, err := monitor.GetFailureLogs(ctx, "owner/repo", checks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs == "" {
		t.Error("expected non-empty logs")
	}
	// Should contain both check names
	if !strings.Contains(logs, "test") || !strings.Contains(logs, "lint") {
		t.Errorf("logs should contain check names, got: %s", logs)
	}
}

func TestCIMonitor_GetFailureLogs_FallbackToOutput(t *testing.T) {
	provider := &mockCIProvider{
		logsFunc: func(ctx context.Context, repo string, checkRunID int64) (string, error) {
			return "", nil // Empty logs
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	checks := []providers.CICheck{
		{ID: 0, Name: "test", Status: providers.CIStatusFailure, Output: "Test output"},
	}

	logs, err := monitor.GetFailureLogs(ctx, "owner/repo", checks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(logs, "Test output") {
		t.Errorf("expected output fallback, got: %s", logs)
	}
}

func TestCIMonitor_CheckCI(t *testing.T) {
	provider := &mockCIProvider{
		statusFunc: func(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
			return &providers.CIResult{
				OverallStatus: providers.CIStatusPending,
				Checks: []providers.CICheck{
					{Name: "build", Status: providers.CIStatusPending},
				},
			}, nil
		},
	}

	monitor := NewCIMonitor(provider, 10*time.Millisecond, 1*time.Second)
	ctx := context.Background()

	result, err := monitor.CheckCI(ctx, "owner/repo", 123)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.OverallStatus != providers.CIStatusPending {
		t.Errorf("expected pending, got %v", result.OverallStatus)
	}
}
