package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
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

	// Concurrent processing
	workerPool         *WorkerPool
	depDetector        *DependencyDetector
	allStates          map[string]map[int]*state.State // repo -> issueNum -> state
	allStatesMu        sync.RWMutex
	claudeClient       *claude.Client

	// Legacy single-issue tracking (for RunOnce)
	currentRepo  string
	currentIssue int
}

// NewDaemon creates a new daemon
func NewDaemon(cfg *config.Config, provider providers.Provider, logger *log.Logger) *Daemon {
	claudeClient := claude.NewClient(cfg.Claude.Command, cfg.Claude.Timeout)

	return &Daemon{
		config:       cfg,
		provider:     provider,
		orchestrator: New(cfg, provider, logger),
		logger:       logger,
		claudeClient: claudeClient,
		allStates:    make(map[string]map[int]*state.State),
	}
}

// Run starts the daemon polling loop for multiple repositories
func (d *Daemon) Run(ctx context.Context, repos []string) error {
	d.logger.Printf("Starting daemon for repos: %v", repos)
	d.logger.Printf("Polling interval: %s", d.config.PollInterval)
	d.logger.Printf("Trigger label: %s", d.config.TriggerLabel)
	d.logger.Printf("Concurrency: max %d per repo, %d total", d.config.Concurrency.MaxPerRepo, d.config.Concurrency.MaxTotal)

	// Initialize worker pool
	d.workerPool = NewWorkerPool(ctx, d.config.Concurrency.MaxPerRepo, d.config.Concurrency.MaxTotal)
	d.workerPool.SetWorkerFunc(d.processJobWorker)
	d.workerPool.Start()

	// Initialize dependency detector
	d.depDetector = NewDependencyDetector(d.provider, d.claudeClient, d.config.Concurrency.DependencyDetection)

	ticker := time.NewTicker(d.config.PollInterval)
	defer ticker.Stop()

	// Initial poll
	if err := d.poll(ctx, repos); err != nil {
		d.logger.Printf("Poll error: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			d.logger.Printf("Daemon shutting down...")
			return d.Shutdown(ctx)
		case <-ticker.C:
			if err := d.poll(ctx, repos); err != nil {
				d.logger.Printf("Poll error: %v", err)
			}
		}
	}
}

// RunSingleRepo runs the daemon for a single repository (backwards compatible)
func (d *Daemon) RunSingleRepo(ctx context.Context, repo string) error {
	return d.Run(ctx, []string{repo})
}

// poll checks for issues to process across all repositories
func (d *Daemon) poll(ctx context.Context, repos []string) error {
	// 1. Drain results channel to process completed jobs first
	d.processCompletedJobs(ctx)

	// 2. Fetch all issues with trigger label across all configured repos
	allIssues := d.fetchTriggeredIssues(ctx, repos)

	// 3. Load state for each issue, filter out completed/failed
	pendingIssues := d.filterPendingIssues(ctx, allIssues)

	// 4. Detect dependencies for new issues
	d.detectDependencies(ctx, pendingIssues)

	// 5. Resolve dependencies, mark blocked issues
	readyIssues := d.resolveReadyIssues(ctx, pendingIssues)

	// 6. Respect per-repo limits when submitting to worker pool
	for _, issueInfo := range readyIssues {
		job := &Job{
			Issue:      issueInfo.issue,
			Repository: issueInfo.repo,
			State:      issueInfo.state,
		}
		if d.workerPool.TrySubmit(job) {
			d.logger.Printf("Submitted issue #%d from %s to worker pool", issueInfo.issue.Number, issueInfo.repo)
		}
	}

	// 7. Log status of all active/blocked issues
	d.reportStatus()

	return nil
}

// issueInfo holds issue data with repo context
type issueInfo struct {
	issue *providers.Issue
	repo  string
	state *state.State
}

