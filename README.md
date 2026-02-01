# llm-bridge

Go service that bridges Discord and Terminal interfaces to Claude CLI, enabling multi-channel LLM interaction.

## Features

- **Multi-provider input** — Connect Discord bots and local terminal simultaneously
- **Input merging** — Messages from multiple sources merged to LLM stdin with conflict prefixing
- **Output broadcast** — All LLM output sent to every connected channel
- **Rate limiting** — Per-user and per-channel token-bucket rate limiting
- **Idle timeout** — Automatic LLM process shutdown after configurable idle period
- **File attachments** — Long outputs automatically sent as file attachments

## Prerequisites

- Go 1.21+ (for local development/debugging)
- Docker & Docker Compose (for running and CI)
- Discord bot token

## Quick Start

### Local Development (Debug)

```bash
# Build with debug symbols and race detector
make debug

# Run
export DISCORD_BOT_TOKEN=your_token
./llm-bridge serve --config llm-bridge.yaml
```

### Docker (Production)

```bash
# Configure
cp llm-bridge.yaml.example llm-bridge.yaml
# Edit llm-bridge.yaml with your settings

# Run
docker-compose up -d
```

## Development

### Local Go (Debugging)

```bash
make build          # Standard build
make debug          # Build with race detector + debug symbols
make test           # Run tests locally
make lint           # Run linter locally
```

### Docker (CI-equivalent)

```bash
make docker-test    # Run tests in container
make docker-lint    # Run linter in container
make docker-build   # Build in container
make docker         # Build Docker image
```

## Configuration

Copy `llm-bridge.yaml.example` to `llm-bridge.yaml`:

```yaml
repos:
  my-repo:
    provider: discord
    channel_id: "123456789012345678"
    llm: claude
    working_dir: /path/to/repo

defaults:
  llm: claude
  idle_timeout: 10m
  rate_limit:
    enabled: true
    user_rate: 0.5
    user_burst: 3
```

See `llm-bridge.yaml.example` for all options.

## Commands

| Input            | Description                   |
| ---------------- | ----------------------------- |
| `/status`        | Show LLM status and idle time |
| `/cancel`        | Send SIGINT to LLM            |
| `/restart`       | Restart LLM process           |
| `/select <repo>` | Select repo for terminal      |
| `/help`          | Show available commands        |
| `!commit`        | Translates to `/commit` for LLM |

## Architecture

```
cmd/llm-bridge/     Entry point (Cobra CLI)
internal/
  bridge/           Core orchestration, session management, output fanout
  config/           YAML configuration parsing
  llm/              LLM interface, Claude PTY wrapper
  provider/         Discord and Terminal providers
  ratelimit/        Token-bucket rate limiting
  router/           Command routing (/ and ! prefixes)
  output/           Output formatting, file attachments
```

## CI

GitHub Actions runs on PRs with the `ci` label:
- Tests and coverage (90% threshold) in Docker container
- Linting via golangci-lint Docker image
- Docker image build verification

## License

[Add license]
