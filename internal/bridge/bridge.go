package bridge

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/llm-bridge/internal/config"
	"github.com/anthropics/llm-bridge/internal/git"
	"github.com/anthropics/llm-bridge/internal/llm"
	"github.com/anthropics/llm-bridge/internal/output"
	"github.com/anthropics/llm-bridge/internal/provider"
	"github.com/anthropics/llm-bridge/internal/ratelimit"
	"github.com/anthropics/llm-bridge/internal/router"
)

// LLMFactory creates LLM instances. Defaults to llm.New.
type LLMFactory func(backend, workingDir, claudePath string, resume bool) (llm.LLM, error)

// DiscordFactory creates Discord provider instances. Defaults to provider.NewDiscord.
type DiscordFactory func(token string, channelIDs []string) provider.Provider

// TerminalFactory creates Terminal provider instances. Defaults to provider.NewTerminal.
type TerminalFactory func(channelID string) *provider.Terminal

// GitDetector detects git repository info for a directory.
type GitDetector func(dir string) (*git.RepoInfo, error)

// WorktreeLister lists git worktrees for a directory.
type WorktreeLister func(dir string) ([]git.WorktreeInfo, error)

// CloneRepoFunc clones a git repository. Defaults to git.CloneRepo.
type CloneRepoFunc func(url, destDir string) error

// AddWorktreeFunc creates a new git worktree. Defaults to git.AddWorktree.
type AddWorktreeFunc func(repoDir, wtDir, branch string) error

type Bridge struct {
	cfg             *config.Config
	cfgPath         string
	providers       map[string]provider.Provider
	repos           map[string]*repoSession
	output          *output.Handler
	discordFactory  DiscordFactory
	terminalFactory TerminalFactory
	llmFactory      LLMFactory
	gitDetector     GitDetector
	worktreeLister  WorktreeLister
	cloneRepo       CloneRepoFunc
	addWorktree     AddWorktreeFunc

	userLimiter    *ratelimit.Limiter
	channelLimiter *ratelimit.Limiter

	mu               sync.Mutex
	terminalRepoName string
}

type repoSession struct {
	name      string
	llm       llm.LLM
	channels  []channelRef
	cancelCtx context.CancelFunc
	merger    *Merger        // per-repo conflict detection
	gitInfo   *git.RepoInfo  // nil if not a git repo
}

type channelRef struct {
	provider  provider.Provider
	channelID string
}

func New(cfg *config.Config, cfgPath string) *Bridge {
	b := &Bridge{
		cfg:       cfg,
		cfgPath:   cfgPath,
		providers: make(map[string]provider.Provider),
		repos:     make(map[string]*repoSession),
		output:    output.NewHandler(cfg.Defaults.OutputThreshold),
		llmFactory: llm.New,
		discordFactory: func(token string, channelIDs []string) provider.Provider {
			return provider.NewDiscord(token, channelIDs)
		},
		terminalFactory: provider.NewTerminal,
		gitDetector:     git.DetectRepo,
		worktreeLister:  git.ListWorktrees,
		cloneRepo:       git.CloneRepo,
		addWorktree:     git.AddWorktree,
	}

	if cfg.Defaults.RateLimit.GetRateLimitEnabled() {
		b.userLimiter = ratelimit.NewLimiter(ratelimit.Config{
			Rate:  cfg.Defaults.RateLimit.GetUserRate(),
			Burst: cfg.Defaults.RateLimit.GetUserBurst(),
		})
		b.channelLimiter = ratelimit.NewLimiter(ratelimit.Config{
			Rate:  cfg.Defaults.RateLimit.GetChannelRate(),
			Burst: cfg.Defaults.RateLimit.GetChannelBurst(),
		})
	}

	return b
}

