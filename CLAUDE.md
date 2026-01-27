# llm-bridge

Go service bridging Discord/Telegram/Terminal to Claude/Codex CLI.

## Build

```bash
go build -o llm-bridge ./cmd/llm-bridge
```

## Run

```bash
export DISCORD_BOT_TOKEN=your_token
export TELEGRAM_BOT_TOKEN=your_token
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
- `internal/provider/` - Discord/Telegram/Terminal providers
- `internal/router/` - Command routing (/, !)
- `internal/output/` - Output handling, file attachments

## Features

- **Input merging** - Multiple sources merged to LLM stdin
- **Conflict prefixing** - `[Discord]` prefix when sources collide
- **Output broadcast** - All output sent to ALL connected channels
- **Idle timeout** - LLM process stops after idle period
- **LLM selection** - Configure claude or codex per repo
- **Terminal** - Local stdin/stdout always enabled

## Commands

| Input | Handler | Description |
|-------|---------|-------------|
| `/status` | Bridge | Show LLM status and idle time |
| `/cancel` | Bridge | Send SIGINT to LLM |
| `/restart` | Bridge | Restart LLM process |
| `/select <repo>` | Bridge | Select repo for terminal |
| `/help` | Bridge | Show available commands |
| `!commit` | LLM | Translates to `/commit` |
| text | LLM | Raw message to LLM |
