# llm-bridge Parity TODO (vs claude-code-discord)

Source baseline:
- https://github.com/zebbern/claude-code-discord

Last reviewed:
- 2026-02-07

Scope:
- Track feature parity gaps and quality-of-life improvements relative to `claude-code-discord`.
- Focus on current `llm-bridge` architecture (Discord-first, no local terminal provider).

---

## Recently Applied (This Branch)

- [x] Added Codex backend implementation and wiring (`llm: codex`, `defaults.codex_path`)
- [x] Removed terminal provider code and tests entirely
- [x] Removed `/select` as a bridge command; `/select ...` now routes to LLM as plain slash input
- [x] Restricted dynamic repo/worktree flows to Discord provider
- [x] Enforced explicit `channel-id` in `/clone` and `/add-worktree`
- [x] Updated docs/config examples to reflect Discord-only operation
- [x] Full `bazel test //...` passes after removal/refactor

---

## Current State Snapshot

Current architecture:
- Discord provider only
- Repo-per-channel routing via config
- PTY-backed LLM sessions (`claude` and `codex`)
- Session lifecycle commands (`/status`, `/cancel`, `/restart`)
- Dynamic repo/worktree registration (`/clone`, `/add-worktree`)
- Rate limiting, idle timeout, output attachment fallback

Current bridge commands:
- `/help`
- `/status`
- `/cancel`
- `/restart`
- `/worktrees`
- `/list-repos`
- `/clone <url> <name> <channel-id>`
- `/add-worktree <name> <branch> <channel-id>`
- `/remove-repo <name>`

Current config highlights:
- `defaults.llm` supports `claude` and `codex`
- `defaults.claude_path`
- `defaults.codex_path`
- `defaults.idle_timeout`
- `defaults.rate_limit`

---

## Implemented Parity Items

- [x] Discord bot integration
- [x] Command routing (`/` bridge commands, `::` passthrough translation)
- [x] LLM lifecycle controls (`/status`, `/cancel`, `/restart`)
- [x] Dynamic repo registration (`/clone`)
- [x] Worktree creation and listing (`/add-worktree`, `/worktrees`)
- [x] Repo introspection (`/list-repos`, `/remove-repo`)
- [x] Provider guardrails (Discord-only for repo/worktree mutation commands)
- [x] Per-user and per-channel rate limiting
- [x] Idle timeout auto-stop
- [x] File attachment for large outputs
- [x] Session resume support
- [x] Codex backend support
- [x] Terminal provider fully removed from runtime and tests

---

## Priority Plan

## P0 (Critical)

### 1) ANSI/control-sequence sanitization for Discord output

Problem:
- PTY output can contain ANSI/control sequences and render poorly in Discord.

Tasks:
- [ ] Strip ANSI escapes before provider send
- [ ] Remove control chars unsafe for Discord rendering
- [ ] Keep raw output available for debugging (optional toggle)
- [ ] Add golden tests with real captured PTY fragments

Suggested files:
- `internal/output/output.go`
- `internal/bridge/bridge.go`
- `internal/output/output_test.go`

Acceptance criteria:
- Discord never receives visible escape artifacts
- Existing file-attachment behavior remains unchanged

### 2) Structured response formatting (tool-use aware)

Problem:
- Raw stream forwarding misses QoL formatting available in `claude-code-discord`.

Tasks:
- [ ] Build output normalizer that classifies content into sections:
  - assistant response
  - tool invocation
  - tool result
  - status/progress
- [ ] Provider-specific renderer for Discord markdown blocks
- [ ] Fallback to cleaned raw text when parser confidence is low

Suggested files:
- `internal/output/output.go`
- `internal/output/output_test.go`
- `internal/bridge/bridge.go`

Acceptance criteria:
- Tool invocations/results are visibly separated in Discord output
- No regression in plain-text responses

---

## P1 (High)

### 3) `/todos` parity

Target commands:
- [ ] `/todos list`
- [ ] `/todos add <text> [priority]`
- [ ] `/todos complete <id>`
- [ ] `/todos delete <id>`
- [ ] `/todos generate`
- [ ] `/todos prioritize`

Data model:
- [ ] id, text, priority, completed, timestamps
- [ ] JSON persistence under repo-scoped bridge data directory

Suggested files:
- `internal/todo/todo.go`
- `internal/todo/todo_test.go`
- `internal/router/router.go`
- `internal/bridge/bridge.go`

Acceptance criteria:
- Todos persist across restarts
- Concurrency-safe updates
- Full command coverage with tests

### 4) Dynamic Discord channel creation

Goal:
- Allow `/clone` without explicit channel ID by creating channel automatically.

Tasks:
- [ ] Add optional auto-channel mode in config
- [ ] Create channel under configured category
- [ ] Persist created channel ID to config
- [ ] Add permission/error handling (`Manage Channels`)

Suggested files:
- `internal/provider/discord.go`
- `internal/bridge/bridge.go`
- `internal/config/config.go`

Acceptance criteria:
- `/clone` can succeed with generated channel in auto mode
- Failure paths are explicit and non-destructive

### 5) `/shell*` command family

Target commands:
- [ ] `/shell <cmd>`
- [ ] `/shell-input <id> <text>`
- [ ] `/shell-list`
- [ ] `/shell-kill <id>`

Security baseline:
- [ ] allowlist or policy engine
- [ ] timeouts
- [ ] repo-root path constraints
- [ ] explicit audit logging

Acceptance criteria:
- Multi-session process management works
- Security gates block unsafe commands by default

---

## P2 (Medium)

### 6) Host/system observability commands

Candidate commands:
- [ ] `/system-info`
- [ ] `/processes [filter]`
- [ ] `/disk-usage`
- [ ] `/network-info`
- [ ] `/uptime`

Notes:
- Use `gopsutil` where practical
- Keep output concise to avoid Discord flooding

### 7) Enhanced model modes

Tasks:
- [ ] Per-session mode controls for Claude/Codex backends
- [ ] Configurable flags/presets (plan/auto-accept/etc. where supported)
- [ ] Explicit backend capability matrix in docs

---

## P3 (Future)

### 8) Agent orchestration layer

- [ ] Define specialized agent profiles
- [ ] Per-agent tool restrictions
- [ ] Isolation and context boundaries
- [ ] Auditable orchestration flow

### 9) Additional operational commands

- [ ] `/service-status`
- [ ] `/system-logs`
- [ ] `/env-vars` (safe filtered view)
- [ ] MCP management command surface

---

## Not Planned / Removed

- [x] Terminal stdin/stdout provider removed
- [x] `/select` bridge command removed

Reason:
- Product direction is Discord-first operation.

---

## Engineering Quality Gates

For each shipped feature:
- [ ] Unit tests with table-driven coverage
- [ ] Maintain `internal/` coverage threshold
- [ ] No unsafe defaults for commands with host impact
- [ ] Clear user-facing error messages for permission/config failures

---

## References

- `claude-code-discord`: https://github.com/zebbern/claude-code-discord
- ANSI stripping: https://pkg.go.dev/github.com/charmbracelet/x/ansi
- System stats: https://github.com/shirou/gopsutil
- Claude CLI reference: https://code.claude.com/docs/en/cli-reference
