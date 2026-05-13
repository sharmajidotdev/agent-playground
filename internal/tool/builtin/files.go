package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ReadFile implements a tool for reading file contents.
type ReadFile struct {
	WorkDir string
}

func (t *ReadFile) Name() string { return "read_file" }
func (t *ReadFile) Description() string {
	return "Read the contents of a file. Supports optional line range (start_line, end_line) for partial reads."
}

func (t *ReadFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Relative path to the file from the workspace root"
			},
			"start_line": {
				"type": "integer",
				"description": "Starting line number (1-based, optional)"
			},
			"end_line": {
				"type": "integer",
				"description": "Ending line number (1-based, inclusive, optional)"
			}
		},
		"required": ["path"]
	}`)
}

func (t *ReadFile) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path      string `json:"path"`
		StartLine int    `json:"start_line"`
		EndLine   int    `json:"end_line"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := t.resolvePath(params.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	if params.StartLine > 0 || params.EndLine > 0 {
		lines := strings.Split(content, "\n")
		start := params.StartLine
		end := params.EndLine
		if start < 1 {
			start = 1
		}
		if end < 1 || end > len(lines) {
			end = len(lines)
		}
		if start > len(lines) {
			return "", fmt.Errorf("start_line %d exceeds file length %d", start, len(lines))
		}
		content = strings.Join(lines[start-1:end], "\n")
	}

	return content, nil
}

func (t *ReadFile) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.WorkDir, path)
}

// WriteFile implements a tool for creating/overwriting files.
type WriteFile struct {
	WorkDir string
}

func (t *WriteFile) Name() string { return "write_file" }
func (t *WriteFile) Description() string {
	return "Create or overwrite a file with the given content. Parent directories are created automatically."
}

func (t *WriteFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Relative path to the file from the workspace root"
			},
			"content": {
				"type": "string",
				"description": "The full content to write to the file"
			}
		},
		"required": ["path", "content"]
	}`)
}

func (t *WriteFile) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := t.resolvePath(params.Path)

	// Ensure parent directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(fullPath, []byte(params.Content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(params.Content), params.Path), nil
}

func (t *WriteFile) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.WorkDir, path)
}

// EditFile implements a tool for search-and-replace edits within a file.
type EditFile struct {
	WorkDir string
}

func (t *EditFile) Name() string { return "edit_file" }
func (t *EditFile) Description() string {
	return "Edit a file by replacing an exact string with new content. The old_string must match exactly (including whitespace)."
}

func (t *EditFile) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Relative path to the file from the workspace root"
			},
			"old_string": {
				"type": "string",
				"description": "The exact text to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The text to replace old_string with"
			}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *EditFile) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := t.resolvePath(params.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	content := string(data)
	count := strings.Count(content, params.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in file %s", params.Path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_string matches %d locations in %s - make it more specific", count, params.Path)
	}

	newContent := strings.Replace(content, params.OldString, params.NewString, 1)
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully edited %s", params.Path), nil
}

func (t *EditFile) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.WorkDir, path)
}

// ListDirectory implements a tool for listing directory contents.
type ListDirectory struct {
	WorkDir string
}

func (t *ListDirectory) Name() string { return "list_directory" }
func (t *ListDirectory) Description() string {
	return "List the contents of a directory. Returns file/folder names (folders end with /)."
}

func (t *ListDirectory) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Relative path to the directory from the workspace root. Use '.' for workspace root."
			}
		},
		"required": ["path"]
	}`)
}

func (t *ListDirectory) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	fullPath := t.resolvePath(params.Path)
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var lines []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		} else {
			info, _ := entry.Info()
			if info != nil {
				name += " (" + strconv.FormatInt(info.Size(), 10) + " bytes)"
			}
		}
		lines = append(lines, name)
	}

	if len(lines) == 0 {
		return "(empty directory)", nil
	}
	return strings.Join(lines, "\n"), nil
}

func (t *ListDirectory) resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(t.WorkDir, path)
}

// SearchFiles implements grep-like search across workspace files.
type SearchFiles struct {
	WorkDir string
}

func (t *SearchFiles) Name() string { return "search_files" }
func (t *SearchFiles) Description() string {
	return "Search for a text pattern across files in the workspace. Returns matching lines with file paths and line numbers."
}

func (t *SearchFiles) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "The text pattern to search for (case-insensitive)"
			},
			"path": {
				"type": "string",
				"description": "Optional subdirectory to limit search to. Defaults to entire workspace."
			},
			"file_pattern": {
				"type": "string",
				"description": "Optional glob pattern for file names (e.g. '*.go', '*.ts')"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *SearchFiles) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Pattern     string `json:"pattern"`
		Path        string `json:"path"`
		FilePattern string `json:"file_pattern"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	searchDir := t.WorkDir
	if params.Path != "" {
		searchDir = filepath.Join(t.WorkDir, params.Path)
	}

	pattern := strings.ToLower(params.Pattern)
	var results []string
	maxResults := 50

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip hidden directories and common non-source dirs
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 1024*1024 { // Skip files > 1MB
			return nil
		}

		// Check file pattern match
		if params.FilePattern != "" {
			matched, _ := filepath.Match(params.FilePattern, info.Name())
			if !matched {
				return nil
			}
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		relPath, _ := filepath.Rel(t.WorkDir, path)

		for i, line := range lines {
			if len(results) >= maxResults {
				return fmt.Errorf("max results reached")
			}
			if strings.Contains(strings.ToLower(line), pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", relPath, i+1, strings.TrimSpace(line)))
			}
		}
		return nil
	})

	if err != nil && err.Error() != "max results reached" {
		return "", fmt.Errorf("search error: %w", err)
	}

	if len(results) == 0 {
		return "No matches found.", nil
	}

	output := strings.Join(results, "\n")
	if len(results) >= maxResults {
		output += fmt.Sprintf("\n\n(showing first %d results)", maxResults)
	}
	return output, nil
}
