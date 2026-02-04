# Claude AI Agent Guide

This file provides entry points for AI agents working on this codebase.

**For comprehensive project context, see [AGENTS.md](AGENTS.md).**

## Quick Summary

Ultra Engineer is a Go application that orchestrates Claude Code CLI instances to automatically implement GitHub/Gitea issues. It uses a state machine to track workflow phases and stores state in issue body HTML comments.

## Key Entry Points

- `cmd/ultra-engineer/` - CLI commands
- `internal/orchestrator/` - Core orchestration logic
- `internal/workflow/` - Phase implementations
- `internal/providers/` - Git provider integrations
- `internal/state/` - Workflow state management
