# llm-bridge

Go service bridging Discord/Terminal to Claude/Codex CLI.

## Development Environment

Go is NOT installed locally. Use Docker for all Go commands:
`docker run --rm -v /root/llm-bridge:/app -w /app golang:1.21 go <command>`

## Dependencies

Prefer well-known open source libraries over hand-rolled implementations.
Use popular, battle-tested Go packages when available (e.g., rate limiting,
caching, validation) rather than reimplementing from scratch.

## Build

```bash
go build -o llm-bridge ./cmd/llm-bridge
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
- `internal/llm/` - LLM interface, Claude/Codex wrappers (PTY-based)
- `internal/provider/` - Discord/Terminal providers
- `internal/ratelimit/` - Per-user and per-channel rate limiting
- `internal/router/` - Command routing (/, !)
- `internal/output/` - Output handling, file attachments

## Features

- **Input merging** - Multiple sources merged to LLM stdin
- **Conflict prefixing** - `[Discord]` prefix when sources collide
- **Output broadcast** - All output sent to ALL connected channels
- **Idle timeout** - LLM process stops after idle period
- **LLM selection** - Configure claude or codex per repo
- **Terminal** - Local stdin/stdout always enabled
- **Rate limiting** - Per-user and per-channel token-bucket rate limiting

## CI

- GitHub Actions on PRs with `ci` label
- Coverage threshold: 90% (enforced)
- `go test -v -race -coverprofile=coverage.out ./...`
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
