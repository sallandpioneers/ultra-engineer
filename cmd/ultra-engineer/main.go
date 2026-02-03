package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	// Version information (set via ldflags at build time)
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

var (
	configPath string
	verbose    bool
	logFile    string
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
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Path to log file (logs to both stdout and file)")

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
			fmt.Printf("ultra-engineer %s\n", version)
			fmt.Printf("  Commit:     %s\n", commit)
			fmt.Printf("  Built:      %s\n", buildDate)
			fmt.Printf("  Go version: %s\n", runtime.Version())
		},
	}
}

// setupLogger creates a logger that writes to stdout and optionally to a file.
// It returns the logger, a cleanup function to close the file handle, and any error.
// If logFilePath is empty, the logger writes to stdout only.
// If the file cannot be opened, it logs a warning to stderr and returns a stdout-only logger.
func setupLogger(logFilePath string, verbose bool) (*log.Logger, func(), error) {
	flags := log.LstdFlags
	if verbose {
		flags |= log.Lshortfile
	}

	// If no log file specified, return stdout-only logger
	if logFilePath == "" {
		logger := log.New(os.Stdout, "[ultra-engineer] ", flags)
		return logger, func() {}, nil
	}

	// Create parent directories if they don't exist
	dir := filepath.Dir(logFilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create log directory %s: %v, logging to stdout only\n", dir, err)
		logger := log.New(os.Stdout, "[ultra-engineer] ", flags)
		return logger, func() {}, nil
	}

	// Open log file
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to open log file %s: %v, logging to stdout only\n", logFilePath, err)
		logger := log.New(os.Stdout, "[ultra-engineer] ", flags)
		return logger, func() {}, nil
	}

	// Create multi-writer for both stdout and file
	multiWriter := io.MultiWriter(os.Stdout, file)
	logger := log.New(multiWriter, "[ultra-engineer] ", flags)

	cleanup := func() {
		file.Sync()
		file.Close()
	}

	return logger, cleanup, nil
}
