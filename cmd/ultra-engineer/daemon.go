package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/orchestrator"
	"github.com/anthropics/ultra-engineer/internal/providers"
)

func daemonCmd() *cobra.Command {
	var repos []string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon, polling for issues to process",
		Long: `Run Ultra Engineer as a daemon that continuously polls for issues
with the trigger label and processes them automatically.

Supports monitoring multiple repositories concurrently.
Repositories can be specified via --repo flags or the "repos" field in config.yaml.

Example:
  ultra-engineer daemon --repo owner/repo
  ultra-engineer daemon --repo owner/repo1 --repo owner/repo2`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(repos)
		},
	}

	cmd.Flags().StringArrayVar(&repos, "repo", nil, "Repository to monitor (owner/repo), can be specified multiple times")

	return cmd
}

func runDaemon(cliRepos []string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// CLI flags take precedence; fall back to config repos
	repos := cliRepos
	if len(repos) == 0 {
		repos = cfg.Repos
	}
	if len(repos) == 0 {
		return fmt.Errorf("no repositories specified (use --repo flag or \"repos\" in config.yaml)")
	}

	// Determine log file path (CLI flag takes precedence over config)
	logFilePath := logFile
	if logFilePath == "" {
		logFilePath = cfg.LogFile
	}

	// Create logger
	logger, cleanup, err := setupLogger(logFilePath, verbose)
	if err != nil {
		return fmt.Errorf("failed to setup logger: %w", err)
	}
	defer cleanup()

	// Create provider
	provider, err := createProvider(cfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Create daemon
	daemon := orchestrator.NewDaemon(cfg, provider, logger)

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigCh:
			logger.Println("Received shutdown signal")
			cancel()
		case <-ctx.Done():
			// Context cancelled, exit goroutine
		}
	}()

	// Run daemon
	return daemon.Run(ctx, repos)
}

func createProvider(cfg *config.Config) (providers.Provider, error) {
	switch cfg.Provider {
	case "gitea":
		return providers.NewGiteaProvider(cfg.Gitea.URL, cfg.Gitea.Token), nil
	case "github":
		return providers.NewGitHubProvider(cfg.GitHub.Token), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", cfg.Provider)
	}
}
