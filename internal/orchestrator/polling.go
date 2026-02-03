package orchestrator

import (
	"context"
	"log"
	"time"

	"github.com/anthropics/ultra-engineer/internal/claude"
	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// Daemon runs the polling loop
type Daemon struct {
	config       *config.Config
	provider     providers.Provider
	orchestrator *Orchestrator
	logger       *log.Logger

	// Currently processing issue (one at a time for now)
	currentRepo  string
	currentIssue int
}

// NewDaemon creates a new daemon
func NewDaemon(cfg *config.Config, provider providers.Provider, logger *log.Logger) *Daemon {
	return &Daemon{
		config:       cfg,
		provider:     provider,
		orchestrator: New(cfg, provider, logger),
		logger:       logger,
	}
}

// Run starts the daemon polling loop
func (d *Daemon) Run(ctx context.Context, repo string) error {
	d.logger.Printf("Starting daemon for repo: %s", repo)
	d.logger.Printf("Polling interval: %s", d.config.PollInterval)
	d.logger.Printf("Trigger label: %s", d.config.TriggerLabel)

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	if err := d.poll(ctx, repo); err != nil {
		d.logger.Printf("Poll error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			d.logger.Printf("Daemon shutting down...")
			return ctx.Err()
		case <-ticker.C:
			if err := d.poll(ctx, repo); err != nil {
				d.logger.Printf("Poll error: %v", err)
			}
		}
	}
}

// poll checks for issues to process
func (d *Daemon) poll(ctx context.Context, repo string) error {
	// If currently processing an issue, continue with it
	if d.currentIssue != 0 {
		return d.continueCurrentIssue(ctx)
	}

	// Look for new issues with trigger label
	issues, err := d.provider.ListIssuesWithLabel(ctx, repo, d.config.TriggerLabel)
	if err != nil {
		return err
	}

	if len(issues) == 0 {
		return nil
	}

	// Find the first issue that needs processing
	for _, issue := range issues {
		// Check if issue is in a waiting phase
		phase := state.ParsePhaseFromLabels(issue.Labels)

		switch phase {
		case state.PhaseCompleted, state.PhaseFailed:
			// Skip completed/failed issues
			continue
		case state.PhaseQuestions, state.PhaseApproval, state.PhaseReview:
			// These need to check for user input
			d.currentRepo = repo
			d.currentIssue = issue.Number
			return d.continueCurrentIssue(ctx)
		default:
			// New issue or processing phase
			d.currentRepo = repo
			d.currentIssue = issue.Number
			return d.processIssue(ctx, issue)
		}
	}

	return nil
}

// continueCurrentIssue continues processing the current issue
func (d *Daemon) continueCurrentIssue(ctx context.Context) error {
	issue, err := d.provider.GetIssue(ctx, d.currentRepo, d.currentIssue)
	if err != nil {
		d.currentIssue = 0
		return err
	}

	err = d.processIssue(ctx, issue)

	// Re-fetch the issue to get updated labels after processing
	updatedIssue, fetchErr := d.provider.GetIssue(ctx, d.currentRepo, d.currentIssue)
	if fetchErr != nil {
		// If we can't fetch, check the original issue labels as fallback
		phase := state.ParsePhaseFromLabels(issue.Labels)
		if phase == state.PhaseCompleted || phase == state.PhaseFailed {
			d.currentIssue = 0
		}
		return err
	}

	// Check if we should stop processing this issue
	phase := state.ParsePhaseFromLabels(updatedIssue.Labels)
	if phase == state.PhaseCompleted || phase == state.PhaseFailed {
		d.currentIssue = 0
	}

	return err
}

// processIssue processes a single issue with retry logic
func (d *Daemon) processIssue(ctx context.Context, issue *providers.Issue) error {
	var lastErr error

	for attempt := 1; attempt <= d.config.Retry.MaxAttempts; attempt++ {
		err := d.orchestrator.ProcessIssue(ctx, d.currentRepo, issue)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check for rate limiting
		if claude.IsRateLimited(err) {
			d.logger.Printf("Rate limited, waiting %s before retry...", d.config.Retry.RateLimitRetry)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.config.Retry.RateLimitRetry):
				continue
			}
		}

		// Normal retry with backoff
		if attempt < d.config.Retry.MaxAttempts {
			backoff := d.config.Retry.BackoffBase * time.Duration(attempt)
			d.logger.Printf("Error processing issue (attempt %d/%d): %v, retrying in %s",
				attempt, d.config.Retry.MaxAttempts, err, backoff)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}
	}

	d.logger.Printf("Failed to process issue after %d attempts: %v", d.config.Retry.MaxAttempts, lastErr)
	d.currentIssue = 0
	return lastErr
}

// RunOnce processes a single issue once (for manual runs)
func (d *Daemon) RunOnce(ctx context.Context, repo string, issueNum int) error {
	issue, err := d.provider.GetIssue(ctx, repo, issueNum)
	if err != nil {
		return err
	}

	return d.orchestrator.ProcessIssue(ctx, repo, issue)
}
