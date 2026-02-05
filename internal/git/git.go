// Package git provides git repository and worktree detection using the git CLI.
package git

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// WorktreeInfo holds information about a git worktree.
type WorktreeInfo struct {
	Path   string // absolute path to the worktree directory
	Branch string // branch checked out (e.g., "main", "feature/foo")
	Head   string // short commit hash
	IsMain bool   // true if this is the main working tree
	IsBare bool   // true if this is a bare worktree entry
}

// RepoInfo holds information about a git repository.
type RepoInfo struct {
	RootDir    string         // path to the main working tree
	GitDir     string         // path to .git directory
	Branch     string         // current branch name
	IsWorktree bool           // true if the given path is a linked worktree
	Worktrees  []WorktreeInfo // all worktrees for this repo
}

// runGit executes a git command in the given directory and returns trimmed stdout.
// It returns an error if the command exits with a non-zero status.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// DetectRepo detects the git repository at the given directory and returns
// detailed information including worktree status. It returns an error if the
// directory is not inside a git repository.
func DetectRepo(dir string) (*RepoInfo, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	rootDir, err := runGit(absDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("detect repo: %w", err)
	}

	gitDir, err := runGit(absDir, "rev-parse", "--git-dir")
	if err != nil {
		return nil, fmt.Errorf("detect git dir: %w", err)
	}
	// Make gitDir absolute if it is relative.
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(absDir, gitDir)
	}
	gitDir = filepath.Clean(gitDir)

	branch, err := CurrentBranch(absDir)
	if err != nil {
		return nil, fmt.Errorf("detect branch: %w", err)
	}

	// Determine if this is a linked worktree by comparing --git-dir and --git-common-dir.
	gitCommonDir, err := runGit(absDir, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("detect git common dir: %w", err)
	}
	if !filepath.IsAbs(gitCommonDir) {
		gitCommonDir = filepath.Join(absDir, gitCommonDir)
	}
	gitCommonDir = filepath.Clean(gitCommonDir)

	isWorktree := gitDir != gitCommonDir

	worktrees, err := ListWorktrees(absDir)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	return &RepoInfo{
		RootDir:    rootDir,
		GitDir:     gitDir,
		Branch:     branch,
		IsWorktree: isWorktree,
		Worktrees:  worktrees,
	}, nil
}

// ListWorktrees lists all worktrees for the git repository at the given directory.
// It parses the porcelain output of "git worktree list --porcelain".
// The first entry in the returned slice is always the main worktree.
func ListWorktrees(dir string) ([]WorktreeInfo, error) {
	output, err := runGit(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}

	if output == "" {
		return nil, nil
	}

	var worktrees []WorktreeInfo
	blocks := splitWorktreeBlocks(output)

	for i, block := range blocks {
		wt := parseWorktreeBlock(block)
		if wt.Path == "" {
			continue
		}
		// The first entry from "git worktree list" is always the main worktree.
		if i == 0 {
			wt.IsMain = true
		}
		worktrees = append(worktrees, wt)
	}

	return worktrees, nil
}

// splitWorktreeBlocks splits the porcelain output into blocks separated by blank lines.
func splitWorktreeBlocks(output string) [][]string {
	lines := strings.Split(output, "\n")
	var blocks [][]string
	var current []string

	for _, line := range lines {
		if line == "" {
			if len(current) > 0 {
				blocks = append(blocks, current)
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, current)
	}
	return blocks
}

// parseWorktreeBlock parses a single worktree block from porcelain output.
func parseWorktreeBlock(lines []string) WorktreeInfo {
	var wt WorktreeInfo
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "worktree "):
			wt.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			hash := strings.TrimPrefix(line, "HEAD ")
			// Use only first 7 characters as short hash.
			if len(hash) > 7 {
				hash = hash[:7]
			}
			wt.Head = hash
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			// Strip refs/heads/ prefix to get the branch name.
			wt.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "bare":
			wt.IsBare = true
		case line == "detached":
			wt.Branch = "HEAD"
		}
	}
	return wt
}

// CurrentBranch returns the name of the currently checked-out branch in the given
// directory. It returns "HEAD" if the repository is in a detached HEAD state.
func CurrentBranch(dir string) (string, error) {
	branch, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("current branch: %w", err)
	}
	return branch, nil
}

// IsGitRepo returns true if the given directory is inside a git work tree.
func IsGitRepo(dir string) bool {
	result, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && result == "true"
}

// safeRepoNameRe validates repo/worktree names to prevent path traversal and shell issues.
// Only allows alphanumeric characters, hyphens, and underscores.
var safeRepoNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsSafeRepoName returns true if the name is safe to use as a repo or worktree name.
func IsSafeRepoName(name string) bool {
	return safeRepoNameRe.MatchString(name)
}

// allowedURLSchemes contains the URL schemes allowed for git clone.
var allowedURLSchemes = []string{"https://", "http://", "git://", "ssh://", "git@"}

// IsAllowedGitURL returns true if the URL uses an allowed scheme.
func IsAllowedGitURL(url string) bool {
	for _, scheme := range allowedURLSchemes {
		if strings.HasPrefix(url, scheme) {
			return true
		}
	}
	return false
}

// CloneRepo clones a git repository from url to destDir.
// It returns an error if destDir already exists or if the clone fails.
// Note: URL validation should be done by the caller (e.g., bridge) for security.
func CloneRepo(url, destDir string) error {
	// Check if destination already exists.
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("clone repo: destination %q already exists", destDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clone repo: check destination: %w", err)
	}

	// Get the parent directory for running the clone command.
	parentDir := filepath.Dir(destDir)
	if parentDir == "" {
		parentDir = "."
	}

	_, err := runGit(parentDir, "clone", url, destDir)
	if err != nil {
		return fmt.Errorf("clone repo: %w", err)
	}
	return nil
}

// AddWorktree creates a new git worktree at wtDir with a new branch.
// The -b flag creates a new branch; this will fail if the branch already exists.
// It returns an error if wtDir already exists or if the worktree creation fails.
func AddWorktree(repoDir, wtDir, branch string) error {
	// Check if worktree directory already exists.
	if _, err := os.Stat(wtDir); err == nil {
		return fmt.Errorf("add worktree: directory %q already exists", wtDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("add worktree: check directory: %w", err)
	}

	_, err := runGit(repoDir, "worktree", "add", wtDir, "-b", branch)
	if err != nil {
		return fmt.Errorf("add worktree: %w", err)
	}
	return nil
}
