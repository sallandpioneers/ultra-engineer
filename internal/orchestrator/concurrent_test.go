package orchestrator

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/ultra-engineer/internal/providers"
	"github.com/anthropics/ultra-engineer/internal/state"
)

func TestWorkerPoolLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	maxPerRepo := 2
	maxTotal := 3

	wp := NewWorkerPool(ctx, maxPerRepo, maxTotal)

	var processedCount int32
	wp.SetWorkerFunc(func(ctx context.Context, job *Job) error {
		atomic.AddInt32(&processedCount, 1)
		time.Sleep(100 * time.Millisecond) // Simulate work
		return nil
	})

	wp.Start()

	// Create jobs for 2 repos
	jobs := []*Job{
		{Issue: &providers.Issue{Number: 1}, Repository: "repo-a", State: state.NewState()},
		{Issue: &providers.Issue{Number: 2}, Repository: "repo-a", State: state.NewState()},
		{Issue: &providers.Issue{Number: 3}, Repository: "repo-a", State: state.NewState()}, // Should be rejected (max per repo)
		{Issue: &providers.Issue{Number: 4}, Repository: "repo-b", State: state.NewState()},
		{Issue: &providers.Issue{Number: 5}, Repository: "repo-b", State: state.NewState()}, // Should be rejected (max total)
	}

	// Submit jobs
	submitted := 0
	for _, job := range jobs {
		if wp.TrySubmit(job) {
			submitted++
		}
	}

	// Should only submit up to maxTotal (3)
	if submitted > maxTotal {
		t.Errorf("expected at most %d jobs submitted, got %d", maxTotal, submitted)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	// Drain results
	for i := 0; i < submitted; i++ {
		select {
		case result := <-wp.Results():
			wp.OnJobComplete(result.Job.Repository)
		case <-time.After(time.Second):
			t.Error("timeout waiting for result")
		}
	}

	wp.Shutdown()
}

func TestWorkerPoolPerRepoLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	maxPerRepo := 1
	maxTotal := 5

	wp := NewWorkerPool(ctx, maxPerRepo, maxTotal)

	var completedCount int32
	wp.SetWorkerFunc(func(ctx context.Context, job *Job) error {
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&completedCount, 1)
		return nil
	})

	wp.Start()

	// Try to submit 3 jobs for the same repo
	jobs := []*Job{
		{Issue: &providers.Issue{Number: 1}, Repository: "repo-a", State: state.NewState()},
		{Issue: &providers.Issue{Number: 2}, Repository: "repo-a", State: state.NewState()},
		{Issue: &providers.Issue{Number: 3}, Repository: "repo-a", State: state.NewState()},
	}

	submitted := 0
	for _, job := range jobs {
		if wp.TrySubmit(job) {
			submitted++
		}
	}

	// Only 1 should be submitted (maxPerRepo = 1)
	if submitted != maxPerRepo {
		t.Errorf("expected %d jobs submitted for single repo, got %d", maxPerRepo, submitted)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	wp.Shutdown()

	// Verify the job completed
	if atomic.LoadInt32(&completedCount) != 1 {
		t.Errorf("expected 1 job completed, got %d", completedCount)
	}
}

func TestWorkerPoolShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use high per-repo limit so all jobs can be submitted
	wp := NewWorkerPool(ctx, 5, 5)

	var completedJobs int32
	wp.SetWorkerFunc(func(ctx context.Context, job *Job) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
			atomic.AddInt32(&completedJobs, 1)
			return nil
		}
	})

	wp.Start()

	// Submit jobs to different repos to avoid per-repo limit
	for i := 0; i < 3; i++ {
		wp.TrySubmit(&Job{
			Issue:      &providers.Issue{Number: i + 1},
			Repository: fmt.Sprintf("repo-%d", i),
			State:      state.NewState(),
		})
	}

	// Give jobs time to complete
	time.Sleep(200 * time.Millisecond)

	// Shutdown should stop accepting new jobs
	wp.StopAccepting()

	// Try to submit another job - should fail
	if wp.TrySubmit(&Job{
		Issue:      &providers.Issue{Number: 100},
		Repository: "repo-new",
		State:      state.NewState(),
	}) {
		t.Error("expected job submission to fail after StopAccepting")
	}

	// Wait for existing jobs
	wp.Shutdown()

	// All submitted jobs should have completed
	completed := atomic.LoadInt32(&completedJobs)
	if completed != 3 {
		t.Errorf("expected 3 jobs completed, got %d", completed)
	}
}

func TestWorkerPoolStateTracking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wp := NewWorkerPool(ctx, 2, 5)

	stateCapture := make(chan map[string]*state.State, 1)
	wp.SetWorkerFunc(func(ctx context.Context, job *Job) error {
		// Capture active states mid-processing
		time.Sleep(50 * time.Millisecond)
		stateCapture <- wp.GetActiveStates()
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	wp.Start()

	testState := state.NewState()
	testState.CurrentPhase = state.PhaseImplementing

	wp.TrySubmit(&Job{
		Issue:      &providers.Issue{Number: 42},
		Repository: "test-repo",
		State:      testState,
	})

	// Get the captured states
	select {
	case states := <-stateCapture:
		if len(states) != 1 {
			t.Errorf("expected 1 active state, got %d", len(states))
		}
		jobID := "test-repo-42"
		if st, ok := states[jobID]; ok {
			if st.CurrentPhase != state.PhaseImplementing {
				t.Errorf("expected phase %s, got %s", state.PhaseImplementing, st.CurrentPhase)
			}
		} else {
			t.Errorf("expected state for job %s", jobID)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for state capture")
	}

	wp.Shutdown()
}

func TestParseJobID(t *testing.T) {
	tests := []struct {
		jobID    string
		repo     string
		issueNum int
	}{
		{"repo-123", "repo", 123},
		{"owner/repo-456", "owner/repo", 456},
		{"org-name/repo-name-789", "org-name/repo-name", 789},
		{"simple-1", "simple", 1},
		{"", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.jobID, func(t *testing.T) {
			repo, issueNum := ParseJobID(tt.jobID)
			if repo != tt.repo {
				t.Errorf("expected repo %q, got %q", tt.repo, repo)
			}
			if issueNum != tt.issueNum {
				t.Errorf("expected issueNum %d, got %d", tt.issueNum, issueNum)
			}
		})
	}
}

func TestJobID(t *testing.T) {
	job := &Job{
		Issue:      &providers.Issue{Number: 42},
		Repository: "owner/repo",
	}

	expected := "owner/repo-42"
	if job.JobID() != expected {
		t.Errorf("expected JobID %q, got %q", expected, job.JobID())
	}
}
