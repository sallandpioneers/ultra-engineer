package workflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
)

// CIMonitor handles CI status polling and fix orchestration
type CIMonitor struct {
	provider     providers.CIProvider
	pollInterval time.Duration
	timeout      time.Duration
}

// NewCIMonitor creates a new CI monitor
func NewCIMonitor(provider providers.CIProvider, pollInterval, timeout time.Duration) *CIMonitor {
	return &CIMonitor{
		provider:     provider,
		pollInterval: pollInterval,
		timeout:      timeout,
	}
}

// CIWaitResult represents the outcome of waiting for CI
type CIWaitResult struct {
	Status       providers.CIStatus
	FailedChecks []providers.CICheck
	TimedOut     bool
}

// WaitForCI polls CI status until completion or timeout
func (m *CIMonitor) WaitForCI(ctx context.Context, repo string, prNumber int) (*CIWaitResult, error) {
	deadline := time.Now().Add(m.timeout)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Check immediately on first call, then poll on ticker
	checkNow := true

	for {
		if !checkNow {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-ticker.C:
				// Continue to check
			}
		}
		checkNow = false

		if time.Now().After(deadline) {
			return &CIWaitResult{
				Status:   providers.CIStatusUnknown,
				TimedOut: true,
			}, nil
		}

		result, err := m.provider.GetCIStatus(ctx, repo, prNumber)
		if err != nil {
			// Log but continue polling on transient errors
			continue
		}

		switch result.OverallStatus {
		case providers.CIStatusSuccess:
			return &CIWaitResult{
				Status: providers.CIStatusSuccess,
			}, nil

		case providers.CIStatusFailure:
			// Collect failed checks
			var failed []providers.CICheck
			for _, check := range result.Checks {
				if check.Status == providers.CIStatusFailure {
					failed = append(failed, check)
				}
			}
			return &CIWaitResult{
				Status:       providers.CIStatusFailure,
				FailedChecks: failed,
			}, nil

		case providers.CIStatusPending:
			// Continue polling
			continue

		case providers.CIStatusUnknown:
			// No CI configured, treat as success
			return &CIWaitResult{
				Status: providers.CIStatusSuccess,
			}, nil
		}
	}
}

// GetFailureLogs retrieves and combines logs for failed checks
func (m *CIMonitor) GetFailureLogs(ctx context.Context, repo string, checks []providers.CICheck) (string, error) {
	var logs strings.Builder

	for i, check := range checks {
		if i > 0 {
			logs.WriteString("\n\n---\n\n")
		}

		logs.WriteString(fmt.Sprintf("## %s\n\n", check.Name))

		// Try to get detailed logs
		if check.ID != 0 {
			logContent, err := m.provider.GetCILogs(ctx, repo, check.ID)
			if err == nil && logContent != "" {
				logs.WriteString(logContent)
				continue
			}
		}

		// Fall back to the output field if logs not available
		if check.Output != "" {
			logs.WriteString(check.Output)
		} else if check.DetailsURL != "" {
			logs.WriteString(fmt.Sprintf("Details: %s\n", check.DetailsURL))
		} else {
			logs.WriteString(fmt.Sprintf("Check failed with conclusion: %s\n", check.Conclusion))
		}
	}

	return logs.String(), nil
}

// CheckCI performs a single CI status check (non-blocking)
func (m *CIMonitor) CheckCI(ctx context.Context, repo string, prNumber int) (*providers.CIResult, error) {
	return m.provider.GetCIStatus(ctx, repo, prNumber)
}
