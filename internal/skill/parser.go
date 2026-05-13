package skill

import (
	"strings"
)

func Parse(name, content string) Skill {
	s := Skill{Name: name, Raw: content}

	sections := splitSections(content)

	if len(sections) == 0 {
		s.Persona = strings.TrimSpace(content)
		return s
	}

	for heading, body := range sections {
		switch strings.ToLower(heading) {
		case "persona":
			s.Persona = body
		case "tools":
			s.Tools = parseToolConfig(body)
		case "knowledge":
			s.Knowledge = parseBulletList(body)
		case "rules":
			s.Rules = parseBulletList(body)
		default:
			if s.Persona != "" {
				s.Persona += "\n\n"
			}
			s.Persona += body
		}
	}

	return s
}

// splitSections splits markdown content by H1 headings (# Heading).
// Returns map of heading name -> body content.
func splitSections(content string) map[string]string {
	sections := make(map[string]string)
	lines := strings.Split(content, "\n")

	var currentHeading string
	var currentBody []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			// Save previous section
			if currentHeading != "" {
				sections[currentHeading] = strings.TrimSpace(strings.Join(currentBody, "\n"))
			}
			currentHeading = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			currentBody = nil
		} else {
			currentBody = append(currentBody, line)
		}
	}
	// Save last section
	if currentHeading != "" {
		sections[currentHeading] = strings.TrimSpace(strings.Join(currentBody, "\n"))
	}
	return sections
}

// parseToolConfig parses tool preferences from section body.
// Expected format:
//
// Preferred: tool1, tool2, tool3
// Disabled: tool4, tool5
func parseToolConfig(body string) ToolConfig {
	var cfg ToolConfig
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "preferred:") {
			val := strings.TrimPrefix(line, line[:strings.Index(line, ":")+1])
			cfg.Preferred = parseCSV(val)
		} else if strings.HasPrefix(lower, "disabled:") {
			val := strings.TrimPrefix(line, line[:strings.Index(line, ":")+1])
			cfg.Disabled = parseCSV(val)
		}
	}
	return cfg
}

// parseBulletList extracts items from a markdown bullet list.
func parseBulletList(body string) []string {
	var items []string
	lines := strings.Split(body, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Remove bullet prefix (-, *, •)
		for _, prefix := range []string{"- ", "* ", "• "} {
			if strings.HasPrefix(line, prefix) {
				line = strings.TrimPrefix(line, prefix)
				break
			}
		}

		items = append(items, strings.TrimSpace(line))
	}
	return items
}

// parseCSV splits a comma-separated string into trimmed tokens.
func parseCSV(s string) []string {
	var result []string
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
