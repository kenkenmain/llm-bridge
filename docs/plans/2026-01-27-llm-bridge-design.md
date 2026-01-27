# llm-bridge Design

**Date:** 2026-01-27
**Status:** Draft

## Overview

A standalone Go service that bridges Discord/Telegram to Claude/Codex CLI for bidirectional communication. Enables remote task dispatch, mid-task interaction, and parallel terminal access.

## Requirements

1. **Bidirectional communication** - Send tasks via Discord/Telegram, receive responses
2. **Terminal + remote parallel** - Both local terminal and Discord/Telegram can interact simultaneously
3. **One instance per repo** - Persistent Claude/Codex process per configured repository
4. **Skill support** - Claude Code skills accessible via `!` prefix in chat
5. **Dynamic registration** - CLI command to add new repos

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  llm-bridge (per repo)                                       │
│                                                              │
│  ┌─────────────┐                                            │
│  │  Discord/   │◄──┐                                        │
│  │  Telegram   │   │      ┌─────────────┐                   │
│  └─────────────┘   │      │             │                   │
│                    ├─────►│  Claude/    │                   │
│  ┌─────────────┐   │      │  Codex CLI  │                   │
│  │  Local      │◄──┘      │  (PTY)      │                   │
│  │  Terminal   │          │             │                   │
│  └─────────────┘          └─────────────┘                   │
│                                                              │
│  • Inputs merged to stdin    • Outputs broadcast to all     │
│  • Prefix on conflict only   • File attachment for long out │
│  • !cmd → /cmd translation   • Persistent session (resume)  │
└──────────────────────────────────────────────────────────────┘
```

## Design Decisions

### 1. Input Handling

- All inputs (Discord, Telegram, Terminal) merged into Claude's stdin
- **Prefix only on conflict** - When multiple sources send messages concurrently:
    - `[Discord] message`
    - `[Terminal] message`
- Single source active → no prefix

### 2. Output Handling

- All Claude output broadcast to ALL connected channels
- **Long output threshold** (default 1500 chars):
    - Below threshold → send as message
    - Above threshold → upload as `.md` file attachment

### 3. LLM Process Management

- **Support both** Claude CLI and Codex CLI (configurable per repo)
- **Implement Claude first**, Codex later
- **Persistent session** - Use `claude --resume` to maintain context across spawns
- Process spawned on first message, exits when idle (configurable timeout)

### 4. Command Routing

| Prefix | Handler                    | Examples                            |
| ------ | -------------------------- | ----------------------------------- |
| `/`    | Bridge                     | `/status`, `/cancel`, `/restart`    |
| `!`    | Claude (translated to `/`) | `!commit` → `/commit`, `!review-pr` |
| (none) | Claude (raw message)       | "refactor auth module"              |

**Bridge commands:**

- `/status` - Is Claude running? Current state?
- `/cancel` - Interrupt current task (SIGINT)
- `/restart` - Restart Claude process

### 5. Configuration

**File:** `llm-bridge.yaml`

```yaml
repos:
    notification-hooks:
        provider: discord
        channel_id: "123456789"
        llm: claude
        working_dir: /root/notification-hooks

    llm-flow:
        provider: telegram
        channel_id: "987654321"
        llm: codex
        working_dir: /root/llm-flow

defaults:
    llm: claude
    output_threshold: 1500
    idle_timeout: 10m
    resume_session: true

providers:
    discord:
        bot_token: "${DISCORD_BOT_TOKEN}"
    telegram:
        bot_token: "${TELEGRAM_BOT_TOKEN}"
```

### 6. Repo Registration

**CLI command:**

```bash
llm-bridge add-repo <name> \
  --provider discord \
  --channel 123456789 \
  --llm claude \
  --dir /path/to/repo
```

Writes to `llm-bridge.yaml`.

## Project Structure

```
llm-bridge/
├── cmd/
│   └── llm-bridge/
│       └── main.go           # Entry point, CLI commands
├── internal/
│   ├── bridge/
│   │   └── bridge.go         # Core bridge logic
│   ├── config/
│   │   └── config.go         # YAML config parsing
│   ├── llm/
│   │   ├── llm.go            # LLM interface
│   │   ├── claude.go         # Claude CLI wrapper
│   │   └── codex.go          # Codex CLI wrapper (later)
│   ├── provider/
│   │   ├── provider.go       # Provider interface
│   │   ├── discord.go        # Discord bot
│   │   └── telegram.go       # Telegram bot
│   ├── router/
│   │   └── router.go         # Command routing (/, !)
│   └── output/
│       └── output.go         # Output handling, file attachments
├── .github/
│   ├── workflows/ci.yml
│   ├── ISSUE_TEMPLATE/
│   └── PULL_REQUEST_TEMPLATE.md
├── Makefile
├── go.mod
├── llm-bridge.yaml.example
├── CLAUDE.md
└── README.md
```

## Message Flow

### Task Dispatch (Discord → Claude)

```
1. User sends "refactor auth module" in Discord #notification-hooks
2. Bridge receives message, looks up repo config
3. If Claude not running → spawn with --resume
4. Send message to Claude's stdin
5. Claude processes, outputs to stdout
6. Bridge captures output:
   - If < 1500 chars → send to Discord + Terminal
   - If >= 1500 chars → upload as file.md
```

### Skill Invocation (Discord → Claude)

```
1. User sends "!commit" in Discord
2. Bridge detects ! prefix
3. Translates to "/commit"
4. Sends "/commit" to Claude's stdin
5. Claude CLI handles skill loading
6. Output broadcast as above
```

### Bridge Command (Discord → Bridge)

```
1. User sends "/status" in Discord
2. Bridge intercepts (does not forward to Claude)
3. Bridge responds: "Claude: running, idle for 2m"
```

### Concurrent Input

```
1. Discord: "add logging"
2. Terminal (same time): "use slog"
3. Bridge merges with prefix:
   → "[Discord] add logging"
   → "[Terminal] use slog"
4. Claude sees both, addresses both
```

## Security Considerations

1. **Bot tokens** - Env vars, not in config file
2. **Channel validation** - Only respond to configured channels
3. **Working directory isolation** - Claude runs in repo's working_dir
4. **No shell injection** - Sanitize inputs before CLI args

## Error Handling

- **Claude crashes** - Log error, notify channels, respawn on next message
- **Provider disconnect** - Reconnect with backoff, buffer messages
- **Config errors** - Fail fast on startup, clear error messages
- **Long-running task** - Periodic status updates to prevent timeout perception

## Future Extensions

1. **Codex support** - Second LLM backend
2. **Web UI** - Browser-based terminal alongside Discord
3. **Multi-user** - User authentication, per-user sessions
4. **Webhooks** - HTTP callbacks for integrations
5. **Message history** - Persist conversation for search/reference