// processCompletedJobs drains the results channel non-blocking
func (d *Daemon) processCompletedJobs(ctx context.Context) {
	if d.workerPool == nil {
		return
	}

	for {
		select {
		case result := <-d.workerPool.Results():
			d.workerPool.OnJobComplete(result.Job.Repository)

			if result.Error != nil {
				d.logger.Printf("Issue #%d failed: %v", result.Job.Issue.Number, result.Error)
			} else {
				d.logger.Printf("Issue #%d completed successfully", result.Job.Issue.Number)
			}

			// Trigger re-evaluation of blocked issues
			d.allStatesMu.RLock()
			repoStates := d.allStates[result.Job.Repository]
			d.allStatesMu.RUnlock()

			readyIssues, _ := d.orchestrator.CheckAndUnblockIssues(ctx, result.Job.Repository,
				result.Job.Issue.Number, repoStates)

			// Submit newly-ready issues to worker pool
			for _, issue := range readyIssues {
				d.allStatesMu.RLock()
				st := d.allStates[result.Job.Repository][issue.Number]
				d.allStatesMu.RUnlock()

				job := &Job{
					Issue:      issue,
					Repository: result.Job.Repository,
					State:      st,
				}
				if d.workerPool.TrySubmit(job) {
					d.logger.Printf("Unblocked issue #%d submitted to worker pool", issue.Number)
				}
			}
		default:
			return // No more results
		}
	}
}

// fetchTriggeredIssues fetches all issues with the trigger label from all repos
func (d *Daemon) fetchTriggeredIssues(ctx context.Context, repos []string) []issueInfo {
	var allIssues []issueInfo

	for _, repo := range repos {
		issues, err := d.provider.ListIssuesWithLabel(ctx, repo, d.config.TriggerLabel)
		if err != nil {
			d.logger.Printf("Error fetching issues from %s: %v", repo, err)
			continue
		}

		for _, issue := range issues {
			allIssues = append(allIssues, issueInfo{
				issue: issue,
				repo:  repo,
			})
		}
	}

	return allIssues
}

// filterPendingIssues loads state for each issue and filters out completed/failed
func (d *Daemon) filterPendingIssues(ctx context.Context, issues []issueInfo) []issueInfo {
	var pending []issueInfo

	for _, info := range issues {
		phase := state.ParsePhaseFromLabels(info.issue.Labels)

		// Skip completed/failed issues
		if phase == state.PhaseCompleted || phase == state.PhaseFailed {
			continue
		}

		// Load or create state
		st, err := d.orchestrator.loadState(ctx, info.repo, info.issue.Number)
		if err != nil {
			st = state.NewState()
			if phase != state.PhaseNew {
				st.CurrentPhase = phase
			}
		}

		// Store state in our tracking map
		d.allStatesMu.Lock()
		if d.allStates[info.repo] == nil {
			d.allStates[info.repo] = make(map[int]*state.State)
		}
		d.allStates[info.repo][info.issue.Number] = st
		d.allStatesMu.Unlock()

		info.state = st
		pending = append(pending, info)
	}

	return pending
}

// detectDependencies detects dependencies for issues that don't have them yet
func (d *Daemon) detectDependencies(ctx context.Context, issues []issueInfo) {
	for _, info := range issues {
		// Skip if dependencies already detected
		if info.state.DependsOn != nil {
			continue
		}

		deps, err := d.depDetector.DetectDependencies(ctx, info.repo, info.issue)
		if err != nil {
			d.logger.Printf("Error detecting dependencies for #%d: %v", info.issue.Number, err)
			continue
		}

		if len(deps) > 0 {
			info.state.DependsOn = deps
			d.logger.Printf("Issue #%d depends on: %v", info.issue.Number, deps)
		}
	}

	// Check for cycles across all known issues
	d.checkForCycles()
}

// checkForCycles checks for dependency cycles across all tracked issues
func (d *Daemon) checkForCycles() {
	d.allStatesMu.RLock()
	defer d.allStatesMu.RUnlock()

	depGraph := make(map[int][]int)
	for _, repoStates := range d.allStates {
		for issueNum, st := range repoStates {
			if len(st.DependsOn) > 0 {
				depGraph[issueNum] = st.DependsOn
			}
		}
	}

	if err := d.depDetector.CheckForCycles(depGraph); err != nil {
		d.logger.Printf("Warning: %v", err)
		// Mark issues in cycles as failed
		// This would require more complex cycle detection to identify which issues
	}
}