func (b *Bridge) Start(ctx context.Context) error {
	// Initialize Discord if configured
	token := b.cfg.Providers.Discord.GetBotToken()
	if token != "" {
		channelIDs := b.channelIDsForProvider("discord")
		if len(channelIDs) > 0 {
			discord := b.discordFactory(token, channelIDs)
			if err := discord.Start(ctx); err != nil {
				return fmt.Errorf("start discord: %w", err)
			}
			b.providers["discord"] = discord
			go b.handleMessages(ctx, discord)
			slog.Info("discord provider started", "channels", len(channelIDs))
		}
	}

	// Initialize Terminal (always enabled for local interaction)
	terminal := b.terminalFactory("terminal")
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
			_ = repo.llm.Stop()
		}
		if repo.cancelCtx != nil {
			repo.cancelCtx()
		}
	}

	for _, prov := range b.providers {
		_ = prov.Stop()
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
		if b.isRateLimited(prov, msg) {
			return
		}
		b.handleLLMMessage(ctx, prov, msg, route)
	}
}

// isRateLimited checks per-user and per-channel rate limits.
// Returns true if the message should be rejected.
// User rate limiting uses AuthorID (stable unique ID), so terminal messages
// (which have empty AuthorID) are implicitly not user-rate-limited.
func (b *Bridge) isRateLimited(prov provider.Provider, msg provider.Message) bool {
	if b.userLimiter == nil || b.channelLimiter == nil {
		return false
	}

	if msg.AuthorID != "" && !b.userLimiter.Allow(msg.AuthorID) {
		_ = prov.Send(msg.ChannelID, fmt.Sprintf("Rate limited: too many messages from user %s. Please wait.", msg.Author))
		slog.Warn("rate limited user", "user", msg.Author, "author_id", msg.AuthorID, "channel", msg.ChannelID)
		return true
	}

	if !b.channelLimiter.Allow(msg.ChannelID) {
		_ = prov.Send(msg.ChannelID, "Rate limited: too many messages in this channel. Please wait.")
		slog.Warn("rate limited channel", "channel", msg.ChannelID)
		return true
	}

	return false
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
	case "worktrees":
		response = b.listWorktrees(channelID)
	case "list-repos":
		response = b.handleListRepos()
	case "remove-repo":
		response = b.handleRemoveRepo(route.Args)
	case "clone":
		response = b.handleClone(prov.Name(), route.Args)
	case "add-worktree":
		response = b.handleAddWorktree(prov.Name(), channelID, route.Args)
	case "help":
		response = `Commands:
  /help                                  - Show this help
  /status                                - Show LLM status and idle time
  /cancel                                - Send SIGINT to LLM
  /restart                               - Restart LLM process
  /select <repo>                         - Select repo for terminal

Repo Management:
  /list-repos                            - List all configured repos
  /clone <url> <name> [channel-id]       - Clone a git repo and register it
  /remove-repo <name>                    - Remove a repo from config

Worktrees:
  /worktrees                             - List git worktrees for current repo
  /add-worktree <name> <branch> [channel-id] - Create worktree from current repo

Skills: ::commit, ::review-pr, etc.`
	default:
		response = fmt.Sprintf("Unknown command: %s", route.Command)
	}

	_ = prov.Send(channelID, response)
}

func (b *Bridge) handleLLMMessage(ctx context.Context, prov provider.Provider, msg provider.Message, route router.Route) {
	repoName := b.repoForChannel(msg.ChannelID)
	if repoName == "" {
		_ = prov.Send(msg.ChannelID, "No repo configured for this channel")
		return
	}

	repo := b.cfg.Repos[repoName]
	session, err := b.getOrCreateSession(ctx, repoName, repo, prov)
	if err != nil {
		slog.Error("failed to create session", "error", err, "repo", repoName)
		_ = prov.Send(msg.ChannelID, fmt.Sprintf("Error starting LLM: %v", err))
		return
	}

	formatted := session.merger.FormatMessage(prov.Name(), route.Raw)

	llmMsg := llm.Message{
		Source:  prov.Name(),
		Content: formatted,
	}

	if err := session.llm.Send(llmMsg); err != nil {
		slog.Error("send to llm failed", "error", err, "repo", repoName)
		_ = prov.Send(msg.ChannelID, fmt.Sprintf("Error: %v", err))
	}
}

