package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configPath string
	verbose    bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "ultra-engineer",
		Short: "Orchestrate Claude Code to implement issues from git providers",
		Long: `Ultra Engineer orchestrates Claude Code CLI instances to automatically
implement issues from GitHub, Gitea, and GitLab.

It handles the full workflow:
- Q&A: Ask clarifying questions about the issue
- Planning: Create and review implementation plans
- Implementation: Write code with automated review cycles
- PR: Create PRs, fix CI failures, and auto-merge`,
	}

	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "config.yaml", "Path to config file")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")

	rootCmd.AddCommand(daemonCmd())
	rootCmd.AddCommand(runCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(abortCmd())
	rootCmd.AddCommand(versionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("ultra-engineer v0.1.0")
		},
	}
}
