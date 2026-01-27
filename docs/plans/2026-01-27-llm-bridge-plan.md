# llm-bridge Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go service that bridges Discord/Telegram/Terminal to Claude CLI for bidirectional communication with persistent sessions, input merging, and output broadcasting.

**Architecture:** PTY-based Claude process management with provider abstraction for Discord/Telegram/Terminal. Input merging from multiple sources with conflict prefixing, output broadcasting to ALL channels. Command routing separates bridge commands (/) from Claude skills (!). Idle timeout for process lifecycle.

**Tech Stack:** Go 1.21+, discordgo, telebot, creack/pty, gopkg.in/yaml.v3, cobra (CLI)

---

## Tasks (Task API Format)

```yaml
tasks:
  - id: 1
    description: "Initialize Go module"
    subagent_type: "Bash"
    prompt: |
      Initialize the Go module and create basic project structure:

      cd /root/llm-bridge
      go mod init github.com/anthropics/llm-bridge
      mkdir -p cmd/llm-bridge internal/{bridge,config,llm,provider,router,output}
      ls -la
    dependencies: []

  - id: 2
    description: "Create config types"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/config/config.go with YAML configuration types.

      File: internal/config/config.go

      ```go
      package config

      import (
          "fmt"
          "os"
          "path/filepath"
          "time"

          "gopkg.in/yaml.v3"
      )

      type Config struct {
          Repos     map[string]RepoConfig `yaml:"repos"`
          Defaults  Defaults              `yaml:"defaults"`
          Providers ProviderConfigs       `yaml:"providers"`
      }

      type RepoConfig struct {
          Provider   string `yaml:"provider"`
          ChannelID  string `yaml:"channel_id"`
          LLM        string `yaml:"llm"`
          WorkingDir string `yaml:"working_dir"`
      }

      type Defaults struct {
          LLM             string `yaml:"llm"`
          ClaudePath      string `yaml:"claude_path"`      // Path to Claude binary (default: "claude")
          OutputThreshold int    `yaml:"output_threshold"`
          IdleTimeout     string `yaml:"idle_timeout"`
          ResumeSession   *bool  `yaml:"resume_session"`
      }

      func (d Defaults) GetClaudePath() string {
          if d.ClaudePath == "" {
              return "claude"
          }
          return d.ClaudePath
      }

      func (d Defaults) GetResumeSession() bool {
          if d.ResumeSession == nil {
              return true // default true
          }
          return *d.ResumeSession
      }

      func (d Defaults) GetIdleTimeoutDuration() time.Duration {
          dur, err := time.ParseDuration(d.IdleTimeout)
          if err != nil {
              return 10 * time.Minute
          }
          return dur
      }

      type ProviderConfigs struct {
          Discord  DiscordConfig  `yaml:"discord"`
          Telegram TelegramConfig `yaml:"telegram"`
      }

      type DiscordConfig struct {
          BotToken string `yaml:"bot_token"`
      }

      type TelegramConfig struct {
          BotToken string `yaml:"bot_token"`
      }

      func Load(path string) (*Config, error) {
          data, err := os.ReadFile(path)
          if err != nil {
              return nil, fmt.Errorf("read config: %w", err)
          }

          // Expand environment variables
          expanded := os.ExpandEnv(string(data))

          var cfg Config
          if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
              return nil, fmt.Errorf("parse config: %w", err)
          }

          // Apply defaults
          if cfg.Defaults.LLM == "" {
              cfg.Defaults.LLM = "claude"
          }
          if cfg.Defaults.OutputThreshold == 0 {
              cfg.Defaults.OutputThreshold = 1500
          }
          if cfg.Defaults.IdleTimeout == "" {
              cfg.Defaults.IdleTimeout = "10m"
          }

          return &cfg, nil
      }

      func DefaultPath() string {
          return filepath.Join(".", "llm-bridge.yaml")
      }
      ```
    dependencies: [1]

  - id: 3
    description: "Create LLM interface"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/llm/llm.go with the LLM interface definition.

      File: internal/llm/llm.go

      ```go
      package llm

      import (
          "context"
          "io"
          "time"
      )

      // Message represents input from a source
      type Message struct {
          Source  string // "discord", "telegram", "terminal"
          Content string
      }

      // LLM defines the interface for LLM backends
      type LLM interface {
          // Start spawns the LLM process
          Start(ctx context.Context) error

          // Stop terminates the LLM process
          Stop() error

          // Send writes a message to the LLM's stdin
          Send(msg Message) error

          // Output returns the reader for LLM output
          Output() io.Reader

          // Running returns true if the LLM process is active
          Running() bool

          // Cancel sends interrupt signal (SIGINT)
          Cancel() error

          // LastActivity returns time of last input/output
          LastActivity() time.Time

          // UpdateActivity updates the last activity timestamp (call on output)
          UpdateActivity()

          // Name returns the LLM backend name ("claude", "codex")
          Name() string
      }
      ```
    dependencies: [1]

  - id: 4
    description: "Create Claude wrapper"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/llm/claude.go implementing the Claude CLI wrapper with PTY.

      File: internal/llm/claude.go

      ```go
      package llm

      import (
          "context"
          "fmt"
          "io"
          "os"
          "os/exec"
          "sync"
          "syscall"
          "time"

          "github.com/creack/pty"
      )

      type Claude struct {
          workingDir    string
          resumeSession bool
          claudePath    string // Path to Claude binary

          mu           sync.Mutex
          cmd          *exec.Cmd
          ptmx         *os.File
          running      bool
          lastActivity time.Time
      }

      type ClaudeOption func(*Claude)

      func WithWorkingDir(dir string) ClaudeOption {
          return func(c *Claude) {
              c.workingDir = dir
          }
      }

      func WithResume(resume bool) ClaudeOption {
          return func(c *Claude) {
              c.resumeSession = resume
          }
      }

      func WithClaudePath(path string) ClaudeOption {
          return func(c *Claude) {
              if path != "" {
                  c.claudePath = path
              }
          }
      }

      func NewClaude(opts ...ClaudeOption) *Claude {
          c := &Claude{
              workingDir:    ".",
              resumeSession: true,
              claudePath:    "claude", // default to PATH lookup
              lastActivity:  time.Now(),
          }
          for _, opt := range opts {
              opt(c)
          }
          return c
      }

      func (c *Claude) Name() string {
          return "claude"
      }

      func (c *Claude) Start(ctx context.Context) error {
          c.mu.Lock()
          defer c.mu.Unlock()

          if c.running {
              return nil
          }

          args := []string{}
          if c.resumeSession {
              args = append(args, "--resume")
          }

          c.cmd = exec.CommandContext(ctx, c.claudePath, args...)
          c.cmd.Dir = c.workingDir
          c.cmd.Env = os.Environ()

          var err error
          c.ptmx, err = pty.Start(c.cmd)
          if err != nil {
              return fmt.Errorf("start pty: %w", err)
          }

          c.running = true
          c.lastActivity = time.Now()

          // Monitor process exit
          go func() {
              c.cmd.Wait()
              c.mu.Lock()
              c.running = false
              c.mu.Unlock()
          }()

          return nil
      }

      func (c *Claude) Stop() error {
          c.mu.Lock()
          defer c.mu.Unlock()

          if !c.running || c.cmd == nil || c.cmd.Process == nil {
              return nil
          }

          // Send SIGTERM first
          if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
              c.cmd.Process.Kill()
          }

          if c.ptmx != nil {
              c.ptmx.Close()
          }

          c.running = false
          return nil
      }

      func (c *Claude) Send(msg Message) error {
          c.mu.Lock()
          defer c.mu.Unlock()

          if !c.running || c.ptmx == nil {
              return fmt.Errorf("claude not running")
          }

          c.lastActivity = time.Now()
          _, err := c.ptmx.WriteString(msg.Content + "\n")
          return err
      }

      func (c *Claude) Output() io.Reader {
          c.mu.Lock()
          defer c.mu.Unlock()
          return c.ptmx
      }

      func (c *Claude) Running() bool {
          c.mu.Lock()
          defer c.mu.Unlock()
          return c.running
      }

      func (c *Claude) Cancel() error {
          c.mu.Lock()
          defer c.mu.Unlock()

          if !c.running || c.cmd == nil || c.cmd.Process == nil {
              return nil
          }

          return c.cmd.Process.Signal(syscall.SIGINT)
      }

      func (c *Claude) LastActivity() time.Time {
          c.mu.Lock()
          defer c.mu.Unlock()
          return c.lastActivity
      }

      func (c *Claude) UpdateActivity() {
          c.mu.Lock()
          defer c.mu.Unlock()
          c.lastActivity = time.Now()
      }
      ```
    dependencies: [3]

  - id: 5
    description: "Create Codex wrapper stub"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/llm/codex.go as a stub for Codex CLI support.

      File: internal/llm/codex.go

      ```go
      package llm

      import (
          "context"
          "fmt"
          "io"
          "time"
      )

      // Codex implements the LLM interface for Codex CLI (stub for future)
      type Codex struct {
          workingDir string
      }

      func NewCodex(workingDir string) *Codex {
          return &Codex{workingDir: workingDir}
      }

      func (c *Codex) Name() string {
          return "codex"
      }

      func (c *Codex) Start(ctx context.Context) error {
          return fmt.Errorf("codex support not yet implemented")
      }

      func (c *Codex) Stop() error {
          return nil
      }

      func (c *Codex) Send(msg Message) error {
          return fmt.Errorf("codex not running")
      }

      func (c *Codex) Output() io.Reader {
          return nil
      }

      func (c *Codex) Running() bool {
          return false
      }

      func (c *Codex) Cancel() error {
          return nil
      }

      func (c *Codex) LastActivity() time.Time {
          return time.Time{}
      }

      func (c *Codex) UpdateActivity() {
          // No-op for stub
      }
      ```
    dependencies: [3]

  - id: 6
    description: "Create LLM factory"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/llm/factory.go for LLM selection based on config.

      File: internal/llm/factory.go

      ```go
      package llm

      import "fmt"

      // New creates an LLM instance based on the backend name
      func New(backend, workingDir, claudePath string, resume bool) (LLM, error) {
          switch backend {
          case "claude", "":
              return NewClaude(
                  WithWorkingDir(workingDir),
                  WithResume(resume),
                  WithClaudePath(claudePath),
              ), nil
          case "codex":
              return NewCodex(workingDir), nil
          default:
              return nil, fmt.Errorf("unknown LLM backend: %s", backend)
          }
      }
      ```
    dependencies: [4, 5]

  - id: 7
    description: "Create provider interface"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/provider/provider.go with the provider interface.

      File: internal/provider/provider.go

      ```go
      package provider

      import (
          "context"
      )

      // Message represents a chat message
      type Message struct {
          ChannelID string
          Content   string
          Author    string
          Source    string // provider name
      }

      // Provider defines the interface for chat providers
      type Provider interface {
          // Name returns the provider name ("discord", "telegram", "terminal")
          Name() string

          // Start connects to the chat service
          Start(ctx context.Context) error

          // Stop disconnects from the chat service
          Stop() error

          // Send sends a message to a channel
          Send(channelID string, content string) error

          // SendFile sends a file to a channel
          SendFile(channelID string, filename string, content []byte) error

          // Messages returns a channel of incoming messages
          Messages() <-chan Message
      }
      ```
    dependencies: [1]

  - id: 8
    description: "Create Discord provider"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/provider/discord.go implementing Discord bot.

      File: internal/provider/discord.go

      ```go
      package provider

      import (
          "bytes"
          "context"
          "fmt"
          "sync"

          "github.com/bwmarrin/discordgo"
      )

      type Discord struct {
          token    string
          channels map[string]bool

          mu       sync.Mutex
          session  *discordgo.Session
          messages chan Message
          stopped  bool
      }

      func NewDiscord(token string, channelIDs []string) *Discord {
          channels := make(map[string]bool)
          for _, id := range channelIDs {
              channels[id] = true
          }
          return &Discord{
              token:    token,
              channels: channels,
              messages: make(chan Message, 100),
          }
      }

      func (d *Discord) Name() string {
          return "discord"
      }

      func (d *Discord) Start(ctx context.Context) error {
          var err error
          d.session, err = discordgo.New("Bot " + d.token)
          if err != nil {
              return fmt.Errorf("create session: %w", err)
          }

          d.session.AddHandler(d.handleMessage)
          d.session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

          if err := d.session.Open(); err != nil {
              return fmt.Errorf("open session: %w", err)
          }

          return nil
      }

      func (d *Discord) Stop() error {
          d.mu.Lock()
          defer d.mu.Unlock()

          if d.stopped {
              return nil
          }
          d.stopped = true

          if d.session != nil {
              d.session.Close()
          }
          close(d.messages)
          return nil
      }

      func (d *Discord) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
          if m.Author.ID == s.State.User.ID {
              return
          }

          if !d.channels[m.ChannelID] {
              return
          }

          d.mu.Lock()
          stopped := d.stopped
          d.mu.Unlock()

          if stopped {
              return
          }

          d.messages <- Message{
              ChannelID: m.ChannelID,
              Content:   m.Content,
              Author:    m.Author.Username,
              Source:    "discord",
          }
      }

      func (d *Discord) Send(channelID string, content string) error {
          if d.session == nil {
              return fmt.Errorf("discord not connected")
          }
          _, err := d.session.ChannelMessageSend(channelID, content)
          return err
      }

      func (d *Discord) SendFile(channelID string, filename string, content []byte) error {
          if d.session == nil {
              return fmt.Errorf("discord not connected")
          }
          _, err := d.session.ChannelFileSend(channelID, filename, bytes.NewReader(content))
          return err
      }

      func (d *Discord) Messages() <-chan Message {
          return d.messages
      }
      ```
    dependencies: [7]

  - id: 9
    description: "Create Telegram provider"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/provider/telegram.go implementing Telegram bot.

      File: internal/provider/telegram.go

      ```go
      package provider

      import (
          "bytes"
          "context"
          "fmt"
          "strconv"
          "sync"
          "time"

          tele "gopkg.in/telebot.v3"
      )

      type Telegram struct {
          token    string
          channels map[string]bool

          mu       sync.Mutex
          bot      *tele.Bot
          messages chan Message
          stopped  bool
      }

      func NewTelegram(token string, channelIDs []string) *Telegram {
          channels := make(map[string]bool)
          for _, id := range channelIDs {
              channels[id] = true
          }
          return &Telegram{
              token:    token,
              channels: channels,
              messages: make(chan Message, 100),
          }
      }

      func (t *Telegram) Name() string {
          return "telegram"
      }

      func (t *Telegram) Start(ctx context.Context) error {
          pref := tele.Settings{
              Token:  t.token,
              Poller: &tele.LongPoller{Timeout: 10 * time.Second},
          }

          var err error
          t.bot, err = tele.NewBot(pref)
          if err != nil {
              return fmt.Errorf("create bot: %w", err)
          }

          t.bot.Handle(tele.OnText, func(c tele.Context) error {
              chatID := strconv.FormatInt(c.Chat().ID, 10)

              if !t.channels[chatID] {
                  return nil
              }

              t.mu.Lock()
              stopped := t.stopped
              t.mu.Unlock()

              if stopped {
                  return nil
              }

              t.messages <- Message{
                  ChannelID: chatID,
                  Content:   c.Text(),
                  Author:    c.Sender().Username,
                  Source:    "telegram",
              }
              return nil
          })

          go t.bot.Start()
          return nil
      }

      func (t *Telegram) Stop() error {
          t.mu.Lock()
          defer t.mu.Unlock()

          if t.stopped {
              return nil
          }
          t.stopped = true

          if t.bot != nil {
              t.bot.Stop()
          }
          close(t.messages)
          return nil
      }

      func (t *Telegram) Send(channelID string, content string) error {
          if t.bot == nil {
              return fmt.Errorf("telegram not connected")
          }

          chatID, err := strconv.ParseInt(channelID, 10, 64)
          if err != nil {
              return fmt.Errorf("invalid chat ID: %w", err)
          }

          _, err = t.bot.Send(&tele.Chat{ID: chatID}, content)
          return err
      }

      func (t *Telegram) SendFile(channelID string, filename string, content []byte) error {
          if t.bot == nil {
              return fmt.Errorf("telegram not connected")
          }

          chatID, err := strconv.ParseInt(channelID, 10, 64)
          if err != nil {
              return fmt.Errorf("invalid chat ID: %w", err)
          }

          doc := &tele.Document{
              File:     tele.FromReader(bytes.NewReader(content)),
              FileName: filename,
          }
          _, err = t.bot.Send(&tele.Chat{ID: chatID}, doc)
          return err
      }

      func (t *Telegram) Messages() <-chan Message {
          return t.messages
      }
      ```
    dependencies: [7]

  - id: 10
    description: "Create Terminal provider"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/provider/terminal.go for local terminal input/output.

      File: internal/provider/terminal.go

      ```go
      package provider

      import (
          "bufio"
          "context"
          "fmt"
          "io"
          "os"
          "sync"
      )

      // Terminal provides local stdin/stdout as a provider
      type Terminal struct {
          channelID string // virtual channel ID for terminal
          reader    io.Reader
          writer    io.Writer

          mu       sync.Mutex
          messages chan Message
          stopped  bool
      }

      func NewTerminal(channelID string) *Terminal {
          return &Terminal{
              channelID: channelID,
              reader:    os.Stdin,
              writer:    os.Stdout,
              messages:  make(chan Message, 100),
          }
      }

      func (t *Terminal) Name() string {
          return "terminal"
      }

      func (t *Terminal) Start(ctx context.Context) error {
          go t.readLoop(ctx)
          return nil
      }

      func (t *Terminal) readLoop(ctx context.Context) {
          scanner := bufio.NewScanner(t.reader)
          for scanner.Scan() {
              select {
              case <-ctx.Done():
                  return
              default:
              }

              t.mu.Lock()
              stopped := t.stopped
              t.mu.Unlock()

              if stopped {
                  return
              }

              t.messages <- Message{
                  ChannelID: t.channelID,
                  Content:   scanner.Text(),
                  Author:    "terminal",
                  Source:    "terminal",
              }
          }
      }

      func (t *Terminal) Stop() error {
          t.mu.Lock()
          defer t.mu.Unlock()

          if t.stopped {
              return nil
          }
          t.stopped = true
          close(t.messages)
          return nil
      }

      func (t *Terminal) Send(channelID string, content string) error {
          _, err := fmt.Fprintln(t.writer, content)
          return err
      }

      func (t *Terminal) SendFile(channelID string, filename string, content []byte) error {
          fmt.Fprintf(t.writer, "--- %s ---\n%s\n--- end ---\n", filename, string(content))
          return nil
      }

      func (t *Terminal) Messages() <-chan Message {
          return t.messages
      }

      func (t *Terminal) ChannelID() string {
          return t.channelID
      }
      ```
    dependencies: [7]

  - id: 11
    description: "Create command router"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/router/router.go for command routing logic.

      File: internal/router/router.go

      ```go
      package router

      import (
          "strings"
      )

      type RouteType int

      const (
          RouteToLLM RouteType = iota
          RouteToBridge
      )

      type Route struct {
          Type    RouteType
          Command string
          Args    string
          Raw     string
      }

      var BridgeCommands = map[string]bool{
          "status":  true,
          "cancel":  true,
          "restart": true,
          "help":    true,
          "select":  true,
      }

      func Parse(content string) Route {
          content = strings.TrimSpace(content)

          if strings.HasPrefix(content, "/") {
              cmd, args := parseCommand(content[1:])
              if BridgeCommands[cmd] {
                  return Route{
                      Type:    RouteToBridge,
                      Command: cmd,
                      Args:    args,
                      Raw:     content,
                  }
              }
              return Route{
                  Type: RouteToLLM,
                  Raw:  content,
              }
          }

          if strings.HasPrefix(content, "!") {
              translated := "/" + content[1:]
              return Route{
                  Type: RouteToLLM,
                  Raw:  translated,
              }
          }

          return Route{
              Type: RouteToLLM,
              Raw:  content,
          }
      }

      func parseCommand(s string) (cmd, args string) {
          parts := strings.SplitN(s, " ", 2)
          cmd = strings.ToLower(parts[0])
          if len(parts) > 1 {
              args = parts[1]
          }
          return
      }
      ```
    dependencies: [1]

  - id: 12
    description: "Create output handler"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/output/output.go for output handling.

      File: internal/output/output.go

      ```go
      package output

      import (
          "fmt"
          "time"
      )

      type Handler struct {
          threshold int
      }

      func NewHandler(threshold int) *Handler {
          if threshold <= 0 {
              threshold = 1500
          }
          return &Handler{threshold: threshold}
      }

      func (h *Handler) ShouldAttach(content string) bool {
          return len(content) > h.threshold
      }

      func (h *Handler) FormatFile(content string) (filename string, data []byte) {
          filename = fmt.Sprintf("response-%s.md", time.Now().Format("150405"))
          data = []byte(content)
          return
      }

      func (h *Handler) Truncate(content string, maxLen int) string {
          if len(content) <= maxLen {
              return content
          }
          return content[:maxLen-3] + "..."
      }
      ```
    dependencies: [1]

  - id: 13
    description: "Create input merger"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/bridge/merger.go for input conflict detection and prefixing.

      File: internal/bridge/merger.go

      ```go
      package bridge

      import (
          "fmt"
          "sync"
          "time"
      )

      // Merger handles input conflict detection and prefixing
      type Merger struct {
          mu          sync.Mutex
          lastSource  string
          lastTime    time.Time
          conflictWin time.Duration
      }

      func NewMerger(conflictWindow time.Duration) *Merger {
          if conflictWindow <= 0 {
              conflictWindow = 2 * time.Second
          }
          return &Merger{
              conflictWin: conflictWindow,
          }
      }

      // FormatMessage adds source prefix only if there's a conflict
      // (multiple sources sending within the conflict window)
      func (m *Merger) FormatMessage(source, content string) string {
          m.mu.Lock()
          defer m.mu.Unlock()

          now := time.Now()
          needsPrefix := false

          // Check if different source within conflict window
          if m.lastSource != "" && m.lastSource != source {
              if now.Sub(m.lastTime) < m.conflictWin {
                  needsPrefix = true
              }
          }

          m.lastSource = source
          m.lastTime = now

          if needsPrefix {
              return fmt.Sprintf("[%s] %s", source, content)
          }
          return content
      }
      ```
    dependencies: [1]

  - id: 14
    description: "Create bridge core"
    subagent_type: "general-purpose"
    prompt: |
      Create internal/bridge/bridge.go with core bridge logic including input merging, output broadcasting, idle timeout, and nil session handling.

      File: internal/bridge/bridge.go

      ```go
      package bridge

      import (
          "bufio"
          "context"
          "fmt"
          "log/slog"
          "sync"
          "time"

          "github.com/anthropics/llm-bridge/internal/config"
          "github.com/anthropics/llm-bridge/internal/llm"
          "github.com/anthropics/llm-bridge/internal/output"
          "github.com/anthropics/llm-bridge/internal/provider"
          "github.com/anthropics/llm-bridge/internal/router"
      )

      type Bridge struct {
          cfg       *config.Config
          providers map[string]provider.Provider
          repos     map[string]*repoSession
          output    *output.Handler
          merger    *Merger

          mu               sync.Mutex
          terminalRepoName string // currently selected repo for terminal
      }

      type repoSession struct {
          name      string
          llm       llm.LLM
          channels  []channelRef // all channels to broadcast to
          cancelCtx context.CancelFunc
      }

      type channelRef struct {
          provider  provider.Provider
          channelID string
      }

      func New(cfg *config.Config) *Bridge {
          return &Bridge{
              cfg:       cfg,
              providers: make(map[string]provider.Provider),
              repos:     make(map[string]*repoSession),
              output:    output.NewHandler(cfg.Defaults.OutputThreshold),
              merger:    NewMerger(2 * time.Second),
          }
      }

      func (b *Bridge) Start(ctx context.Context) error {
          // Initialize Discord if configured
          if b.cfg.Providers.Discord.BotToken != "" {
              channelIDs := b.channelIDsForProvider("discord")
              if len(channelIDs) > 0 {
                  discord := provider.NewDiscord(b.cfg.Providers.Discord.BotToken, channelIDs)
                  if err := discord.Start(ctx); err != nil {
                      return fmt.Errorf("start discord: %w", err)
                  }
                  b.providers["discord"] = discord
                  go b.handleMessages(ctx, discord)
                  slog.Info("discord provider started", "channels", len(channelIDs))
              }
          }

          // Initialize Telegram if configured
          if b.cfg.Providers.Telegram.BotToken != "" {
              channelIDs := b.channelIDsForProvider("telegram")
              if len(channelIDs) > 0 {
                  telegram := provider.NewTelegram(b.cfg.Providers.Telegram.BotToken, channelIDs)
                  if err := telegram.Start(ctx); err != nil {
                      return fmt.Errorf("start telegram: %w", err)
                  }
                  b.providers["telegram"] = telegram
                  go b.handleMessages(ctx, telegram)
                  slog.Info("telegram provider started", "channels", len(channelIDs))
              }
          }

          // Initialize Terminal (always enabled for local interaction)
          terminal := provider.NewTerminal("terminal")
          if err := terminal.Start(ctx); err != nil {
              return fmt.Errorf("start terminal: %w", err)
          }
          b.providers["terminal"] = terminal
          go b.handleTerminalMessages(ctx, terminal)
          slog.Info("terminal provider started")

          // Start idle timeout checker
          go b.idleTimeoutLoop(ctx)

          <-ctx.Done()
          return b.Stop()
      }

      func (b *Bridge) Stop() error {
          b.mu.Lock()
          defer b.mu.Unlock()

          for _, repo := range b.repos {
              if repo.llm != nil {
                  repo.llm.Stop()
              }
              if repo.cancelCtx != nil {
                  repo.cancelCtx()
              }
          }

          for _, prov := range b.providers {
              prov.Stop()
          }

          return nil
      }

      func (b *Bridge) channelIDsForProvider(providerName string) []string {
          var ids []string
          for _, repo := range b.cfg.Repos {
              if repo.Provider == providerName {
                  ids = append(ids, repo.ChannelID)
              }
          }
          return ids
      }

      func (b *Bridge) handleMessages(ctx context.Context, prov provider.Provider) {
          for {
              select {
              case <-ctx.Done():
                  return
              case msg, ok := <-prov.Messages():
                  if !ok {
                      return
                  }
                  b.processMessage(ctx, prov, msg)
              }
          }
      }

      func (b *Bridge) processMessage(ctx context.Context, prov provider.Provider, msg provider.Message) {
          route := router.Parse(msg.Content)

          switch route.Type {
          case router.RouteToBridge:
              b.handleBridgeCommand(prov, msg.ChannelID, route)
          case router.RouteToLLM:
              b.handleLLMMessage(ctx, prov, msg, route)
          }
      }

      func (b *Bridge) handleBridgeCommand(prov provider.Provider, channelID string, route router.Route) {
          var response string

          switch route.Command {
          case "status":
              response = b.getStatus(channelID)
          case "cancel":
              response = b.cancelLLM(channelID)
          case "restart":
              response = b.restartLLM(channelID)
          case "help":
              response = "Commands: /status, /cancel, /restart, /help\nSkills: !commit, !review-pr, etc."
          default:
              response = fmt.Sprintf("Unknown command: %s", route.Command)
          }

          prov.Send(channelID, response)
      }

      func (b *Bridge) handleLLMMessage(ctx context.Context, prov provider.Provider, msg provider.Message, route router.Route) {
          repoName := b.repoForChannel(msg.ChannelID)
          if repoName == "" {
              prov.Send(msg.ChannelID, "No repo configured for this channel")
              return
          }

          repo := b.cfg.Repos[repoName]
          session, err := b.getOrCreateSession(ctx, repoName, repo, prov)
          if err != nil {
              slog.Error("failed to create session", "error", err, "repo", repoName)
              prov.Send(msg.ChannelID, fmt.Sprintf("Error starting LLM: %v", err))
              return
          }

          // Format with conflict prefix if needed
          formatted := b.merger.FormatMessage(prov.Name(), route.Raw)

          llmMsg := llm.Message{
              Source:  prov.Name(),
              Content: formatted,
          }

          if err := session.llm.Send(llmMsg); err != nil {
              slog.Error("send to llm failed", "error", err, "repo", repoName)
              prov.Send(msg.ChannelID, fmt.Sprintf("Error: %v", err))
          }
      }

      func (b *Bridge) getOrCreateSession(ctx context.Context, repoName string, repo config.RepoConfig, prov provider.Provider) (*repoSession, error) {
          b.mu.Lock()
          defer b.mu.Unlock()

          if session, ok := b.repos[repoName]; ok && session.llm.Running() {
              // Add this provider/channel if not already tracked
              b.addChannelToSession(session, prov, repo.ChannelID)
              return session, nil
          }

          // Determine LLM backend
          llmBackend := repo.LLM
          if llmBackend == "" {
              llmBackend = b.cfg.Defaults.LLM
          }

          // Create LLM instance
          llmInstance, err := llm.New(llmBackend, repo.WorkingDir, b.cfg.Defaults.GetClaudePath(), b.cfg.Defaults.GetResumeSession())
          if err != nil {
              return nil, fmt.Errorf("create llm: %w", err)
          }

          // Create cancellable context for this session
          sessionCtx, cancel := context.WithCancel(ctx)

          if err := llmInstance.Start(sessionCtx); err != nil {
              cancel()
              return nil, fmt.Errorf("start llm: %w", err)
          }

          session := &repoSession{
              name:      repoName,
              llm:       llmInstance,
              channels:  []channelRef{{provider: prov, channelID: repo.ChannelID}},
              cancelCtx: cancel,
          }
          b.repos[repoName] = session

          go b.readOutput(session, repoName)

          slog.Info("started llm session", "repo", repoName, "llm", llmBackend, "dir", repo.WorkingDir)
          return session, nil
      }

      func (b *Bridge) addChannelToSession(session *repoSession, prov provider.Provider, channelID string) {
          for _, ch := range session.channels {
              if ch.provider.Name() == prov.Name() && ch.channelID == channelID {
                  return
              }
          }
          session.channels = append(session.channels, channelRef{provider: prov, channelID: channelID})
      }

      func (b *Bridge) readOutput(session *repoSession, repoName string) {
          output := session.llm.Output()
          if output == nil {
              slog.Warn("llm output is nil", "repo", repoName)
              return
          }

          reader := bufio.NewReader(output)
          var buffer string
          ticker := time.NewTicker(500 * time.Millisecond)
          defer ticker.Stop()

          for {
              select {
              case <-ticker.C:
                  if buffer != "" {
                      b.broadcastOutput(session, buffer)
                      buffer = ""
                  }
              default:
                  // Non-blocking read with timeout would be better, but ReadString blocks
                  // For now, read line by line
                  line, err := reader.ReadString('\n')
                  if err != nil {
                      if buffer != "" {
                          b.broadcastOutput(session, buffer)
                      }
                      slog.Info("llm output ended", "repo", repoName)
                      return
                  }
                  buffer += line

                  // Update activity on output received
                  session.llm.UpdateActivity()

                  // Flush if buffer exceeds threshold
                  if len(buffer) > b.cfg.Defaults.OutputThreshold {
                      b.broadcastOutput(session, buffer)
                      buffer = ""
                  }
              }
          }
      }

      // handleTerminalMessages handles terminal input and routes to repos
      func (b *Bridge) handleTerminalMessages(ctx context.Context, term *provider.Terminal) {
          for {
              select {
              case <-ctx.Done():
                  return
              case msg, ok := <-term.Messages():
                  if !ok {
                      return
                  }
                  // Terminal needs special handling - determine target repo
                  // For now, use first configured repo or allow /select command
                  b.processTerminalMessage(ctx, term, msg)
              }
          }
      }

      func (b *Bridge) processTerminalMessage(ctx context.Context, term *provider.Terminal, msg provider.Message) {
          route := router.Parse(msg.Content)

          // Special terminal command: /select <repo>
          if route.Type == router.RouteToBridge && route.Command == "select" {
              if route.Args == "" {
                  // List available repos
                  var repos []string
                  for name := range b.cfg.Repos {
                      repos = append(repos, name)
                  }
                  term.Send("", fmt.Sprintf("Usage: /select <repo-name>\nAvailable repos: %v\nCurrently selected: %s", repos, b.getTerminalRepo()))
                  return
              }
              if _, ok := b.cfg.Repos[route.Args]; !ok {
                  term.Send("", fmt.Sprintf("Unknown repo: %s", route.Args))
                  return
              }
              b.mu.Lock()
              b.terminalRepoName = route.Args
              b.mu.Unlock()
              term.Send("", fmt.Sprintf("Selected repo: %s", route.Args))
              return
          }

          // Get terminal's selected repo (or default to first)
          repoName := b.getTerminalRepo()
          if repoName == "" {
              term.Send("", "No repos configured. Add repos to llm-bridge.yaml")
              return
          }

          repo := b.cfg.Repos[repoName]

          switch route.Type {
          case router.RouteToBridge:
              b.handleBridgeCommand(term, term.ChannelID(), route)
          case router.RouteToLLM:
              session, err := b.getOrCreateSession(ctx, repoName, repo, term)
              if err != nil {
                  slog.Error("failed to create session", "error", err, "repo", repoName)
                  term.Send("", fmt.Sprintf("Error starting LLM: %v", err))
                  return
              }

              formatted := b.merger.FormatMessage(term.Name(), route.Raw)
              llmMsg := llm.Message{
                  Source:  term.Name(),
                  Content: formatted,
              }

              if err := session.llm.Send(llmMsg); err != nil {
                  slog.Error("send to llm failed", "error", err, "repo", repoName)
                  term.Send("", fmt.Sprintf("Error: %v", err))
              }
          }
      }

      func (b *Bridge) getTerminalRepo() string {
          b.mu.Lock()
          defer b.mu.Unlock()

          if b.terminalRepoName != "" {
              return b.terminalRepoName
          }

          // Default to first configured repo
          for name := range b.cfg.Repos {
              b.terminalRepoName = name
              return name
          }
          return ""
      }

      // broadcastOutput sends output to ALL connected channels
      func (b *Bridge) broadcastOutput(session *repoSession, content string) {
          if content == "" {
              return
          }

          b.mu.Lock()
          channels := make([]channelRef, len(session.channels))
          copy(channels, session.channels)
          b.mu.Unlock()

          for _, ch := range channels {
              if b.output.ShouldAttach(content) {
                  filename, data := b.output.FormatFile(content)
                  if err := ch.provider.SendFile(ch.channelID, filename, data); err != nil {
                      slog.Error("send file failed", "error", err, "provider", ch.provider.Name())
                  }
              } else {
                  if err := ch.provider.Send(ch.channelID, content); err != nil {
                      slog.Error("send failed", "error", err, "provider", ch.provider.Name())
                  }
              }
          }
      }

      func (b *Bridge) idleTimeoutLoop(ctx context.Context) {
          ticker := time.NewTicker(1 * time.Minute)
          defer ticker.Stop()

          timeout := b.cfg.Defaults.GetIdleTimeoutDuration()

          for {
              select {
              case <-ctx.Done():
                  return
              case <-ticker.C:
                  b.checkIdleTimeouts(timeout)
              }
          }
      }

      func (b *Bridge) checkIdleTimeouts(timeout time.Duration) {
          b.mu.Lock()
          defer b.mu.Unlock()

          for name, session := range b.repos {
              if session.llm == nil || !session.llm.Running() {
                  continue
              }

              if time.Since(session.llm.LastActivity()) > timeout {
                  slog.Info("stopping idle llm", "repo", name, "idle", time.Since(session.llm.LastActivity()))
                  session.llm.Stop()
                  if session.cancelCtx != nil {
                      session.cancelCtx()
                  }
                  delete(b.repos, name)

                  // Notify all channels
                  for _, ch := range session.channels {
                      ch.provider.Send(ch.channelID, fmt.Sprintf("LLM stopped due to idle timeout (%v)", timeout))
                  }
              }
          }
      }

      func (b *Bridge) repoForChannel(channelID string) string {
          for name, repo := range b.cfg.Repos {
              if repo.ChannelID == channelID {
                  return name
              }
          }
          return ""
      }

      func (b *Bridge) getStatus(channelID string) string {
          repoName := b.repoForChannel(channelID)
          if repoName == "" {
              return "No repo configured for this channel"
          }

          b.mu.Lock()
          session, ok := b.repos[repoName]
          b.mu.Unlock()

          if !ok || session.llm == nil || !session.llm.Running() {
              return fmt.Sprintf("LLM: not running (repo: %s)", repoName)
          }

          idle := time.Since(session.llm.LastActivity())
          return fmt.Sprintf("LLM: %s running (repo: %s, idle: %v)", session.llm.Name(), repoName, idle.Round(time.Second))
      }

      func (b *Bridge) cancelLLM(channelID string) string {
          repoName := b.repoForChannel(channelID)
          if repoName == "" {
              return "No repo configured"
          }

          b.mu.Lock()
          session, ok := b.repos[repoName]
          b.mu.Unlock()

          if !ok || session.llm == nil || !session.llm.Running() {
              return "LLM not running"
          }

          if err := session.llm.Cancel(); err != nil {
              return fmt.Sprintf("Cancel failed: %v", err)
          }
          return "Sent interrupt signal"
      }

      func (b *Bridge) restartLLM(channelID string) string {
          repoName := b.repoForChannel(channelID)
          if repoName == "" {
              return "No repo configured"
          }

          b.mu.Lock()
          session, ok := b.repos[repoName]
          if ok && session.llm != nil {
              session.llm.Stop()
              if session.cancelCtx != nil {
                  session.cancelCtx()
              }
          }
          delete(b.repos, repoName)
          b.mu.Unlock()

          return "LLM stopped. Will restart on next message."
      }
      ```
    dependencies: [2, 6, 8, 9, 10, 11, 12, 13]

  - id: 15
    description: "Create CLI main"
    subagent_type: "general-purpose"
    prompt: |
      Create cmd/llm-bridge/main.go with CLI commands.

      File: cmd/llm-bridge/main.go

      ```go
      package main

      import (
          "context"
          "fmt"
          "log/slog"
          "os"
          "os/signal"
          "syscall"

          "github.com/spf13/cobra"
          "gopkg.in/yaml.v3"

          "github.com/anthropics/llm-bridge/internal/bridge"
          "github.com/anthropics/llm-bridge/internal/config"
      )

      var cfgFile string

      var rootCmd = &cobra.Command{
          Use:   "llm-bridge",
          Short: "Bridge Discord/Telegram to Claude/Codex",
          Long:  "A bidirectional bridge connecting chat platforms to LLM CLIs.",
      }

      var serveCmd = &cobra.Command{
          Use:   "serve",
          Short: "Start the bridge server",
          RunE: func(cmd *cobra.Command, args []string) error {
              cfg, err := config.Load(cfgFile)
              if err != nil {
                  return fmt.Errorf("load config: %w", err)
              }

              ctx, cancel := context.WithCancel(context.Background())
              defer cancel()

              sigCh := make(chan os.Signal, 1)
              signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
              go func() {
                  <-sigCh
                  slog.Info("shutting down")
                  cancel()
              }()

              b := bridge.New(cfg)
              slog.Info("starting bridge", "config", cfgFile)
              return b.Start(ctx)
          },
      }

      var addRepoCmd = &cobra.Command{
          Use:   "add-repo <name>",
          Short: "Add a repository to the configuration",
          Args:  cobra.ExactArgs(1),
          RunE: func(cmd *cobra.Command, args []string) error {
              name := args[0]

              providerFlag, _ := cmd.Flags().GetString("provider")
              channelFlag, _ := cmd.Flags().GetString("channel")
              llmFlag, _ := cmd.Flags().GetString("llm")
              dirFlag, _ := cmd.Flags().GetString("dir")

              cfg, err := config.Load(cfgFile)
              if err != nil {
                  cfg = &config.Config{
                      Repos: make(map[string]config.RepoConfig),
                  }
              }

              cfg.Repos[name] = config.RepoConfig{
                  Provider:   providerFlag,
                  ChannelID:  channelFlag,
                  LLM:        llmFlag,
                  WorkingDir: dirFlag,
              }

              data, err := yaml.Marshal(cfg)
              if err != nil {
                  return fmt.Errorf("marshal config: %w", err)
              }

              if err := os.WriteFile(cfgFile, data, 0644); err != nil {
                  return fmt.Errorf("write config: %w", err)
              }

              fmt.Printf("Added repo %q to %s\n", name, cfgFile)
              return nil
          },
      }

      func init() {
          rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "llm-bridge.yaml", "config file path")

          rootCmd.AddCommand(serveCmd)
          rootCmd.AddCommand(addRepoCmd)

          addRepoCmd.Flags().String("provider", "discord", "Chat provider (discord, telegram)")
          addRepoCmd.Flags().String("channel", "", "Channel ID")
          addRepoCmd.Flags().String("llm", "claude", "LLM backend (claude, codex)")
          addRepoCmd.Flags().String("dir", ".", "Working directory")
          addRepoCmd.MarkFlagRequired("channel")
          addRepoCmd.MarkFlagRequired("dir")
      }

      func main() {
          slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
              Level: slog.LevelInfo,
          })))

          if err := rootCmd.Execute(); err != nil {
              os.Exit(1)
          }
      }
      ```
    dependencies: [14]

  - id: 16
    description: "Create example config"
    subagent_type: "general-purpose"
    prompt: |
      Create llm-bridge.yaml.example with sample configuration.

      File: llm-bridge.yaml.example

      ```yaml
      # llm-bridge configuration
      # Copy to llm-bridge.yaml and customize

      repos:
        notification-hooks:
          provider: discord
          channel_id: "123456789012345678"
          llm: claude
          working_dir: /home/user/projects/notification-hooks

        my-project:
          provider: telegram
          channel_id: "-1001234567890"
          llm: claude
          working_dir: /home/user/projects/my-project

      defaults:
        llm: claude
        claude_path: claude  # or /usr/local/bin/claude for specific version
        output_threshold: 1500
        idle_timeout: 10m
        resume_session: true

      providers:
        discord:
          bot_token: "${DISCORD_BOT_TOKEN}"
        telegram:
          bot_token: "${TELEGRAM_BOT_TOKEN}"
      ```
    dependencies: [1]

  - id: 17
    description: "Create CLAUDE.md"
    subagent_type: "general-purpose"
    prompt: |
      Create CLAUDE.md with project context.

      File: CLAUDE.md

      ```markdown
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

      ## Commands

      | Input | Handler | Description |
      |-------|---------|-------------|
      | `/status` | Bridge | Show LLM status and idle time |
      | `/cancel` | Bridge | Send SIGINT to LLM |
      | `/restart` | Bridge | Restart LLM process |
      | `!commit` | LLM | Translates to `/commit` |
      | text | LLM | Raw message to LLM |
      ```
    dependencies: [1]

  - id: 18
    description: "Create Makefile"
    subagent_type: "general-purpose"
    prompt: |
      Create Makefile.

      File: Makefile

      ```makefile
      .PHONY: build run clean test lint

      BINARY=llm-bridge
      VERSION?=dev

      build:
      	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) ./cmd/llm-bridge

      run: build
      	./$(BINARY) serve

      clean:
      	rm -f $(BINARY)

      test:
      	go test -v ./...

      lint:
      	golangci-lint run

      deps:
      	go mod download
      	go mod tidy
      ```
    dependencies: [1]

  - id: 19
    description: "Add go dependencies"
    subagent_type: "Bash"
    prompt: |
      Add all required Go dependencies:

      cd /root/llm-bridge && go get github.com/bwmarrin/discordgo && go get gopkg.in/telebot.v3 && go get github.com/creack/pty && go get gopkg.in/yaml.v3 && go get github.com/spf13/cobra && go mod tidy
    dependencies: [15]

  - id: 20
    description: "Build and verify"
    subagent_type: "Bash"
    prompt: |
      Build the project and verify it compiles:

      cd /root/llm-bridge && go build -o llm-bridge ./cmd/llm-bridge && ./llm-bridge --help
    dependencies: [19]

  - id: 21
    description: "Commit implementation"
    subagent_type: "Bash"
    prompt: |
      Stage and commit all implementation files:

      cd /root/llm-bridge
      git add -A
      git status

      Commit with:
      git commit -m "feat: implement llm-bridge with full feature set

      - Discord and Telegram provider support
      - Terminal provider for local stdin/stdout
      - Claude CLI wrapper with PTY (Codex stub)
      - LLM factory for backend selection
      - Input merging with conflict prefixing
      - Output broadcasting to all channels
      - Idle timeout for process lifecycle
      - Command routing (/, ! prefixes)
      - YAML configuration with env var expansion
      - CLI with serve and add-repo commands

      Co-Authored-By: Claude <noreply@anthropic.com>"
    dependencies: [20]

  - id: 22
    description: "Create Dockerfile"
    subagent_type: "general-purpose"
    prompt: |
      Create Dockerfile for containerized deployment.

      File: Dockerfile

      ```dockerfile
      # Build stage
      FROM golang:1.21-alpine AS builder

      WORKDIR /app

      # Install git for go mod download
      RUN apk add --no-cache git

      # Copy go mod files first for caching
      COPY go.mod go.sum ./
      RUN go mod download

      # Copy source and build
      COPY . .
      RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o llm-bridge ./cmd/llm-bridge

      # Runtime stage
      FROM alpine:latest

      # Install Claude CLI dependencies
      RUN apk add --no-cache \
          ca-certificates \
          nodejs \
          npm \
          bash \
          git

      # Install Claude CLI (adjust version as needed)
      # Note: Replace with actual Claude CLI installation method
      RUN npm install -g @anthropic-ai/claude-cli || true

      WORKDIR /app

      # Copy binary from builder
      COPY --from=builder /app/llm-bridge /usr/local/bin/llm-bridge

      # Create config directory
      RUN mkdir -p /etc/llm-bridge

      # Default config path
      ENV LLM_BRIDGE_CONFIG=/etc/llm-bridge/llm-bridge.yaml

      # Expose no ports by default (Discord/Telegram use outbound connections)

      ENTRYPOINT ["llm-bridge"]
      CMD ["serve", "--config", "/etc/llm-bridge/llm-bridge.yaml"]
      ```
    dependencies: [1]

  - id: 23
    description: "Create docker-compose.yml"
    subagent_type: "general-purpose"
    prompt: |
      Create docker-compose.yml for easy deployment.

      File: docker-compose.yml

      ```yaml
      version: '3.8'

      services:
        llm-bridge:
          build: .
          container_name: llm-bridge
          restart: unless-stopped
          environment:
            - DISCORD_BOT_TOKEN=${DISCORD_BOT_TOKEN}
            - TELEGRAM_BOT_TOKEN=${TELEGRAM_BOT_TOKEN}
            - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}
          volumes:
            # Config file
            - ./llm-bridge.yaml:/etc/llm-bridge/llm-bridge.yaml:ro
            # Mount repo directories for Claude to access
            - /root/projects:/root/projects
            # Persist Claude session data
            - claude-data:/root/.claude
          # For terminal access (interactive mode)
          stdin_open: true
          tty: true

      volumes:
        claude-data:
      ```
    dependencies: [22]

  - id: 24
    description: "Create .dockerignore"
    subagent_type: "general-purpose"
    prompt: |
      Create .dockerignore file.

      File: .dockerignore

      ```
      # Build artifacts
      llm-bridge
      *.exe

      # Git
      .git
      .gitignore

      # IDE
      .vscode
      .idea
      *.swp

      # Config (mounted at runtime)
      llm-bridge.yaml
      *.env

      # Docs
      docs/
      *.md
      !CLAUDE.md

      # Tests
      *_test.go

      # Local development
      .agents/
      ```
    dependencies: [1]
```

---

## Task Dependency Graph

```
1 (init module)
├── 2 (config) ──────────────────────────────────────┐
├── 3 (llm interface) → 4 (claude) → 6 (factory)    │
│                     → 5 (codex)  ↗                 │
├── 7 (provider interface) → 8 (discord) ───────────┤
│                         → 9 (telegram) ───────────┤
│                         → 10 (terminal) ──────────┤
├── 11 (router) ────────────────────────────────────┤
├── 12 (output) ────────────────────────────────────┤
├── 13 (merger) ────────────────────────────────────┤
│                                                    ↓
│                                    14 (bridge) → 15 (CLI) → 19 (deps) → 20 (build) → 21 (commit)
├── 16 (example config)
├── 17 (CLAUDE.md)
└── 18 (Makefile)
```

Parallel execution groups:
- After 1: Tasks 2, 3, 7, 11, 12, 13, 16, 17, 18
- After 3: Tasks 4, 5
- After 4, 5: Task 6
- After 7: Tasks 8, 9, 10
- After 2, 6, 8, 9, 10, 11, 12, 13: Task 14
- Sequential: 14 → 15 → 19 → 20 → 21
