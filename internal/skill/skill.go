package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ToolConfig struct {
	Preferred []string
	Disabled  []string
}

type Skill struct {
	Name      string
	Persona   string
	Tools     ToolConfig
	Knowledge []string
	Rules     []string
	Raw       string
}

func LoadAll(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read skill directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)

	var skills []Skill
	for _, f := range files {
		path := filepath.Join(dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read skill file %s: %w", path, err)
		}
		name := strings.TrimSuffix(f, ".md")
		skill := Parse(name, string(data))
		skills = append(skills, skill)
	}
	return skills, nil
}

// MergePersonas combines persona sections from all skills.
func MergePersonas(skills []Skill) string {
	var parts []string
	for _, s := range skills {
		if s.Persona != "" {
			parts = append(parts, s.Persona)
		}
	}
	return strings.Join(parts, "\n\n")
}

// MergeKnowledge combines knowledge items from all skills.
func MergeKnowledge(skills []Skill) []string {
	var all []string
	for _, s := range skills {
		all = append(all, s.Knowledge...)
	}
	return all
}

// MergeRules combines rules from all skills.
func MergeRules(skills []Skill) []string {
	var all []string
	for _, s := range skills {
		all = append(all, s.Rules...)
	}
	return all
}

// MergeToolConfigs merges tool configs. Disabled wins over Preferred if conflict.
func MergeToolConfigs(skills []Skill) ToolConfig {
	disabledSet := make(map[string]bool)
	preferredSet := make(map[string]bool)

	for _, s := range skills {
		for _, t := range s.Tools.Disabled {
			disabledSet[t] = true
		}
		for _, t := range s.Tools.Preferred {
			preferredSet[t] = true
		}
	}

	// Disabled wins over preferred
	for t := range disabledSet {
		delete(preferredSet, t)
	}
	var cfg ToolConfig
	for t := range preferredSet {
		cfg.Preferred = append(cfg.Preferred, t)
	}
	for t := range disabledSet {
		cfg.Disabled = append(cfg.Disabled, t)
	}
	sort.Strings(cfg.Preferred)
	sort.Strings(cfg.Disabled)
	return cfg
}
