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

- [Bazelisk](https://github.com/bazelbuild/bazelisk) (auto-downloads Bazel 8.5.1)
- Docker & Docker Compose (for running)
- Discord bot token (see [Discord Bot Setup Guide](docs/discord-setup.md))

## Discord Setup

To connect llm-bridge to Discord, you need a bot token and channel IDs. The full setup involves creating a Discord application, enabling the Message Content privileged intent, inviting the bot with the correct permissions, and configuring channel IDs in your `llm-bridge.yaml`.

See the [Discord Bot Setup Guide](docs/discord-setup.md) for step-by-step instructions.

Quick invite URL (replace `YOUR_CLIENT_ID` with your application's Client ID):

```
https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&scope=bot&permissions=101376
```

## Quick Start

### Build & Test

```bash
bazel build //cmd/llm-bridge    # build the binary
bazel test //...                 # run all tests
bazel test //:lint               # run linter
```

### Run Locally

```bash
export DISCORD_BOT_TOKEN=your_token
bazel-bin/cmd/llm-bridge/llm-bridge_/llm-bridge serve --config llm-bridge.yaml
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

```bash
bazel build //...               # build everything
bazel test //...                 # run all tests
bazel coverage //...            # generate coverage report
bazel test //:lint               # golangci-lint
bazel run //:gazelle             # regenerate BUILD files after import changes
```

### Makefile Shortcuts

```bash
make build          # bazel build //cmd/llm-bridge
make test           # bazel test //...
make lint           # bazel test //:lint
make coverage       # bazel coverage + 90% threshold check
make gazelle        # bazel run //:gazelle
make image          # build and load OCI image
make run            # docker-compose up -d
make stop           # docker-compose down
```

## Configuration

Copy `llm-bridge.yaml.example` to `llm-bridge.yaml`.

### Minimal Configuration (Self-Discovery Mode)

You can start with just defaults - no pre-defined repos required! Use `/clone` from Discord to add repos dynamically:

```yaml
defaults:
  base_dir: /home/user/repos  # where /clone creates new repos
  llm: claude

providers:
  discord:
    bot_token: "${DISCORD_BOT_TOKEN}"
```

Then from Discord: `/clone https://github.com/user/repo myrepo 123456789012345678`

### Full Configuration (Pre-defined Repos)

```yaml
repos:
  my-repo:
    provider: discord
    channel_id: "123456789012345678"  # Discord channel ID (enable Developer Mode to copy)
    llm: claude
    working_dir: /path/to/repo

defaults:
  base_dir: /home/user/repos
  llm: claude
  idle_timeout: 10m
  rate_limit:
    enabled: true
    user_rate: 0.5
    user_burst: 3

providers:
  discord:
    bot_token: "${DISCORD_BOT_TOKEN}"
```

See `llm-bridge.yaml.example` for all options.

## Commands

### Session Control

| Input            | Description                   |
| ---------------- | ----------------------------- |
| `/status`        | Show LLM status and idle time |
| `/cancel`        | Send SIGINT to LLM            |
| `/restart`       | Restart LLM process           |
| `/select <repo>` | Select repo for terminal      |
| `/help`          | Show available commands        |
| `::commit`       | Translates to `/commit` for LLM |

### Dynamic Repo Management

| Input                                      | Description                        |
| ------------------------------------------ | ---------------------------------- |
| `/clone <url> <name> <channel-id>`         | Clone a git repo and register it (channel-id required for Discord) |
| `/add-worktree <name> <branch> <channel-id>` | Create worktree from current repo (channel-id required for Discord) |
| `/list-repos`                              | List all configured repos          |
| `/remove-repo <name>`                      | Remove a repo from config          |
| `/worktrees`                               | List git worktrees for current repo|

## Architecture

```
cmd/llm-bridge/     Entry point (Cobra CLI)
internal/
  bridge/           Core orchestration, session management, output fanout
  config/           YAML configuration parsing
  llm/              LLM interface, Claude PTY wrapper
  provider/         Discord and Terminal providers
  ratelimit/        Token-bucket rate limiting
  router/           Command routing (/ and :: prefixes)
  output/           Output formatting, file attachments
```

## CI

GitHub Actions runs automatically on PRs to `main` and pushes to `main`:
- Bazel build, test, and lint
- 90% line-coverage threshold enforcement (coverage report uploaded as artifact)
- Docker image build verification

## License

[Add license]