func (b *Bridge) getOrCreateSession(ctx context.Context, repoName string, repo config.RepoConfig, prov provider.Provider) (*repoSession, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if session, ok := b.repos[repoName]; ok && session.llm.Running() {
		b.addChannelToSession(session, prov, repo.ChannelID)
		return session, nil
	}

	llmBackend := repo.LLM
	if llmBackend == "" {
		llmBackend = b.cfg.Defaults.LLM
	}

	llmInstance, err := b.llmFactory(llmBackend, repo.WorkingDir, b.cfg.Defaults.GetClaudePath(), b.cfg.Defaults.GetResumeSession())
	if err != nil {
		return nil, fmt.Errorf("create llm: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(ctx)

	if err := llmInstance.Start(sessionCtx); err != nil {
		cancel()
		return nil, fmt.Errorf("start llm: %w", err)
	}

	var gitInfo *git.RepoInfo
	if b.gitDetector != nil {
		info, err := b.gitDetector(repo.WorkingDir)
		if err != nil {
			slog.Warn("git detection failed", "repo", repoName, "dir", repo.WorkingDir, "error", err)
		} else {
			gitInfo = info
		}
	}

	session := &repoSession{
		name:      repoName,
		llm:       llmInstance,
		channels:  []channelRef{{provider: prov, channelID: repo.ChannelID}},
		cancelCtx: cancel,
		merger:    NewMerger(2 * time.Second),
		gitInfo:   gitInfo,
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
	out := session.llm.Output()
	if out == nil {
		slog.Warn("llm output is nil", "repo", repoName)
		return
	}

	reader := bufio.NewReader(out)
	var buffer string
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Use goroutine for non-blocking reads with buffered channel
	// to prevent PTY backpressure when broadcast is slow
	type readResult struct {
		line string
		err  error
	}
	lines := make(chan readResult, 100) // Buffer to prevent blocking on slow broadcasts
	go func() {
		defer close(lines)
		for {
			line, err := reader.ReadString('\n')
			lines <- readResult{line, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ticker.C:
			if buffer != "" {
				b.broadcastOutput(session, buffer)
				buffer = ""
			}
		case result, ok := <-lines:
			if !ok {
				// Channel closed
				if buffer != "" {
					b.broadcastOutput(session, buffer)
				}
				slog.Info("llm output ended", "repo", repoName)
				return
			}
			if result.err != nil {
				// Include any partial line before error
				buffer += result.line
				if buffer != "" {
					b.broadcastOutput(session, buffer)
				}
				slog.Info("llm output ended", "repo", repoName, "error", result.err)
				return
			}
			buffer += result.line
			session.llm.UpdateActivity()

			if len(buffer) > b.cfg.Defaults.OutputThreshold {
				b.broadcastOutput(session, buffer)
				buffer = ""
			}
		}
	}
}

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

func (b *Bridge) handleTerminalMessages(ctx context.Context, term *provider.Terminal) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-term.Messages():
			if !ok {
				return
			}
			b.processTerminalMessage(ctx, term, msg)
		}
	}
}

func (b *Bridge) processTerminalMessage(ctx context.Context, term *provider.Terminal, msg provider.Message) {
	route := router.Parse(msg.Content)

	if route.Type == router.RouteToBridge && route.Command == "select" {
		if route.Args == "" {
			var repos []string
			for name := range b.cfg.Repos {
				repos = append(repos, name)
			}
			_ = term.Send("", fmt.Sprintf("Usage: /select <repo-name>\nAvailable repos: %v\nCurrently selected: %s", repos, b.getTerminalRepo()))
			return
		}
		if _, ok := b.cfg.Repos[route.Args]; !ok {
			_ = term.Send("", fmt.Sprintf("Unknown repo: %s", route.Args))
			return
		}
		b.mu.Lock()
		b.terminalRepoName = route.Args
		b.mu.Unlock()
		_ = term.Send("", fmt.Sprintf("Selected repo: %s", route.Args))
		return
	}

	repoName := b.getTerminalRepo()
	if repoName == "" {
		_ = term.Send("", "No repos configured. Add repos to llm-bridge.yaml")
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
			_ = term.Send("", fmt.Sprintf("Error starting LLM: %v", err))
			return
		}

		formatted := session.merger.FormatMessage(term.Name(), route.Raw)
		llmMsg := llm.Message{
			Source:  term.Name(),
			Content: formatted,
		}

		if err := session.llm.Send(llmMsg); err != nil {
			slog.Error("send to llm failed", "error", err, "repo", repoName)
			_ = term.Send("", fmt.Sprintf("Error: %v", err))
		}
	}
}

