package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/state"
)

func abortCmd() *cobra.Command {
	var repo string
	var issueNum int

	cmd := &cobra.Command{
		Use:   "abort",
		Short: "Abort processing of an issue",
		Long: `Abort processing of an issue by adding the abort label.

This will stop any ongoing processing and mark the issue as failed.

Example:
  ultra-engineer abort --repo owner/repo --issue 123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}
			if issueNum == 0 {
				return fmt.Errorf("--issue is required")
			}

			return abortIssue(repo, issueNum)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository (owner/repo)")
	cmd.Flags().IntVar(&issueNum, "issue", 0, "Issue number")
	cmd.MarkFlagRequired("repo")
	cmd.MarkFlagRequired("issue")

	return cmd
}

func abortIssue(repo string, issueNum int) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create provider
	provider, err := createProvider(cfg)
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	ctx := context.Background()

	// Add abort label
	if err := provider.AddLabel(ctx, repo, issueNum, "abort"); err != nil {
		return fmt.Errorf("failed to add abort label: %w", err)
	}

	// Post abort comment
	comment := "**Processing aborted** via CLI command."
	if err := provider.CreateComment(ctx, repo, issueNum, comment); err != nil {
		return fmt.Errorf("failed to post abort comment: %w", err)
	}

	// Update phase label
	if err := provider.AddLabel(ctx, repo, issueNum, state.PhaseFailed.Label()); err != nil {
		return fmt.Errorf("failed to add failed label: %w", err)
	}

	// Remove trigger label (best-effort, don't fail if it doesn't exist)
	if err := provider.RemoveLabel(ctx, repo, issueNum, cfg.TriggerLabel); err != nil {
		// Log but don't fail - the abort was still successful
		fmt.Fprintf(os.Stderr, "Warning: failed to remove trigger label: %v\n", err)
	}

	fmt.Printf("Aborted processing of issue #%d\n", issueNum)
	return nil
}
