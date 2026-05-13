package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"agent/internal/skill"
)

// Tool defines the interface that all tools must implement.
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description of what the tool does.
	Description() string

	// Schema returns the JSON Schema for the tool's input parameters.
	Schema() json.RawMessage

	// Execute runs the tool with the given input and returns the result.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}

// Definition represents a tool definition for sending to AI providers.
type Definition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Registry manages available tools and applies skill-based filtering.
type Registry struct {
	tools     map[string]Tool
	preferred []string
	disabled  map[string]bool
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools:    make(map[string]Tool),
		disabled: make(map[string]bool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// ApplySkillConfig applies tool filtering from merged skill configurations.
// Disabled tools are removed from availability; preferred tools are listed first.
func (r *Registry) ApplySkillConfig(cfg skill.ToolConfig) {
	for _, name := range cfg.Disabled {
		r.disabled[name] = true
	}
	r.preferred = cfg.Preferred
}

// Get returns a tool by name, or nil if not found/disabled.
func (r *Registry) Get(name string) Tool {
	if r.disabled[name] {
		return nil
	}
	return r.tools[name]
}

// Execute runs a tool by name with the given input.
func (r *Registry) Execute(ctx context.Context, name string, input json.RawMessage) (string, error) {
	t := r.Get(name)
	if t == nil {
		return "", fmt.Errorf("tool %q not found or is disabled", name)
	}
	return t.Execute(ctx, input)
}

// Definitions returns tool definitions for all enabled tools, with preferred tools first.
func (r *Registry) Definitions() []Definition {
	var preferred []Definition
	var rest []Definition

	preferredSet := make(map[string]bool)
	for _, name := range r.preferred {
		preferredSet[name] = true
	}

	// Collect all enabled tools
	var allNames []string
	for name := range r.tools {
		if !r.disabled[name] {
			allNames = append(allNames, name)
		}
	}
	sort.Strings(allNames)

	for _, name := range allNames {
		t := r.tools[name]
		def := Definition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		}
		if preferredSet[name] {
			preferred = append(preferred, def)
		} else {
			rest = append(rest, def)
		}
	}

	return append(preferred, rest...)
}

// ReadOnlyToolNames are tools that only read state — safe to use during the planning phase.
// Write/execute tools are excluded so the AI structurally cannot mutate anything until planning is done.
var ReadOnlyToolNames = []string{"read_file", "list_directory", "search_files", "ask_question"}

// DefinitionsFor returns tool definitions for the given tool names (if enabled).
// Use this to restrict the tool set passed to the provider for a specific phase.
func (r *Registry) DefinitionsFor(names []string) []Definition {
	var defs []Definition
	for _, name := range names {
		if r.disabled[name] {
			continue
		}
		t, ok := r.tools[name]
		if !ok {
			continue
		}
		defs = append(defs, Definition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}
	return defs
}

// ListEnabled returns names of all enabled tools.
func (r *Registry) ListEnabled() []string {
	var names []string
	for name := range r.tools {
		if !r.disabled[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
