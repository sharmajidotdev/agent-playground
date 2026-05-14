package pr

import (
	"agent/internal/logging"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"agent/internal/score"
)

// Platform represents a Git hosting platform.
type Platform string

const (
	PlatformGitHub Platform = "github"
	PlatformGitLab Platform = "gitlab"
)

// Config holds PR creation settings.
type Config struct {
	Platform   Platform //detected or from env
	Token      string   // GIT_TOKEN
	RepoOwner  string   // e.g., "myorg"
	RepoName   string   // e.g., "myrepo"
	BaseBranch string   // target branch for PR
	HeadBranch string   // the agent's working branch
	RemoteURL  string   // full remote URL for detection
}

// PRResult contains the output of creating a PR.
type PRResult struct {
	URL      string `json:"url"`
	Number   int    `json:"number"`
	Platform string `json:"platform"`
}

// CreatePR creates a pull request on the detected platform.
func CreatePR(ctx context.Context, cfg *Config, title, body string) (*PRResult, error) {
	platform := cfg.Platform
	if platform == "" {
		platform = DetectPlatform(cfg.RemoteURL)
	}
	if platform == "" {
		return nil, fmt.Errorf("unable to detect git platform from remote URL: %s", cfg.RemoteURL)
	}
	logging.Infof("[pr] Creating PR on %s: %s/%s (%s + %s)", platform, cfg.RepoOwner, cfg.RepoName, cfg.HeadBranch, cfg.BaseBranch)

	switch platform {
	case PlatformGitHub:
		return createGitHubPR(ctx, cfg, title, body)
	case PlatformGitLab:
		return createGitLabPR(ctx, cfg, title, body)
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}

// DetectPlatform determines the hosting platform from the remote URL.
func DetectPlatform(remoteURL string) Platform {
	lower := strings.ToLower(remoteURL)
	if strings.Contains(lower, "github.com") {
		return PlatformGitHub
	}
	if strings.Contains(lower, "gitlab.com") || strings.Contains(lower, "gitlab") {
		return PlatformGitLab
	}
	return ""
}

// ParseRemoteURL extracts owner and repo name from a git remote URL.
func ParseRemoteURL(remoteURL string) (owner, repo string) {
	// Handle HTTPS: https://github.com/owner/repo.git
	// Handle SSH: git@github.com:owner/repo.git
	url := remoteURL
	url = strings.TrimSuffix(url, ".git")

	if strings.Contains(url, "://") {
		// HTTPS format
		parts := strings.Split(url, "/")
		if len(parts) >= 2 {
			repo = parts[len(parts)-1]
			owner = parts[len(parts)-2]
		}
	} else if strings.Contains(url, ":") {
		// SSH format (git@host:owner/repo)
		colonIdx := strings.LastIndex(url, ":")
		path := url[colonIdx+1:]
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			owner = parts[0]
			repo = parts[1]
		}
	}
	return
}

// GetRemoteURL gets the remote URL from the workspace git config.
func GetRemoteURL(workDir string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// BuildPRBody creates a formatted PR description.
func BuildPRBody(taskDescription string, summary string, sc *score.Score, filesChanged []string) string {
	var b strings.Builder

	b.WriteString("## Task\n\n")
	b.WriteString(taskDescription)
	b.WriteString("\n\n")

	if summary != "" {
		b.WriteString("## Summary\n\n")
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	b.WriteString("## Score\n\n")
	b.WriteString(fmt.Sprintf(" ** Total: %d/100 ** \n\n", sc.Total))
	b.WriteString("| Component | Score |\n| -------| ------- |\n")
	for component, pts := range sc.Breakdown {
		b.WriteString(fmt.Sprintf("| %s | %d |\n", component, pts))
	}
	b.WriteString("\n")

	if len(sc.Details) > 0 {
		b.WriteString("### Notes\n\n")
		for _, d := range sc.Details {
			b.WriteString(fmt.Sprintf("- %s\n", d))
		}
		b.WriteString("\n")
	}

	if len(filesChanged) > 0 {
		b.WriteString("## Files Changed\n\n")
		for _, f := range filesChanged {
			b.WriteString(fmt.Sprintf("- `%s'\n", f))
		}
		b.WriteString("\n")
	}

	b.WriteString(" --- \n*Created by AI Coding Agent*\n")
	return b.String()
}

// GetChangedFiles returns a list of files changed on the branch vs base.
func GetChangedFiles(workDir, baseBranch string) []string {
	cmd := exec.Command("git", "diff", " --name-only", baseBranch+" .. HEAD")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		if l != "" {
			files = append(files, l)
		}
	}
	return files
}

// --- GitHub API ---

func createGitHubPR(ctx context.Context, cfg *Config, title, body string) (*PRResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", cfg.RepoOwner, cfg.RepoName)

	payload := map[string]interface{}{
		"title": title,
		"body":  body,
		"head":  cfg.HeadBranch,
		"base":  cfg.BaseBranch,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal PR payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logging.Errorf("GitHub API request: %v", err)
		return nil, fmt.Errorf("GitHub API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		logging.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse GitHub response: %w", err)
	}

	return &PRResult{
		URL:      result.HTMLURL,
		Number:   result.Number,
		Platform: string(PlatformGitHub),
	}, nil
}

// --- GitLab API ---

func createGitLabPR(ctx context.Context, cfg *Config, title, body string) (*PRResult, error) {
	// GitLab uses project path encoding: owner%2Frepo
	projectPath := fmt.Sprintf("%s%%2F%s", cfg.RepoOwner, cfg.RepoName)
	url := fmt.Sprintf("https://gitlab.com/api/v4/projects/%s/merge_requests", projectPath)

	payload := map[string]interface{}{
		"title":         title,
		"description":   body,
		"source_branch": cfg.HeadBranch,
		"target_branch": cfg.BaseBranch,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal MR payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", cfg.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logging.Errorf("GitLab API request: %v", err)
		return nil, fmt.Errorf("GitLab API request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		logging.Errorf("GitLab API returned %d: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("GitLab API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		WebURL string `json:"web_url"`
		IID    int    `json:"iid"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("parse GitLab response: %w", err)
	}

	return &PRResult{
		URL:      result.WebURL,
		Number:   result.IID,
		Platform: string(PlatformGitLab),
	}, nil
}
