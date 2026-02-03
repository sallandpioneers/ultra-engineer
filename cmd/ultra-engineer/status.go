package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/anthropics/ultra-engineer/internal/config"
	"github.com/anthropics/ultra-engineer/internal/state"
)

func statusCmd() *cobra.Command {
	var repo string
	var issueNum int

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Check status of issues being processed",
		Long: `Check the current status of issues being processed by Ultra Engineer.

If --issue is specified, shows detailed status for that issue.
Otherwise, lists all issues with the trigger label.

Example:
  ultra-engineer status --repo owner/repo
  ultra-engineer status --repo owner/repo --issue 123`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			if issueNum > 0 {
				return showIssueStatus(repo, issueNum)
			}
			return listIssues(repo)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "Repository (owner/repo)")
	cmd.Flags().IntVar(&issueNum, "issue", 0, "Specific issue number (optional)")
	cmd.MarkFlagRequired("repo")

	return cmd
}

func listIssues(repo string) error {
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

	// Get issues with trigger label
	issues, err := provider.ListIssuesWithLabel(ctx, repo, cfg.TriggerLabel)
	if err != nil {
		return fmt.Errorf("failed to list issues: %w", err)
	}

	if len(issues) == 0 {
		fmt.Printf("No issues found with label '%s'\n", cfg.TriggerLabel)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ISSUE\tTITLE\tPHASE\tAUTHOR")
	fmt.Fprintln(w, "-----\t-----\t-----\t------")

	for _, issue := range issues {
		phase := state.ParsePhaseFromLabels(issue.Labels)
		title := issue.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Fprintf(w, "#%d\t%s\t%s\t%s\n", issue.Number, title, phase, issue.Author)
	}

	w.Flush()
	return nil
}

func showIssueStatus(repo string, issueNum int) error {
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

	// Get issue
	issue, err := provider.GetIssue(ctx, repo, issueNum)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}

	// Get comments to find state
	comments, err := provider.GetComments(ctx, repo, issueNum)
	if err != nil {
		return fmt.Errorf("failed to get comments: %w", err)
	}

	// Parse state from comments
	var commentBodies []string
	for _, c := range comments {
		commentBodies = append(commentBodies, c.Body)
	}
	st, _ := state.ParseFromComments(commentBodies)

	// Display status
	fmt.Printf("Issue #%d: %s\n", issue.Number, issue.Title)
	fmt.Printf("Author: %s\n", issue.Author)
	fmt.Printf("State: %s\n", issue.State)
	fmt.Println()

	phase := state.ParsePhaseFromLabels(issue.Labels)
	fmt.Printf("Processing Phase: %s\n", phase)

	if st != nil {
		fmt.Printf("Q&A Rounds: %d\n", st.QARound)
		fmt.Printf("Plan Version: %d\n", st.PlanVersion)
		fmt.Printf("Review Iteration: %d\n", st.ReviewIteration)
		if st.PRNumber > 0 {
			fmt.Printf("PR Number: #%d\n", st.PRNumber)
		}
		if st.BranchName != "" {
			fmt.Printf("Branch: %s\n", st.BranchName)
		}
		if st.Error != "" {
			fmt.Printf("Error: %s\n", st.Error)
		}
		fmt.Printf("Last Updated: %s\n", st.LastUpdated.Format("2006-01-02 15:04:05"))
	} else {
		fmt.Println("(No processing state found)")
	}

	return nil
}
