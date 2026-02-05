# llm-bridge Feature Porting TODO

Features to port from [zebbern/claude-code-discord](https://github.com/zebbern/claude-code-discord) to [llm-bridge](https://github.com/anthropics/llm-bridge).

## Architecture Note

| Aspect | zebbern (TypeScript/Deno) | llm-bridge (Go) |
|--------|---------------------------|-----------------|
| **Auth** | APPLICATION_ID | CHANNEL_ID |
| **Routing** | Dynamic channel creation | Pre-configured channel→repo mapping |
| **PTY** | Piped I/O (no PTY) | True PTY via `creack/pty` |
| **ANSI** | No ANSI escapes | Raw ANSI output (needs stripping) |

**Key Difference:** zebbern uses Discord APPLICATION_ID for bot authentication with dynamic channel creation within a category. llm-bridge uses CHANNEL_ID as the primary routing key with pre-configured repos.

## PTY vs Piped I/O Comparison

Why does llm-bridge use PTY while zebbern uses pipes? Here's the trade-off:

| Aspect | Piped I/O (zebbern) | PTY (llm-bridge) |
|--------|---------------------|------------------|
| **Output** | Clean text, no escapes | Raw ANSI sequences |
| **Complexity** | Simple | Needs ANSI stripping |
| **Signal handling** | Manual process kill | `SIGINT` propagates naturally |
| **Terminal detection** | CLI sees non-TTY | CLI sees real terminal |
| **Interactive features** | Disabled/limited | Full support (menus, prompts) |
| **Colors/formatting** | Often disabled | Full terminal colors |
| **Cross-platform** | Consistent | Windows PTY is newer |
| **Claude CLI behavior** | Basic output mode | Rich output mode |

### Verdict: PTY is better for Claude CLI

**Reasons:**
1. **Signal propagation** - `/cancel` needs `SIGINT` to reach Claude's process group
2. **Rich output** - Claude CLI detects TTY and enables better formatting
3. **Interactive features** - Session resume, prompts, progress indicators work
4. **Terminal behavior** - Line buffering, cursor control behave correctly

**The fix:** Don't switch to pipes — strip ANSI escapes before sending to Discord:

```go
import "github.com/charmbracelet/x/ansi"

cleaned := ansi.Strip(rawPtyOutput)  // Remove escape sequences
provider.Send(channelID, cleaned)     // Send clean text to Discord
```

**zebbern's approach** avoids ANSI complexity by not using PTY, but loses terminal features that Claude CLI expects.

---

## Priority Legend

- **P1:** Critical - Blocking issues or high-value features
- **P2:** High - Important features for parity
- **P3:** Medium - Nice-to-have features
- **P4:** Low - Future consideration

---

## Already Implemented in llm-bridge ✅

- [x] Discord bot integration (`discordgo`)
- [x] Terminal provider (stdin/stdout)
- [x] `/status` - Show LLM status and idle time
- [x] `/cancel` - Send SIGINT to LLM
- [x] `/restart` - Restart LLM process
- [x] `/help` - Show available commands
- [x] `/clone <url> <name> <channel-id>` - Clone and register repo
- [x] `/add-worktree <name> <branch> <channel-id>` - Create git worktree
- [x] `/worktrees` - List git worktrees
- [x] `/list-repos` - List configured repos
- [x] `/remove-repo <name>` - Unregister repo
- [x] `/select <repo>` - Terminal repo selection
- [x] Rate limiting (per-user, per-channel token bucket)
- [x] Large output as file attachment (>1500 chars)
- [x] Git branch display in status
- [x] Session resume (`--resume` flag)
- [x] Idle timeout auto-stop (10m default)
- [x] `::skill` → `/skill` translation for LLM

---

## P1: Critical 🔴

### ANSI Escape Sequence Handling

**Issue:** PTY output contains ANSI escape sequences (colors, cursor movement) that appear as garbled text in Discord.

**Solution:** Strip ANSI escapes before sending to Discord.

| Item | Details |
|------|---------|
| Files | `internal/output/output.go`, `go.mod` |
| Go Library | `github.com/charmbracelet/x/ansi` → `ansi.Strip()` |
| Test | Capture Claude output sample, verify clean output |

```go
import "github.com/charmbracelet/x/ansi"
cleaned := ansi.Strip(rawPtyOutput)
```

### Todo Management (`/todos`)

**zebbern implementation:**
- Actions: list, add, complete, generate, prioritize
- Priority levels: low, medium, high, critical
- Persistence: JSON file (`todos.json` in `.bot-data/`)
- Data: id, text, priority, completed, source, line, timestamps

**llm-bridge implementation:**

- [ ] `/todos list` - List all tracked todos
- [ ] `/todos add <text> [priority]` - Add new todo (default: medium)
- [ ] `/todos complete <id>` - Mark todo complete
- [ ] `/todos delete <id>` - Remove todo
- [ ] `/todos generate` - Ask Claude to generate todos from context
- [ ] `/todos prioritize` - Ask Claude to prioritize todos

| Item | Details |
|------|---------|
| Files | `internal/todo/todo.go`, `internal/todo/todo_test.go` |
| Persistence | JSON at `{working_dir}/.llm-bridge/todos.json` |
| Data Structure | Same as zebbern: id, text, priority, completed, timestamps |
| Router | Add to `BridgeCommands` map in `internal/router/router.go` |
| Handler | Add switch cases in `internal/bridge/bridge.go` |

---

## P2: High 🟠

### Shell Command Execution (`/shell`)

**zebbern implementation:**
- `/shell <cmd>` - Execute command
- `/shell-input <id> <text>` - Send input to process
- `/shell-list` - List running sessions
- `/shell-kill <id>` - Terminate session
- Uses `Deno.Command` with piped I/O (NOT PTY)
- Cross-platform: python3→python, ls→dir on Windows

**llm-bridge implementation:**

- [ ] `/shell <cmd>` - Execute shell command
- [ ] `/shell-input <id> <text>` - Send stdin to running process
- [ ] `/shell-list` - List active shell sessions
- [ ] `/shell-kill <id>` - Terminate shell session

| Item | Details |
|------|---------|
| Files | `internal/shell/shell.go`, `internal/shell/shell_test.go` |
| Go Library | `os/exec` (stdlib) |
| Session Tracking | Map with unique IDs, start time, command |
| Security | Command allowlist, path restrictions, timeouts |

**Security Considerations:**
- [ ] Allowlist of safe commands (git, ls, cat, etc.)
- [ ] Working directory restrictions (repo root only)
- [ ] Execution timeout (30s default)
- [ ] Resource limits (memory, file descriptors)
- [ ] User permission levels (admin-only destructive commands)

### Claude Enhanced Modes

**zebbern features:**
- Thinking modes: none, think, think-hard, ultrathink
- Operation modes: normal, plan, auto-accept, danger
- `/claude-enhanced` command with options

**Research needed:**
- [ ] Verify Claude CLI supports `--thinking-mode` flag
- [ ] Document all CLI flags for enhanced operation
- [ ] `/claude-mode <mode>` - Switch mode per-session

---

## P3: Medium 🟡

### System Information Commands

**zebbern commands:**
- `/system-info` - OS, CPU, memory, kernel
- `/processes` - Running processes with filter/limit
- `/system-resources` - Real-time CPU, memory, load
- `/network-info` - Interfaces, connections, routing
- `/disk-usage` - Filesystem space percentages
- `/uptime` - System uptime and boot info

**llm-bridge implementation:**

- [ ] `/system-info` - OS and hardware summary
- [ ] `/processes [filter]` - Top processes
- [ ] `/disk-usage` - Disk space summary
- [ ] `/network-info` - Network interfaces
- [ ] `/uptime` - System uptime

| Item | Details |
|------|---------|
| Files | `internal/sysinfo/sysinfo.go`, tests |
| Go Library | `github.com/shirou/gopsutil/v3` |

### Dynamic Channel Creation

**zebbern approach:**
- Creates channels automatically for new repos
- Organizes in category (CATEGORY_NAME env var)
- Requires: Manage Channels permission

**llm-bridge consideration:**
- [ ] Auto-create channel on `/clone` if channel-id not provided
- [ ] Use DISCORD_CATEGORY_ID for organization
- [ ] Update `internal/provider/discord.go`

---

## P4: Low (Future) 🟢

### Agent System

**zebbern has 7 specialized agents:**
1. Code Reviewer
2. Security Analyst
3. DevOps Engineer
4. Performance Expert
5. Documentation Writer
6. Test Engineer
7. Architect

**Complexity:** High - needs separate design document

- [ ] Agent orchestration framework
- [ ] Per-agent system prompts
- [ ] Tool access restrictions per agent
- [ ] Isolated context windows

### Additional zebbern Features

- [ ] `/screenshot` - Capture host screen (local/GUI only)
- [ ] `/port-scan` - Check open ports
- [ ] `/service-status` - Systemd service states
- [ ] `/system-logs` - Recent system logs
- [ ] `/env-vars` - Environment variables (filtered)
- [ ] MCP server management (`/mcp`)

---

## Go Library Alternatives

| zebbern (Deno/TypeScript) | Go Equivalent | Package |
|---------------------------|---------------|---------|
| `Deno.Command` | `os/exec` | stdlib |
| `Deno.readTextFile` | `os.ReadFile` | stdlib |
| `Deno.writeTextFile` | `os.WriteFile` | stdlib |
| `JSON.parse/stringify` | `encoding/json` | stdlib |
| discord.js | discordgo | `github.com/bwmarrin/discordgo` |
| (No PTY) | creack/pty | `github.com/creack/pty` ✅ |
| (No ANSI parsing) | charmbracelet/x/ansi | `github.com/charmbracelet/x/ansi` |
| (No system stats) | gopsutil | `github.com/shirou/gopsutil/v3` |
| AbortController | context.Context | stdlib |
| setTimeout | time.After | stdlib |
| Map/Set | map/struct | stdlib |

---

## Testing Requirements

All new features must:
- [ ] Maintain 90% coverage threshold on `internal/` packages
- [ ] Include unit tests with table-driven patterns
- [ ] Use hand-written mocks (no testify)
- [ ] Use `t.TempDir()` for file fixtures

---

## References

- [zebbern/claude-code-discord](https://github.com/zebbern/claude-code-discord) - Source project
- [charmbracelet/x/ansi](https://pkg.go.dev/github.com/charmbracelet/x/ansi) - ANSI escape handling
- [leaanthony/go-ansi-parser](https://github.com/leaanthony/go-ansi-parser) - ANSI to structured data
- [shirou/gopsutil](https://github.com/shirou/gopsutil) - Cross-platform system stats
- [Claude Code CLI](https://code.claude.com/docs/en/cli-reference) - CLI flags reference

---

*Generated by llm-bridge minions workflow*
*Date: 2026-02-05*
