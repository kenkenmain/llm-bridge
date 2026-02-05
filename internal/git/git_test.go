package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupTestRepo creates a temporary git repository with an initial commit.
// It returns the absolute path to the repository directory.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	dir := t.TempDir()

	commands := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}

	// Create a file and make an initial commit.
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# test repo\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	commitCmds := [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "initial commit"},
	}
	for _, args := range commitCmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", args, err, out)
		}
	}

	return dir
}

// getDefaultBranch returns the default branch name for the test repo.
// Modern git uses "main", older versions use "master".
func getDefaultBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("get default branch: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestIsGitRepo_True(t *testing.T) {
	dir := setupTestRepo(t)

	if !IsGitRepo(dir) {
		t.Error("IsGitRepo() = false for a git repository, want true")
	}
}

func TestIsGitRepo_False(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	dir := t.TempDir()

	if IsGitRepo(dir) {
		t.Error("IsGitRepo() = true for a non-git directory, want false")
	}
}

func TestIsGitRepo_Subdirectory(t *testing.T) {
	dir := setupTestRepo(t)

	subdir := filepath.Join(dir, "subdir")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	if !IsGitRepo(subdir) {
		t.Error("IsGitRepo() = false for a subdirectory of a git repo, want true")
	}
}

func TestCurrentBranch_Default(t *testing.T) {
	dir := setupTestRepo(t)
	expected := getDefaultBranch(t, dir)

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != expected {
		t.Errorf("CurrentBranch() = %q, want %q", branch, expected)
	}
}

func TestCurrentBranch_NewBranch(t *testing.T) {
	dir := setupTestRepo(t)

	// Create and checkout a new branch.
	cmd := exec.Command("git", "checkout", "-b", "feature/test-branch")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create branch: %v\n%s", err, out)
	}

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "feature/test-branch" {
		t.Errorf("CurrentBranch() = %q, want %q", branch, "feature/test-branch")
	}
}

func TestCurrentBranch_DetachedHead(t *testing.T) {
	dir := setupTestRepo(t)

	// Detach HEAD by checking out a specific commit.
	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("detach head: %v\n%s", err, out)
	}

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "HEAD" {
		t.Errorf("CurrentBranch() = %q, want %q for detached HEAD", branch, "HEAD")
	}
}

func TestCurrentBranch_NonGitDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	dir := t.TempDir()

	_, err := CurrentBranch(dir)
	if err == nil {
		t.Error("CurrentBranch() expected error for non-git directory")
	}
}

func TestDetectRepo_NormalRepo(t *testing.T) {
	dir := setupTestRepo(t)
	expectedBranch := getDefaultBranch(t, dir)

	info, err := DetectRepo(dir)
	if err != nil {
		t.Fatalf("DetectRepo() error = %v", err)
	}

	if info.RootDir != dir {
		t.Errorf("RootDir = %q, want %q", info.RootDir, dir)
	}
	if info.Branch != expectedBranch {
		t.Errorf("Branch = %q, want %q", info.Branch, expectedBranch)
	}
	if info.IsWorktree {
		t.Error("IsWorktree = true for a normal repo, want false")
	}
	if info.GitDir == "" {
		t.Error("GitDir is empty")
	}
	if len(info.Worktrees) == 0 {
		t.Error("Worktrees is empty, expected at least 1 entry")
	}
}

func TestDetectRepo_NonGitDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	dir := t.TempDir()

	_, err := DetectRepo(dir)
	if err == nil {
		t.Error("DetectRepo() expected error for non-git directory")
	}
}

func TestDetectRepo_NonExistentDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	_, err := DetectRepo("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Error("DetectRepo() expected error for non-existent directory")
	}
}

func TestListWorktrees_SingleRepo(t *testing.T) {
	dir := setupTestRepo(t)
	expectedBranch := getDefaultBranch(t, dir)

	worktrees, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("len(worktrees) = %d, want 1", len(worktrees))
	}

	wt := worktrees[0]
	if wt.Path != dir {
		t.Errorf("worktree Path = %q, want %q", wt.Path, dir)
	}
	if wt.Branch != expectedBranch {
		t.Errorf("worktree Branch = %q, want %q", wt.Branch, expectedBranch)
	}
	if !wt.IsMain {
		t.Error("worktree IsMain = false, want true for main worktree")
	}
	if wt.Head == "" {
		t.Error("worktree Head is empty")
	}
}

