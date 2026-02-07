# llm-bridge

Go service bridging Discord/Terminal to Claude CLI.

## Development Environment

This project uses **Bazel 8.5.1** for hermetic builds, testing, and linting.
Install [Bazelisk](https://github.com/bazelbuild/bazelisk) (auto-downloads the correct Bazel version).

- **Go 1.23.6** (managed by Bazel via `go_sdk.download` in MODULE.bazel — no local install needed)
- **Claude CLI** (`@anthropic-ai/claude-code`) — runtime dependency, spawned as child process via pipes (process-per-message with -p flag)

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

## Coverage

```bash
make coverage                          # run coverage check locally (90% threshold)
bazel coverage //...                   # generate coverage report only
bazel test //:coverage_check_test      # run coverage script self-test
```

Coverage enforces a 90% line-coverage threshold on `internal/` packages. The `cmd/` package is excluded. See `scripts/check-coverage.sh` for options (`--threshold`, `--exclude`, `--lcov-file`).

## Lint

```bash
bazel test //:lint              # run golangci-lint
```

## Gazelle (regenerate BUILD files)

```bash
bazel run //:gazelle            # after changing imports or adding files
```

## Integration Tests

```bash
make integration   # run Discord integration tests
bazel test //internal/provider:discord_integration_test --config=integration  # explicit target
```

Integration tests require network access and a valid Discord bot token. Default credentials are hardcoded in `internal/config/config.go` for this internal project. Override via environment variables:

- `DISCORD_BOT_TOKEN` — override default bot token
- `DISCORD_TEST_CHANNEL_ID` — override default test channel

Integration tests are tagged `manual` so `bazel test //...` skips them. CI runs them explicitly with `DISCORD_BOT_TOKEN` secret.

## Make (shortcuts)

The Makefile wraps Bazel commands for convenience:

```bash
make build        # bazel build //cmd/llm-bridge
make test         # bazel test //...
make lint         # bazel test //:lint
make coverage     # bazel coverage //... + threshold check (90%)
make integration  # run Discord integration tests
make gazelle      # bazel run //:gazelle
make docker       # full Docker build (base + prod image)
make image        # Bazel OCI image build + load
```

## Bazel Configs

```bash
bazel test //... --config=ci          # CI caching + verbose output
bazel build //... --config=race       # Go race detector
bazel test //... --config=integration # run integration tests (requires network + credentials)
bazel coverage //...                  # uses --combined_report=lcov and --instrumentation_filter=//internal/ from .bazelrc
```

## Run

### Local / Bare Metal (preferred for servers you control)

Bazel builds are hermetic — only Bazelisk and Node.js (for Claude CLI) are needed on the host. No Go install required.

```bash
# One-time: install Claude CLI
npm install -g @anthropic-ai/claude-code

# Build and run
bazel build //cmd/llm-bridge
# See docs/discord-setup.md for creating a Discord bot and obtaining this token
export DISCORD_BOT_TOKEN=your_token
export ANTHROPIC_API_KEY=your_key   # required by Claude CLI
./bazel-bin/cmd/llm-bridge/llm-bridge_/llm-bridge serve --config llm-bridge.yaml
```

For production, use a systemd unit for process management (auto-restart, journald logging):

```ini
[Unit]
Description=llm-bridge
After=network.target

[Service]
ExecStart=/usr/local/bin/llm-bridge serve --config /etc/llm-bridge/llm-bridge.yaml
Environment=DISCORD_BOT_TOKEN=xxx
Environment=ANTHROPIC_API_KEY=xxx
Restart=always
User=llm-bridge

[Install]
WantedBy=multi-user.target
```

### Docker (optional — for single-artifact deploys)

Docker bundles Node.js + Claude CLI + the Go binary. Useful when you want a single deployable artifact rather than managing Node.js on the host. Two-stage build:

```bash
docker build -f Dockerfile.base -t llm-bridge-base:latest .  # once
bazel build //cmd/llm-bridge
mkdir -p .build && cp -L bazel-bin/cmd/llm-bridge/llm-bridge_/llm-bridge .build/llm-bridge
docker build -t llm-bridge:latest .
docker compose up -d
```

Note: Docker requires bind-mounting host repo directories (see `docker-compose.yml`). Running bare metal avoids this indirection entirely.

## Add Repo

### Dynamic (from Discord/Terminal)

Use `/clone` to add repos at runtime without restarting:

```
/clone https://github.com/user/repo myrepo 123456789012345678
```

For Discord, `channel-id` is the 18-digit Discord channel ID (enable Developer Mode to copy).

The repo is cloned to `defaults.base_dir/<name>` and immediately available.

### CLI (before starting)

```bash
./llm-bridge add-repo myrepo \
  --provider discord \
  --channel 123456789 \
  --llm claude \
  --dir /path/to/repo
```

### Zero-Config Start

You can start with no pre-defined repos and use `/clone` to add them dynamically:

```yaml
# Minimal llm-bridge.yaml
defaults:
  base_dir: /home/user/repos
  llm: claude

providers:
  discord:
    bot_token: "${DISCORD_BOT_TOKEN}"
```

## Architecture

- `internal/bridge/` - Core bridge logic, input merging, output broadcasting
- `internal/config/` - YAML config parsing
- `internal/llm/` - LLM interface, Claude wrapper (pipe-based, process-per-message)
- `internal/provider/` - Discord/Terminal providers (Discord requires specific Gateway Intents and bot permissions — see `docs/discord-setup.md`)
- `internal/ratelimit/` - Per-user and per-channel rate limiting
- `internal/router/` - Command routing (/, ::)
- `internal/output/` - Output handling, file attachments

## Gotchas

- Lint runs **outside Bazel sandbox** (`no-sandbox` tag) — needs network for first `golangci-lint` download
- Only `claude` LLM backend exists (Codex was removed). Factory defaults empty string to `claude`
- Claude is spawned via `claude -p --output-format stream-json` (process-per-message). Each `Send()` spawns a new subprocess. Output is NDJSON parsed in `readOutput()` to extract text content for broadcasting
- See `llm-bridge.yaml.example` for config structure; can start with empty `repos:` and use `/clone` dynamically
- Bazel builds are fully hermetic (Go SDK downloaded automatically) — only Bazelisk + Node.js needed on host
- Docker solves deployment packaging, not build reproducibility (Bazel already handles that)
- Discord bot requires **Message Content Intent** (privileged) enabled in the Developer Portal — without it, the bot receives empty messages. See `docs/discord-setup.md`

## TODO

- [ ] **Session persistence** — Persist session IDs to disk so sessions survive bridge restarts. On restart, use `-r <session-id>` to resume conversations
- [ ] **Session recovery on restart** — Load persisted session IDs on startup and reconnect to their Discord channels
- [ ] **Path allowlisting** — Validate `working_dir` against an allowlist of base paths before spawning Claude, especially if session creation is ever exposed to Discord users
- [ ] **Bot presence** — Update Discord bot status to show active session count (e.g. `Watching 3 sessions`)

## CI

- GitHub Actions on PRs to `main` and pushes to `main` (automatic, no label required)
- Bazel build, test, and lint
- Coverage enforcement: 90% line-coverage threshold on `internal/` packages (report uploaded as artifact on every run)
- Docker image build verification

## Commands

| Input            | Handler | Description                   |
| ---------------- | ------- | ----------------------------- |
| `/status`        | Bridge  | Show LLM status and idle time |
| `/cancel`        | Bridge  | Send SIGINT to LLM            |
| `/restart`       | Bridge  | Restart LLM process           |
| `/select <repo>` | Bridge  | Select repo for terminal      |
| `/help`          | Bridge  | Show available commands       |
| `::commit`       | LLM     | Translates to `/commit`       |
| text             | LLM     | Raw message to LLM            |

### Dynamic Repo Management

| Input                                      | Handler | Description                        |
| ------------------------------------------ | ------- | ---------------------------------- |
| `/clone <url> <name> <channel-id>`         | Bridge  | Clone a git repo and register it (channel-id required for Discord) |
| `/add-worktree <name> <branch> <channel-id>` | Bridge  | Create worktree from current repo (channel-id required for Discord) |
| `/list-repos`                              | Bridge  | List all configured repos          |
| `/remove-repo <name>`                      | Bridge  | Remove a repo from config          |
| `/worktrees`                               | Bridge  | List git worktrees for current repo|

### Config: defaults.base_dir

The `defaults.base_dir` setting specifies the base directory for cloned repos. Must be an absolute path or "." (defaults to "." if not set).

```yaml
defaults:
  base_dir: /home/user/repos  # absolute path for cloned repos
```