func (b *Bridge) getTerminalRepo() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.terminalRepoName != "" {
		return b.terminalRepoName
	}

	for name := range b.cfg.Repos {
		b.terminalRepoName = name
		return name
	}
	return ""
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
	// Collect sessions to stop under lock, then release lock before stopping
	// to avoid blocking message handling and session creation
	type idleSession struct {
		name     string
		session  *repoSession
		channels []channelRef
	}
	var toStop []idleSession

	b.mu.Lock()
	for name, session := range b.repos {
		if session.llm == nil || !session.llm.Running() {
			continue
		}

		if time.Since(session.llm.LastActivity()) > timeout {
			// Copy channels slice while holding lock
			channels := make([]channelRef, len(session.channels))
			copy(channels, session.channels)
			toStop = append(toStop, idleSession{name, session, channels})
			delete(b.repos, name)
		}
	}
	b.mu.Unlock()

	// Stop sessions and notify channels outside the lock
	for _, idle := range toStop {
		slog.Info("stopping idle llm", "repo", idle.name, "idle", time.Since(idle.session.llm.LastActivity()))
		_ = idle.session.llm.Stop()
		if idle.session.cancelCtx != nil {
			idle.session.cancelCtx()
		}

		for _, ch := range idle.channels {
			_ = ch.provider.Send(ch.channelID, fmt.Sprintf("LLM stopped due to idle timeout (%v)", timeout))
		}
	}
}

func (b *Bridge) repoForChannel(channelID string) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, repo := range b.cfg.Repos {
		if repo.ChannelID == channelID {
			return name
		}
	}
	return ""
}

// repoConfigForChannel returns both the repo name and a copy of its config.
// This is a helper to atomically get both values under lock.
func (b *Bridge) repoConfigForChannel(channelID string) (string, config.RepoConfig, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for name, repo := range b.cfg.Repos {
		if repo.ChannelID == channelID {
			return name, repo, true
		}
	}
	return "", config.RepoConfig{}, false
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
	if session.gitInfo != nil && session.gitInfo.Branch != "" {
		if session.gitInfo.IsWorktree {
			return fmt.Sprintf("LLM: %s running (repo: %s, branch: %s, worktree, idle: %v)", session.llm.Name(), repoName, session.gitInfo.Branch, idle.Round(time.Second))
		}
		return fmt.Sprintf("LLM: %s running (repo: %s, branch: %s, idle: %v)", session.llm.Name(), repoName, session.gitInfo.Branch, idle.Round(time.Second))
	}
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
		_ = session.llm.Stop()
		if session.cancelCtx != nil {
			session.cancelCtx()
		}
	}
	delete(b.repos, repoName)
	b.mu.Unlock()

	return "LLM stopped. Will restart on next message."
}

