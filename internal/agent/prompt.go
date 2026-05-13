package agent

import (
	"fmt"
	"strings"

	"agent/internal/skill"
	"agent/internal/tool"
)

const baseSystemPrompt = ""

// PlanningInstructionPrompt is injected as a user turn to structurally force planning.
// Being a user-turn message (not system prompt) makes it signifcantly harder for the model to ignore.
const PlanningInstructionPrompt = ""

// ExecutionHandoffPrompt is injected to transition from planning to execution.
const ExecutionHandoffPrompt = ""

// BuildSystemPrompt constructs the full system prompt incorporating skills.
func BuildSystemPrompt(skills []skill.Skill, registry *tool.Registry) string {
	var parts []string
	parts = append(parts, baseSystemPrompt)

	// Add persona from skills
	persona := skill.MergePersonas(skills)
	if persona != "" {
		parts = append(parts, fmt.Sprintf("## Your Specialization\n\n%s", persona))
	}
	// Add knowledge
	knowledge := skill.MergeKnowledge(skills)
	if len(knowledge) > 0 {
		section := "## Reference Knowledge\n\n"
		for _, k := range knowledge {
			section += fmt.Sprintf("- %s\n", k)
		}

		parts = append(parts, section)
	}

	// Add rules
	rules := skill.MergeRules(skills)
	if len(rules) > 0 {
		section := "## Rules (MUST follow)\n\n"
		for _, r := range rules {
			section += fmt.Sprintf("- %s\n", r)
		}

		parts = append(parts, section)
	}

	// Add available tools summary
	enabled := registry.ListEnabled()
	if len(enabled) > 0 {
		section := fmt.Sprintf("## Available Tools\n\nYou have access to the following tools: %s", strings.Join(enabled, ", "))
		parts = append(parts, section)
	}

	return strings.Join(parts, "\n\n")
}
