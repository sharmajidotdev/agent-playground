package builtin

import (
	"time"

	"agent/internal/tool"
)

// Tools holds references to special tools that the agent loop needs to inspect.
type Tools struct {
	AskQuestion *AskQuestion
}

// RegisterAll registers all built-in tools with the given registry.
// Returns references to tools that need special handling in the agent loop.
func RegisterAll(registry *tool.Registry, workDir string, toolTimeout time.Duration) *Tools {
	registry.Register(&ReadFile{WorkDir: workDir})
	registry.Register(&WriteFile{WorkDir: workDir})
	registry.Register(&EditFile{WorkDir: workDir})
	registry.Register(&ListDirectory{WorkDir: workDir})
	registry.Register(&SearchFiles{WorkDir: workDir})
	registry.Register(&ShellExec{WorkDir: workDir, Timeout: toolTimeout})
	registry.Register(&Git{WorkDir: workDir})
	registry.Register(&RunTests{WorkDir: workDir, Timeout: toolTimeout * 2})

	askQ := &AskQuestion{}
	registry.Register(askQ)

	return &Tools{AskQuestion: askQ}
}