func TestListWorktrees_WithLinkedWorktree(t *testing.T) {
	dir := setupTestRepo(t)
	expectedBranch := getDefaultBranch(t, dir)

	// Create a linked worktree.
	worktreeDir := filepath.Join(t.TempDir(), "test-worktree")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "feature-branch")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add worktree: %v\n%s", err, out)
	}

	worktrees, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("len(worktrees) = %d, want 2", len(worktrees))
	}

	// First entry should be the main worktree.
	mainWt := worktrees[0]
	if !mainWt.IsMain {
		t.Error("first worktree should have IsMain=true")
	}
	if mainWt.Path != dir {
		t.Errorf("main worktree Path = %q, want %q", mainWt.Path, dir)
	}
	if mainWt.Branch != expectedBranch {
		t.Errorf("main worktree Branch = %q, want %q", mainWt.Branch, expectedBranch)
	}

	// Second entry should be the linked worktree.
	linkedWt := worktrees[1]
	if linkedWt.IsMain {
		t.Error("second worktree should have IsMain=false")
	}
	if linkedWt.Path != worktreeDir {
		t.Errorf("linked worktree Path = %q, want %q", linkedWt.Path, worktreeDir)
	}
	if linkedWt.Branch != "feature-branch" {
		t.Errorf("linked worktree Branch = %q, want %q", linkedWt.Branch, "feature-branch")
	}
	if linkedWt.Head == "" {
		t.Error("linked worktree Head is empty")
	}
}

func TestDetectRepo_LinkedWorktree(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a linked worktree.
	worktreeDir := filepath.Join(t.TempDir(), "linked-wt")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "wt-branch")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add worktree: %v\n%s", err, out)
	}

	info, err := DetectRepo(worktreeDir)
	if err != nil {
		t.Fatalf("DetectRepo() error = %v", err)
	}

	if !info.IsWorktree {
		t.Error("IsWorktree = false for a linked worktree, want true")
	}
	if info.Branch != "wt-branch" {
		t.Errorf("Branch = %q, want %q", info.Branch, "wt-branch")
	}
	if len(info.Worktrees) != 2 {
		t.Errorf("len(Worktrees) = %d, want 2", len(info.Worktrees))
	}
}

func TestDetectRepo_MainWorktreeNotLinked(t *testing.T) {
	dir := setupTestRepo(t)

	// Create a linked worktree so there are 2 worktrees total.
	worktreeDir := filepath.Join(t.TempDir(), "another-wt")
	cmd := exec.Command("git", "worktree", "add", worktreeDir, "-b", "another-branch")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("add worktree: %v\n%s", err, out)
	}

	// DetectRepo on the main working directory should NOT report IsWorktree.
	info, err := DetectRepo(dir)
	if err != nil {
		t.Fatalf("DetectRepo() error = %v", err)
	}

	if info.IsWorktree {
		t.Error("IsWorktree = true for the main worktree, want false")
	}
}

func TestRunGit_NonExistentDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	_, err := runGit("/nonexistent/path", "status")
	if err == nil {
		t.Error("runGit() expected error for non-existent directory")
	}
}

func TestRunGit_InvalidCommand(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	dir := t.TempDir()
	_, err := runGit(dir, "not-a-real-command")
	if err == nil {
		t.Error("runGit() expected error for invalid git command")
	}
}

func TestParseWorktreeBlock_Detached(t *testing.T) {
	lines := []string{
		"worktree /tmp/test",
		"HEAD abc1234def5678",
		"detached",
	}
	wt := parseWorktreeBlock(lines)

	if wt.Path != "/tmp/test" {
		t.Errorf("Path = %q, want %q", wt.Path, "/tmp/test")
	}
	if wt.Head != "abc1234" {
		t.Errorf("Head = %q, want %q", wt.Head, "abc1234")
	}
	if wt.Branch != "HEAD" {
		t.Errorf("Branch = %q, want %q for detached state", wt.Branch, "HEAD")
	}
}

func TestParseWorktreeBlock_Bare(t *testing.T) {
	lines := []string{
		"worktree /tmp/bare-repo",
		"bare",
	}
	wt := parseWorktreeBlock(lines)

	if wt.Path != "/tmp/bare-repo" {
		t.Errorf("Path = %q, want %q", wt.Path, "/tmp/bare-repo")
	}
	if !wt.IsBare {
		t.Error("IsBare = false, want true")
	}
}

func TestParseWorktreeBlock_NormalBranch(t *testing.T) {
	lines := []string{
		"worktree /tmp/test",
		"HEAD abc1234def5678",
		"branch refs/heads/feature/my-feature",
	}
	wt := parseWorktreeBlock(lines)

	if wt.Branch != "feature/my-feature" {
		t.Errorf("Branch = %q, want %q", wt.Branch, "feature/my-feature")
	}
}