func (b *Bridge) listWorktrees(channelID string) string {
	repoName := b.repoForChannel(channelID)
	if repoName == "" {
		return "No repo configured for this channel"
	}

	repo := b.cfg.Repos[repoName]

	worktrees, err := b.worktreeLister(repo.WorkingDir)
	if err != nil {
		return fmt.Sprintf("Not a git repository or git error: %v", err)
	}

	if len(worktrees) <= 1 {
		return fmt.Sprintf("No linked worktrees for %s", repoName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Worktrees for %s:\n", repoName))
	for _, wt := range worktrees {
		marker := "  "
		if wt.Path == repo.WorkingDir {
			marker = "* "
		}

		// Check if this worktree has a configured repo
		configuredRepo := ""
		for name, r := range b.cfg.Repos {
			if r.WorkingDir == wt.Path {
				configuredRepo = name
				break
			}
		}

		line := fmt.Sprintf("  %s%s (%s)", marker, wt.Path, wt.Branch)
		if configuredRepo != "" {
			// Check if session exists
			b.mu.Lock()
			session, active := b.repos[configuredRepo]
			isActive := active && session.llm != nil && session.llm.Running()
			b.mu.Unlock()

			if isActive {
				line += fmt.Sprintf(" -> repo: %s [active]", configuredRepo)
			} else {
				line += fmt.Sprintf(" -> repo: %s", configuredRepo)
			}
		} else {
			line += " -> not configured"
		}
		sb.WriteString(line + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// RuntimeAddRepo adds a repo to memory and persists it to the config file.
// The operation is atomic: if persistence fails, memory is not updated.
func (b *Bridge) RuntimeAddRepo(name string, repo config.RepoConfig) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Persist first - if this fails, memory stays consistent
	if err := config.AddRepo(b.cfgPath, name, repo); err != nil {
		return err
	}

	// Update memory only after successful persistence
	if b.cfg.Repos == nil {
		b.cfg.Repos = make(map[string]config.RepoConfig)
	}
	b.cfg.Repos[name] = repo

	return nil
}

func (b *Bridge) handleListRepos() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.cfg.Repos) == 0 {
		return "No repos configured"
	}

	// Collect and sort repo names alphabetically
	var repoNames []string
	for name := range b.cfg.Repos {
		repoNames = append(repoNames, name)
	}
	sort.Strings(repoNames)

	var sb strings.Builder
	sb.WriteString("Configured repos:\n")

	for _, name := range repoNames {
		repo := b.cfg.Repos[name]

		// Truncate working dir to last 30 chars if longer
		workingDir := repo.WorkingDir
		if len(workingDir) > 30 {
			workingDir = "..." + workingDir[len(workingDir)-27:]
		}

		// Check if session is active
		status := "inactive"
		if session, ok := b.repos[name]; ok && session.llm != nil && session.llm.Running() {
			status = "active"
		}

		// Format: - repo_name (channel: X, dir: Y, status, [branch: Z])
		line := fmt.Sprintf("- %s (channel: %s, dir: %s, %s", name, repo.ChannelID, workingDir, status)
		if repo.Branch != "" {
			line += fmt.Sprintf(", branch: %s", repo.Branch)
		}
		line += ")"
		sb.WriteString(line + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// handleRemoveRepo removes a repo from config and stops its active session.
// This does NOT delete any files on disk - only the config entry.
func (b *Bridge) handleRemoveRepo(args string) string {
	name := strings.TrimSpace(args)
	if name == "" {
		return "Usage: /remove-repo <repo-name>"
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if repo exists in config
	if _, ok := b.cfg.Repos[name]; !ok {
		return fmt.Sprintf("Repo %q not found", name)
	}

	// Stop active session if running
	if session, ok := b.repos[name]; ok {
		if session.llm != nil {
			_ = session.llm.Stop()
		}
		if session.cancelCtx != nil {
			session.cancelCtx()
		}
		delete(b.repos, name)
	}

	// Persist removal to config file
	if err := config.RemoveRepo(b.cfgPath, name); err != nil {
		return fmt.Sprintf("Failed to remove repo: %v", err)
	}

	// Remove from memory after successful persistence
	delete(b.cfg.Repos, name)

	return fmt.Sprintf("Removed repo %q (files on disk were not deleted)", name)
}

// handleClone clones an external repo and registers it as a new llm-bridge repo.
// Usage: /clone <url> <name> [channel-id]
// - url: Git repository URL (required, must use https/git/ssh scheme)
// - name: Repo name (required, alphanumeric with hyphens/underscores only)
// - channel-id: For Discord, required; for Terminal, defaults to "terminal-<name>"
func (b *Bridge) handleClone(providerName string, args string) string {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return "Usage: /clone <url> <name> [channel-id]"
	}

	repoURL := parts[0]
	name := parts[1]
	var channelID string
	if len(parts) >= 3 {
		channelID = parts[2]
	}

	// Security: validate URL scheme (prevents malicious URLs like ext::, file://)
	if !git.IsAllowedGitURL(repoURL) {
		return "Error: URL must use https, git, or ssh scheme"
	}

	// Security: validate name uses only safe characters (alphanumeric, hyphen, underscore)
	if !git.IsSafeRepoName(name) {
		return "Error: name must contain only letters, numbers, hyphens, and underscores"
	}

	// Check if repo name already exists
	b.mu.Lock()
	_, exists := b.cfg.Repos[name]
	b.mu.Unlock()
	if exists {
		return fmt.Sprintf("Error: repo %q already exists", name)
	}

	// Determine destination directory (always relative to BaseDir)
	baseDir := b.cfg.Defaults.GetBaseDir()
	destDir := filepath.Join(baseDir, name)

	// Determine channel ID based on provider
	if channelID == "" {
		if providerName == "discord" {
			return "Error: channel-id is required for Discord"
		}
		// Terminal: default to "terminal-<name>"
		channelID = "terminal-" + name
	}

	// Clone the repository
	if err := b.cloneRepo(repoURL, destDir); err != nil {
		return fmt.Sprintf("Clone failed: %v", err)
	}

	// Create RepoConfig
	repo := config.RepoConfig{
		Provider:   providerName,
		ChannelID:  channelID,
		LLM:        b.cfg.Defaults.LLM,
		WorkingDir: destDir,
	}

	// Persist to config
	if err := b.RuntimeAddRepo(name, repo); err != nil {
		return fmt.Sprintf("Cloned but failed to register: %v", err)
	}

	return fmt.Sprintf("Cloned and registered repo %q (channel: %s)", name, channelID)
}

// handleAddWorktree creates a new git worktree from the current repo and registers it.
// Usage: /add-worktree <name> <branch> [channel-id]
// - name: Worktree name (required, alphanumeric with hyphens/underscores only)
// - branch: Branch name to create (required)
// - channel-id: For Discord, required; for Terminal, defaults to "terminal-<parent>/<name>"
func (b *Bridge) handleAddWorktree(providerName string, parentChannelID string, args string) string {
	parts := strings.Fields(args)
	if len(parts) < 2 {
		return "Usage: /add-worktree <name> <branch> [channel-id]"
	}

	name := parts[0]
	branch := parts[1]
	var newChannelID string
	if len(parts) >= 3 {
		newChannelID = parts[2]
	}

	// Security: validate name uses only safe characters (alphanumeric, hyphen, underscore)
	if !git.IsSafeRepoName(name) {
		return "Error: name must contain only letters, numbers, hyphens, and underscores"
	}

	// Validate branch is also safe (alphanumeric, hyphen, underscore, slash for feature branches)
	if !git.IsSafeRepoName(strings.ReplaceAll(branch, "/", "-")) {
		return "Error: branch must contain only letters, numbers, hyphens, underscores, and slashes"
	}

	// Find current repo from channel (atomically get name and config copy)
	parentRepoName, parentRepo, found := b.repoConfigForChannel(parentChannelID)
	if !found {
		return "Error: no repo configured for this channel"
	}

	// Determine git root: if current repo has GitRoot set (is worktree), use GitRoot; otherwise use WorkingDir
	parentGitRoot := parentRepo.GitRoot
	if parentGitRoot == "" {
		parentGitRoot = parentRepo.WorkingDir
	}

	// Determine worktree destination: <git-root>-<name> (adjacent to main repo)
	wtDir := parentGitRoot + "-" + name

	// Create child repo name with parent/child naming
	childName := parentRepoName + "/" + name

	// Check if child name already exists
	b.mu.Lock()
	_, exists := b.cfg.Repos[childName]
	b.mu.Unlock()
	if exists {
		return fmt.Sprintf("Error: repo %q already exists", childName)
	}

	// Determine channel ID based on provider
	if newChannelID == "" {
		if providerName == "discord" {
			return "Error: channel-id is required for Discord"
		}
		// Terminal: default to "terminal-<childName>"
		newChannelID = "terminal-" + childName
	}

	// Create the worktree
	if err := b.addWorktree(parentGitRoot, wtDir, branch); err != nil {
		return fmt.Sprintf("Failed to create worktree: %v", err)
	}

	// Create RepoConfig
	repo := config.RepoConfig{
		Provider:   parentRepo.Provider,
		ChannelID:  newChannelID,
		LLM:        parentRepo.LLM,
		WorkingDir: wtDir,
		GitRoot:    parentGitRoot,
		Branch:     branch,
	}

	// Persist to config
	if err := b.RuntimeAddRepo(childName, repo); err != nil {
		return fmt.Sprintf("Worktree created but failed to register: %v", err)
	}

	return fmt.Sprintf("Created worktree %q (channel: %s, branch: %s)", childName, newChannelID, branch)
}
