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
	var repo string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon, polling for issues to process",
		Long: `Run Ultra Engineer as a daemon that continuously polls for issues
with the trigger label and processes them automatically.

Example:
  ultra-engineer daemon --repo owner/repo`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			return runDaemon(repo)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository to monitor (owner/repo)")
	cmd.MarkFlagRequired("repo")

	return cmd
}

func runDaemon(repo string) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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
	return daemon.Run(ctx, repo)
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