// resolveReadyIssues returns issues that are ready to be processed (all deps satisfied)
func (d *Daemon) resolveReadyIssues(ctx context.Context, issues []issueInfo) []issueInfo {
	var ready []issueInfo

	// Get active states snapshot before acquiring allStatesMu to avoid deadlock
	activeStates := d.workerPool.GetActiveStates()

	d.allStatesMu.RLock()
	defer d.allStatesMu.RUnlock()

	for _, info := range issues {
		// Check if this specific issue is already being processed
		jobID := fmt.Sprintf("%s-%d", info.repo, info.issue.Number)
		if _, isActive := activeStates[jobID]; isActive {
			continue
		}

		// Check dependencies
		repoStates := d.allStates[info.repo]
		if d.orchestrator.CanProceed(ctx, info.repo, info.issue, info.state, repoStates) {
			ready = append(ready, info)
		} else {
			// Update blocked_by field if blocked
			info.state.BlockedBy = d.getBlockingIssues(info.state, repoStates)
		}
	}

	return ready
}

// getBlockingIssues returns the list of issues blocking this one
func (d *Daemon) getBlockingIssues(st *state.State, repoStates map[int]*state.State) []int {
	var blocking []int

	for _, depNum := range st.DependsOn {
		depState, exists := repoStates[depNum]
		if !exists {
			// Unknown dependency - consider it blocking
			blocking = append(blocking, depNum)
			continue
		}

		if depState.CurrentPhase != state.PhaseCompleted {
			blocking = append(blocking, depNum)
		}
	}

	return blocking
}

// reportStatus logs the current status of issue processing
func (d *Daemon) reportStatus() {
	activeCount := d.workerPool.GetActiveCount()
	if activeCount > 0 {
		d.logger.Printf("Active jobs: %d", activeCount)
	}
}

// processJobWorker is the worker function that processes a single job
func (d *Daemon) processJobWorker(ctx context.Context, job *Job) error {
	return d.orchestrator.ProcessIssue(ctx, job.Repository, job.Issue)
}

// Shutdown performs graceful shutdown of the daemon
func (d *Daemon) Shutdown(ctx context.Context) error {
	if d.workerPool == nil {
		return nil
	}

	d.logger.Printf("Initiating graceful shutdown...")

	// 1. Stop accepting new jobs
	d.workerPool.StopAccepting()

	// 2. Persist state for all in-progress jobs before shutdown
	d.persistAllInProgressStates(ctx)

	// 3. Wait for in-progress jobs with timeout
	done := make(chan struct{})
	go func() {
		d.workerPool.Wait()
		close(done)
	}()

	select {
	case <-done:
		d.logger.Printf("All jobs completed, shutdown complete")
		return nil
	case <-ctx.Done():
		// Force cancel remaining jobs (state already persisted above)
		d.workerPool.Cancel()
		d.logger.Printf("Shutdown timeout, cancelled remaining jobs")
		return ctx.Err()
	}
}

// persistAllInProgressStates saves state for all active jobs
func (d *Daemon) persistAllInProgressStates(ctx context.Context) {
	activeStates := d.workerPool.GetActiveStates()
	for jobID, st := range activeStates {
		repo, issueNum := ParseJobID(jobID)
		comment, err := st.AppendToBody("State saved during shutdown")
		if err != nil {
			d.logger.Printf("Failed to serialize state for %s: %v", jobID, err)
			continue
		}
		if _, err := d.provider.CreateComment(ctx, repo, issueNum, comment); err != nil {
			d.logger.Printf("Failed to persist state for %s: %v", jobID, err)
		}
	}
}

// RunOnce processes a single issue once (for manual runs)
func (d *Daemon) RunOnce(ctx context.Context, repo string, issueNum int) error {
	issue, err := d.provider.GetIssue(ctx, repo, issueNum)
	if err != nil {
		return err
	}

	return d.orchestrator.ProcessIssue(ctx, repo, issue)
}