func TestSplitWorktreeBlocks(t *testing.T) {
	input := "worktree /tmp/a\nHEAD abc\nbranch refs/heads/main\n\nworktree /tmp/b\nHEAD def\nbranch refs/heads/feature\n"
	blocks := splitWorktreeBlocks(input)

	if len(blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(blocks))
	}
	if len(blocks[0]) != 3 {
		t.Errorf("len(blocks[0]) = %d, want 3", len(blocks[0]))
	}
	if len(blocks[1]) != 3 {
		t.Errorf("len(blocks[1]) = %d, want 3", len(blocks[1]))
	}
}

func TestSplitWorktreeBlocks_Empty(t *testing.T) {
	blocks := splitWorktreeBlocks("")
	if len(blocks) != 0 {
		t.Errorf("len(blocks) = %d, want 0 for empty input", len(blocks))
	}
}

func TestCloneRepo_Success(t *testing.T) {
	sourceDir := setupTestRepo(t)
	destDir := filepath.Join(t.TempDir(), "cloned-repo")

	err := CloneRepo(sourceDir, destDir)
	if err != nil {
		t.Fatalf("CloneRepo() error = %v", err)
	}

	// Verify the clone is a valid git repo.
	if !IsGitRepo(destDir) {
		t.Error("cloned directory is not a git repo")
	}

	// Verify the README.md file exists.
	readmePath := filepath.Join(destDir, "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		t.Errorf("README.md not found in cloned repo: %v", err)
	}
}

func TestCloneRepo_DestExists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	sourceDir := setupTestRepo(t)
	destDir := t.TempDir() // This directory already exists.

	err := CloneRepo(sourceDir, destDir)
	if err == nil {
		t.Error("CloneRepo() expected error when destination exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestCloneRepo_InvalidURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	destDir := filepath.Join(t.TempDir(), "cloned-repo")

	// Test non-existent local path - git will fail to clone
	err := CloneRepo("/nonexistent/invalid/repo/path", destDir)
	if err == nil {
		t.Error("CloneRepo() expected error for invalid URL")
	}
}

func TestIsSafeRepoName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"valid-name", true},
		{"valid_name", true},
		{"ValidName123", true},
		{"123numeric", true},
		{"has space", false},
		{"has.dot", false},
		{"has/slash", false},
		{"has..dots", false},
		{"", false},
		{"../traversal", false},
		{"path/to/dir", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSafeRepoName(tt.name)
			if got != tt.want {
				t.Errorf("IsSafeRepoName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestIsAllowedGitURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://github.com/user/repo", true},
		{"http://example.com/repo", true},
		{"git://github.com/user/repo", true},
		{"ssh://git@github.com/user/repo", true},
		{"git@github.com:user/repo.git", true},
		{"file:///local/path", false},
		{"ext::sh -c whoami", false},
		{"/absolute/path", false},
		{"../relative/path", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := IsAllowedGitURL(tt.url)
			if got != tt.want {
				t.Errorf("IsAllowedGitURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestAddWorktree_Success(t *testing.T) {
	repoDir := setupTestRepo(t)
	wtDir := filepath.Join(t.TempDir(), "new-worktree")

	err := AddWorktree(repoDir, wtDir, "feature-test")
	if err != nil {
		t.Fatalf("AddWorktree() error = %v", err)
	}

	// Verify the worktree was created.
	if !IsGitRepo(wtDir) {
		t.Error("worktree directory is not a git repo")
	}

	// Verify the branch name.
	branch, err := CurrentBranch(wtDir)
	if err != nil {
		t.Fatalf("CurrentBranch() error = %v", err)
	}
	if branch != "feature-test" {
		t.Errorf("branch = %q, want %q", branch, "feature-test")
	}
}

func TestAddWorktree_DirExists(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	repoDir := setupTestRepo(t)
	wtDir := t.TempDir() // This directory already exists.

	err := AddWorktree(repoDir, wtDir, "feature-test")
	if err == nil {
		t.Error("AddWorktree() expected error when directory exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestAddWorktree_InvalidRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH, skipping test")
	}

	repoDir := t.TempDir() // Not a git repo.
	wtDir := filepath.Join(t.TempDir(), "new-worktree")

	err := AddWorktree(repoDir, wtDir, "feature-test")
	if err == nil {
		t.Error("AddWorktree() expected error for non-git directory")
	}
}
