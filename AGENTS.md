# Agent Instructions

Project-specific instructions for AI agents working on llm-bridge.

## Project Overview

llm-bridge is a bidirectional bridge connecting chat platforms (Discord, Telegram, Terminal) to LLM CLIs (Claude Code, Codex).

## Development Workflow

This project uses the **kenken iterate workflow** with GitHub CI for testing.

### Testing Strategy

Tests run in GitHub Actions CI, not locally. When implementing:

1. Create a feature branch
2. Implement changes
3. Push and create a PR
4. Add the `ci` label to trigger CI
5. Wait for all checks to pass before merging

### Build Commands

```bash
# Local build via Docker
docker build -t llm-bridge:dev .
docker run --rm llm-bridge:dev --help

# Or via Makefile
make build
make docker
```

### Key Directories

- `cmd/llm-bridge/` - CLI entry point
- `internal/bridge/` - Core bridge logic
- `internal/config/` - Configuration handling
- `internal/llm/` - LLM backend implementations (Claude, Codex)
- `internal/provider/` - Chat provider implementations (Discord, Telegram, Terminal)
- `internal/router/` - Command routing
- `internal/output/` - Output formatting

### Configuration

- `llm-bridge.yaml` - Runtime configuration
- `.claude/kenken-config.json` - Kenken workflow configuration

### Code Style

- Follow existing patterns in the codebase
- Use structured logging with slog
- Handle errors explicitly
- Keep functions focused and small
