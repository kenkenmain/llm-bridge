# llm-bridge

Go service bridging Discord/Terminal to Claude CLI.

## Development Environment

This project supports two development modes:

**Local Go (debugging and development):**
- Run `go build`, `go test`, `make debug` directly when Go is installed locally.
- Faster iteration for development and debugging.

**Docker (CI and production):**
- Run `make docker-test`, `make docker-lint`, `make docker-build` when Go is not installed.
- Ensures consistent environment for CI and production builds.

## Dependencies

Prefer well-known open source libraries over hand-rolled implementations.
Use popular, battle-tested Go packages when available (e.g., rate limiting,
caching, validation) rather than reimplementing from scratch.

## Build

**Local:**
```bash
go build -o llm-bridge ./cmd/llm-bridge
```

**Docker:**
```bash
make docker-build
```

## Run

```bash
export DISCORD_BOT_TOKEN=your_token
./llm-bridge serve --config llm-bridge.yaml
```

## Docker

```bash
docker-compose up -d
```

## Add Repo

```bash
./llm-bridge add-repo myrepo \
  --provider discord \
  --channel 123456789 \
  --llm claude \
  --dir /path/to/repo
```

## Architecture

- `internal/bridge/` - Core bridge logic, input merging, output broadcasting
- `internal/config/` - YAML config parsing
- `internal/llm/` - LLM interface, Claude wrapper (PTY-based)
- `internal/provider/` - Discord/Terminal providers
- `internal/ratelimit/` - Per-user and per-channel rate limiting
- `internal/router/` - Command routing (/, !)
- `internal/output/` - Output handling, file attachments

## Features

- **Input merging** - Multiple sources merged to LLM stdin
- **Conflict prefixing** - `[Discord]` prefix when sources collide
- **Output broadcast** - All output sent to ALL connected channels
- **Idle timeout** - LLM process stops after idle period
- **Claude CLI** - Claude as the LLM backend
- **Terminal** - Local stdin/stdout always enabled
- **Rate limiting** - Per-user and per-channel token-bucket rate limiting

## CI

- GitHub Actions on PRs with `ci` label
- Coverage threshold: 90% (enforced)
- Tests run inside Docker containers
- Docker build verification

## Commands

| Input            | Handler | Description                   |
| ---------------- | ------- | ----------------------------- |
| `/status`        | Bridge  | Show LLM status and idle time |
| `/cancel`        | Bridge  | Send SIGINT to LLM            |
| `/restart`       | Bridge  | Restart LLM process           |
| `/select <repo>` | Bridge  | Select repo for terminal      |
| `/help`          | Bridge  | Show available commands       |
| `!commit`        | LLM     | Translates to `/commit`       |
| text             | LLM     | Raw message to LLM            |
