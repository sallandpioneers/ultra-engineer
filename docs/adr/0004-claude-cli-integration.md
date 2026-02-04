# ADR-0004: Claude CLI Integration

## Status

Accepted

## Context

Ultra Engineer needs to interact with Claude to:

- Generate clarifying questions about issues
- Create implementation plans
- Write code
- Review and refine implementations
- Fix CI failures

We need to decide how to integrate with Claude's capabilities while maintaining:

- **Reliability**: Consistent behavior across invocations
- **Controllability**: Ability to guide Claude's actions
- **Observability**: Understanding what Claude is doing
- **Maintainability**: Minimal custom code for AI interaction

## Decision

We will shell out to the Claude Code CLI binary (`claude`) rather than using the API directly.

**Integration Approach**:
- Invoke `claude` binary with appropriate prompts
- Use `--print` flag for non-interactive responses
- Use `--allowedTools` to restrict available tools per phase
- Use `--systemPrompt` for phase-specific instructions
- Parse structured output (JSON) for machine-readable responses

**Prompt Templates**: Store phase-specific prompts in `internal/claude/prompts.go`:
- Q&A prompt for generating clarifying questions
- Planning prompt for creating implementation plans
- Implementation prompt for writing code
- Review prompt for code review

**Session Management**:
- Each phase invocation is a separate Claude session
- Context is passed via prompts (issue body, previous Q&A, plan, etc.)
- No persistent Claude sessions across phases

**Timeout Handling**:
- Configurable timeout per invocation (default: 30 minutes)
- Long-running implementations get full timeout
- Quick operations (Q&A, planning) typically complete faster

## Consequences

### Positive

- **Leverage existing tooling**: Claude Code CLI handles authentication, tool execution, and sandboxing
- **Minimal maintenance**: Don't need to implement Claude API client
- **Consistent behavior**: Same Claude Code used by humans works in automation
- **Tool filtering**: Can restrict Claude's capabilities per phase (e.g., no file editing during planning)
- **Observable**: CLI output can be logged and debugged

### Negative

- **External dependency**: Requires Claude CLI installed and configured
- **Process overhead**: Shell invocation has more overhead than direct API
- **Limited control**: Can't implement custom tools or streaming handlers
- **Version coupling**: Dependent on Claude CLI version and behavior

### Neutral

- Prompts are the primary control mechanism
- Output parsing depends on Claude's response format
- Error handling relies on exit codes and output

## Alternatives Considered

### Direct API integration

Use Claude API directly via HTTP/SDK.

Rejected because:
- Would need to implement tool execution ourselves
- Claude Code CLI already handles sandboxing, git operations, etc.
- More code to maintain
- Would duplicate existing functionality

### Custom Claude agent

Build a custom agent framework around Claude API.

Rejected because:
- Significant implementation effort
- Claude Code already provides agent capabilities
- Maintenance burden for keeping up with API changes

### MCP (Model Context Protocol) integration

Use MCP servers for specialized capabilities.

Considered for future enhancement, but initial version uses CLI because:
- Simpler to implement
- MCP adds complexity
- CLI provides sufficient capabilities for current workflow

### Persistent sessions

Keep Claude sessions alive across phases.

Rejected because:
- Adds complexity (session management, timeouts)
- Each phase has different context requirements
- Clean slate per phase prevents context pollution
