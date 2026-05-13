package git

import (
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Config holds git-related configuration.
type Config struct {
	Token      string
	WorkDir    string
	RepoURL    string // Optional: for cloning if workspace is empty
	BaseBranch string // Branch to branch off from (default: current)
}

// Setup ensures the workspace is a git repo and creates a new working branch.
// Returns the branch name created.
func Setup(cfg Config) (string, error) {
	if !isGitRepo(cfg.WorkDir) {
		if cfg.RepoURL == "" {
			return "", fmt.Errorf("workspace is not a git repo and no REPO_URL provided - cannot proceed")
		}
		if err := cloneRepo(cfg); err != nil {
			return "", fmt.Errorf("failed to clone repo: %w", err)
		}
	}

	// Configure git credentials if token provided
	if cfg.Token != "" {
		if err := configureCredentials(cfg); err != nil {
			return "", fmt.Errorf("failed to configure git credentials: %w", err)
		}
	}

	// Create and checkout a new branch
	branch := generateBranchName()
	if err := createBranch(cfg.WorkDir, branch, cfg.BaseBranch); err != nil {
		return "", fmt.Errorf("failed to create branch %s: %w", branch, err)
	}

	return branch, nil
}

// Push pushes the current branch to origin.
func Push(workDir, branch, token string) error {
	if token == "" {
		return fmt.Errorf("GIT_TOKEN required for push")
	}
	_, err := runGit(workDir, "push", "-u", "origin", branch)
	return err
}

// CommitAll stages and commits all changes with the given message.
func CommitAll(workDir, message string) error {
	if _, err := runGit(workDir, "add", "-A"); err != nil {
		return err
	}
	// Check if there's anything to commit
	out, _ := runGit(workDir, "status", " -- porcelain")
	if strings.TrimSpace(out) == "" {
		return nil // Nothing to commit
	}
	_, err := runGit(workDir, "commit", "-m", message)
	return err
}

func isGitRepo(dir string) bool {
	_, err := runGit(dir, "rev-parse", " -- git-dir")
	return err == nil
}

func cloneRepo(cfg Config) error {
	repoURL := injectToken(cfg.RepoURL, cfg.Token)
	cmd := exec.Command("git", "clone", repoURL, cfg.WorkDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", stderr.String(), err)
	}
	return nil
}

func configureCredentials(cfg Config) error {
	// Set credential helper to use the token for all HTTPS operations
	remoteURL, _ := runGit(cfg.WorkDir, "remote", "get-url", "origin")
	remoteURL = strings.TrimSpace(remoteURL)
	if remoteURL == "" {
		return nil
	}

	// Inject token into remote URL for push
	tokenURL := injectToken(remoteURL, cfg.Token)
	_, err := runGit(cfg.WorkDir, "remote", "set-url", "origin", tokenURL)
	return err
}

func injectToken(repoURL, token string) string {
	if token == "" {
		return repoURL
	}

	// Handle HTTPS URLs: https://github.com/user/repo https://token@github.com/user/repo
	if strings.HasPrefix(repoURL, "https://") {
		u, err := url.Parse(repoURL)
		if err != nil {
			return repoURL
		}

		u.User = url.UserPassword("x-access-token", token)
		return u.String()
	}

	return repoURL
}

func generateBranchName() string {
	ts := time.Now().Format("20060102-150405")
	return fmt.Sprintf("agent/%s", ts)
}

func createBranch(workDir, branch, baseBranch string) error {
	args := []string{"checkout", "-b", branch}
	if baseBranch != "" {
		args = append(args, baseBranch)
	}
	_, err := runGit(workDir, args...)
	return err
}

// CurrentBranch returns the current git branch name.
func CurrentBranch(workDir string) string {
	out, err := runGit(workDir, "rev-parse", " -- abbrev-ref", "HEAD")
	if err != nil {
		return ""
	}

	return strings.TrimSpace(out)
}

// IsClean returns true if the working directory has no uncommitted changes.
func IsClean(workDir string) bool {
	out, _ := runGit(workDir, "status", " -- porcelain")
	return strings.TrimSpace(out) == ""
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s: %v", strings.TrimSpace(stderr.String()), err)
	}

	return stdout.String(), nil
}

// SanitizeBranchName makes a string safe for use as a git branch name.
func SanitizeBranchName(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9\ -_ /]`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 50 {
		s = s[:50]
	}

	return s
}
