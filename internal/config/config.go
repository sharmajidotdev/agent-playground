package config

import (
	"fmt"
	"os"
	"strconv"
)

// ProviderType represents the AI provider to use.
type ProviderType string

const (
	ProviderClaude ProviderType = "claude"
	ProviderOpenAI ProviderType = "openai"
	ProviderOllama ProviderType = "ollama"
)

// Config holds all configuration for the agent.
type Config struct {
	// Provider settings
	Provider     ProviderType
	AnthropicKey string
	OpenAIKey    string
	OllamaURL    string
	ClaudeModel  string
	OpenAIModel  string
	OllamaModel  string

	// Paths
	WorkspacePath string
	SkillsPath    string
	TaskFile      string

	// Git settings
	GitToken    string
	RepoURL     string
	BaseBranch  string
	GitPlatform string // "github" or "gitlab" - auto-detected if empty

	// Agent settings
	MaxIterations     int
	MaxReworkAttempts int // how many times to rework if score is below threshold
	ToolTimeout       int // seconds
}

// Load reads configuration from environment variables and applies defaults.
func Load() (*Config, error) {
	cfg := &Config{
		AnthropicKey:      os.Getenv("ANTHROPIC_API_KEY"),
		OpenAIKey:         os.Getenv("OPENAI_API_KEY"),
		OllamaURL:         os.Getenv("OLLAMA_URL"),
		ClaudeModel:       getEnvOr("CLAUDE_MODEL", "claude-sonnet-4-20250514"),
		OpenAIModel:       getEnvOr("OPENAI_MODEL", "gpt-40"),
		OllamaModel:       getEnvOr("OLLAMA_MODEL", "llama3.2"),
		WorkspacePath:     getEnvOr("WORKSPACE_PATH", "/workspace"),
		SkillsPath:        getEnvOr("SKILLS_PATH", "/skills"),
		TaskFile:          getEnvOr("TASK_FILE", "/workspace/.task.json"),
		GitToken:          os.Getenv("GIT_TOKEN"),
		RepoURL:           os.Getenv("REPO_URL"),
		BaseBranch:        os.Getenv("BASE_BRANCH"),
		GitPlatform:       os.Getenv("GIT_PLATFORM"),
		MaxIterations:     getEnvInt("MAX_ITERATIONS", 50),
		MaxReworkAttempts: getEnvInt("MAX_REWORK_ATTEMPTS", 2),
		ToolTimeout:       getEnvInt("TOOL_TIMEOUT", 60),
	}

	// Select provider by priority: Claude > OpenAI > Ollama
	switch {
	case cfg.AnthropicKey != "":
		cfg.Provider = ProviderClaude
	case cfg.OpenAIKey != "":
		cfg.Provider = ProviderOpenAI
	case cfg.OllamaURL != "":
		cfg.Provider = ProviderOllama
	default:
		return nil, fmt.Errorf("no AI provider configured: set ANTHROPIC_API_KEY, OPENAI_API_KEY, or OLLAMA_URL")
	}
	return cfg, nil
}

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
