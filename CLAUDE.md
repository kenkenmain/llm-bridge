# llm-bridge

Go service bridging Discord/Terminal to Claude CLI.

## Development Environment

This project uses **Bazel 8.5.1** for hermetic builds, testing, and linting.
Install [Bazelisk](https://github.com/bazelbuild/bazelisk) (auto-downloads the correct Bazel version).

## Dependencies

Prefer well-known open source libraries over hand-rolled implementations.
Use popular, battle-tested Go packages when available (e.g., rate limiting,
caching, validation) rather than reimplementing from scratch.

## Build

```bash
bazel build //cmd/llm-bridge   # build the binary
bazel build //...               # build everything
```

## Test

```bash
bazel test //...                # run all tests
```

## Lint

```bash
bazel test //:lint              # run golangci-lint
```

## Gazelle (regenerate BUILD files)

```bash
bazel run //:gazelle            # after changing imports or adding files
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

- GitHub Actions on PRs to `main` and pushes to `main` (automatic, no label required)
- Bazel build, test, and lint
- Docker image build verification

## Commands

| Input            | Handler | Description                   |
| ---------------- | ------- | ----------------------------- |
| `/status`        | Bridge  | Show LLM status and idle time |
| `/cancel`        | Bridge  | Send SIGINT to LLM            |
| `/restart`       | Bridge  | Restart LLM process           |
| `/select <repo>` | Bridge  | Select repo for terminal      |
| `/help`          | Bridge  | Show available commands        |
| `!commit`        | LLM     | Translates to `/commit`       |
| text             | LLM     | Raw message to LLM            |
