package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/orchestrator"
)

func runCmd() *cobra.Command {
	var repo string
	var issueNum int

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Process a single issue",
		Long: `Process a single issue through the workflow.

This runs a single pass through the state machine. If the issue
requires user input (e.g., answering questions, approving plan),
it will post the request and exit. Run again after providing input.

Example:
  ultra-engineer run --repo owner/repo --issue 123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			if issueNum == 0 {
				return fmt.Errorf("--issue is required")
			}

			return runSingle(repo, issueNum)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository (owner/repo)")
	cmd.Flags().IntVar(&issueNum, "issue", 0, "Issue number")
	cmd.MarkFlagRequired("repo")
	cmd.MarkFlagRequired("issue")

	return cmd
}

func runSingle(repo string, issueNum int) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create logger
	logger := log.New(os.Stdout, "[ultra-engineer] ", log.LstdFlags)
	if verbose {
		logger.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	// Create provider
	provider, err := createProvider(cfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Create daemon (reuse for single run)
	daemon := orchestrator.NewDaemon(cfg, provider, logger)

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		logger.Println("Received shutdown signal")
		cancel()
	}()

	// Run single issue
	return daemon.RunOnce(ctx, repo, issueNum)
}
