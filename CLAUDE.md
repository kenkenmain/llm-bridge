# llm-bridge

Go service bridging Discord/Terminal to Claude CLI.

## Development Environment

This project uses **Bazel 8.5.1** for hermetic builds, testing, and linting.
Install [Bazelisk](https://github.com/bazelbuild/bazelisk) (auto-downloads the correct Bazel version).

- **Go 1.23.6** (managed by Bazel via `go_sdk.download` in MODULE.bazel — no local install needed)
- **Claude CLI** (`@anthropic-ai/claude-code`) — runtime dependency, spawned as child process via PTY

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

## Make (shortcuts)

The Makefile wraps Bazel commands for convenience:

```bash
make build     # bazel build //cmd/llm-bridge
make test      # bazel test //...
make lint      # bazel test //:lint
make gazelle   # bazel run //:gazelle
make docker    # full Docker build (base + prod image)
make image     # Bazel OCI image build + load
```

## Bazel Configs

```bash
bazel test //... --config=ci     # CI caching + verbose output
bazel build //... --config=race  # Go race detector
```

## Run

### Local / Bare Metal (preferred for servers you control)

Bazel builds are hermetic — only Bazelisk and Node.js (for Claude CLI) are needed on the host. No Go install required.

```bash
# One-time: install Claude CLI
npm install -g @anthropic-ai/claude-code

# Build and run
bazel build //cmd/llm-bridge
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

## Gotchas

- Lint runs **outside Bazel sandbox** (`no-sandbox` tag) — needs network for first `golangci-lint` download
- Only `claude` LLM backend exists (Codex was removed). Factory defaults empty string to `claude`
- Claude is spawned via PTY (`creack/pty`), not stdin pipe — output parsing depends on terminal behavior
- `llm-bridge.yaml.example` was removed; see `internal/config/` tests for config structure
- Bazel builds are fully hermetic (Go SDK downloaded automatically) — only Bazelisk + Node.js needed on host
- Docker solves deployment packaging, not build reproducibility (Bazel already handles that)

## TODO

- [ ] **Session persistence** — Wrap Claude in tmux (`tmux new-session -d -s claude-{repo} claude`) so sessions survive bridge crashes. Read output via `tmux capture-pane` instead of direct PTY. Inspired by [disclaude/app](https://github.com/disclaude/app)
- [ ] **Session recovery on restart** — Rediscover orphaned tmux sessions on startup and reconnect to their Discord channels (similar to disclaude's `/claude sync`)
- [ ] **Path allowlisting** — Validate `working_dir` against an allowlist of base paths before spawning Claude, especially if session creation is ever exposed to Discord users
- [ ] **Dual access to sessions** — Allow `tmux attach` to a running Claude session while it's also being driven via Discord, for live debugging
- [ ] **Bot presence** — Update Discord bot status to show active session count (e.g. `Watching 3 sessions`)

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
