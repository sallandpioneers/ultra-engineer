package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

// Job represents a unit of work for the worker pool
type Job struct {
	Issue      *providers.Issue
	Repository string
	State      *state.State
}

// JobID returns a unique identifier for the job
func (j *Job) JobID() string {
	return fmt.Sprintf("%s-%d", j.Repository, j.Issue.Number)
}

// JobResult represents the result of processing a job
type JobResult struct {
	Job   *Job
	Error error
}

// WorkerPool manages concurrent issue processing
type WorkerPool struct {
	maxPerRepo int
	maxTotal   int
	jobQueue   chan *Job
	results    chan *JobResult
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Concurrency limiting (protected by mu)
	mu          sync.Mutex
	activeJobs  map[string]int       // repoKey -> count of active jobs
	totalActive int                  // Total count of active jobs across all repos
	accepting   bool                 // Whether pool is accepting new jobs

	// State tracking for graceful shutdown (protected by mu)
	activeStates map[string]*state.State // jobID -> current state for persistence

	// Worker function - set by caller
	workerFunc func(ctx context.Context, job *Job) error
}

// NewWorkerPool creates a new worker pool with the specified limits
func NewWorkerPool(ctx context.Context, maxPerRepo, maxTotal int) *WorkerPool {
	ctx, cancel := context.WithCancel(ctx)

	return &WorkerPool{
		maxPerRepo:   maxPerRepo,
		maxTotal:     maxTotal,
		jobQueue:     make(chan *Job, maxTotal),
		results:      make(chan *JobResult, maxTotal),
		ctx:          ctx,
		cancel:       cancel,
		activeJobs:   make(map[string]int),
		activeStates: make(map[string]*state.State),
		accepting:    true,
	}
}

// SetWorkerFunc sets the function that processes each job
func (wp *WorkerPool) SetWorkerFunc(fn func(ctx context.Context, job *Job) error) {
	wp.workerFunc = fn
}

// Start launches worker goroutines that consume from jobQueue
func (wp *WorkerPool) Start() {
	// Start workers equal to maxTotal
	for i := 0; i < wp.maxTotal; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// worker processes jobs from the queue
func (wp *WorkerPool) worker() {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case job, ok := <-wp.jobQueue:
			if !ok {
				return
			}
			wp.processJob(job)
		}
	}
}

// processJob executes the worker function for a job
func (wp *WorkerPool) processJob(job *Job) {
	// Register state for graceful shutdown
	wp.RegisterState(job.JobID(), job.State)
	defer wp.UnregisterState(job.JobID())

	var err error
	if wp.workerFunc != nil {
		err = wp.workerFunc(wp.ctx, job)
	}

	// Send result
	select {
	case wp.results <- &JobResult{Job: job, Error: err}:
	case <-wp.ctx.Done():
	}
}

// TrySubmit atomically checks limits and submits job
// Returns false if per-repo or total limit reached, or if pool is shutting down
func (wp *WorkerPool) TrySubmit(job *Job) bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.accepting {
		return false // Pool is shutting down
	}
	if wp.totalActive >= wp.maxTotal {
		return false
	}
	if wp.activeJobs[job.Repository] >= wp.maxPerRepo {
		return false
	}

	wp.activeJobs[job.Repository]++
	wp.totalActive++

	// Non-blocking send since channel is buffered to maxTotal
	select {
	case wp.jobQueue <- job:
		return true
	default:
		// Channel full, revert counts
		wp.activeJobs[job.Repository]--
		wp.totalActive--
		return false
	}
}

// OnJobComplete decrements activeJobs count for the repository
func (wp *WorkerPool) OnJobComplete(repoKey string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.activeJobs[repoKey] > 0 {
		wp.activeJobs[repoKey]--
	}
	if wp.totalActive > 0 {
		wp.totalActive--
	}
}

// RegisterState stores current job state for graceful shutdown persistence
func (wp *WorkerPool) RegisterState(jobID string, st *state.State) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.activeStates[jobID] = st
}

// UnregisterState removes job state after completion
func (wp *WorkerPool) UnregisterState(jobID string) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	delete(wp.activeStates, jobID)
}

// GetActiveStates returns a copy of all active job states for persistence
func (wp *WorkerPool) GetActiveStates() map[string]*state.State {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	result := make(map[string]*state.State, len(wp.activeStates))
	for k, v := range wp.activeStates {
		result[k] = v
	}
	return result
}

// GetActiveCount returns the current number of active jobs
func (wp *WorkerPool) GetActiveCount() int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.totalActive
}

// GetActiveCountForRepo returns the number of active jobs for a specific repo
func (wp *WorkerPool) GetActiveCountForRepo(repo string) int {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	return wp.activeJobs[repo]
}

// Results returns the results channel for reading completed jobs
func (wp *WorkerPool) Results() <-chan *JobResult {
	return wp.results
}

// StopAccepting prevents new jobs from being submitted (called during shutdown)
func (wp *WorkerPool) StopAccepting() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.accepting = false
}

// Wait blocks until all workers complete
func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

// Cancel cancels all in-progress jobs via context
func (wp *WorkerPool) Cancel() {
	wp.cancel()
}

// Shutdown performs graceful shutdown
func (wp *WorkerPool) Shutdown() {
	wp.StopAccepting()
	close(wp.jobQueue)
	wp.Wait()
	wp.cancel()
}

// ParseJobID parses a job ID into repository and issue number
func ParseJobID(jobID string) (repo string, issueNum int) {
	lastDash := strings.LastIndex(jobID, "-")
	if lastDash == -1 {
		return jobID, 0
	}

	repo = jobID[:lastDash]
	fmt.Sscanf(jobID[lastDash+1:], "%d", &issueNum)
	return repo, issueNum
}
