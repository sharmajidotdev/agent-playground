package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ShellExec implements a tool for running shell commands.
type ShellExec struct {
	WorkDir string
	Timeout time.Duration
}

func (t *ShellExec) Name() string { return "shell_exec" }
func (t *ShellExec) Description() string {
	return "Execute a shell command in the workspace directory. Returns stdout and stderr. Use for running builds, installs, or any CLI operation."
}

func (t *ShellExec) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			},
			"working_dir": {
				"type": "string",
				"description": "Optional working directory relative to workspace root"
			}
		},
		"required": ["command"]
	}`)
}

func (t *ShellExec) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Command    string `json:"command"`
		WorkingDir string `json:"working_dir"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", params.Command)
	cmd.Dir = t.WorkDir
	if params.WorkingDir != "" {
		cmd.Dir = params.WorkingDir
		if !strings.HasPrefix(cmd.Dir, "/") {
			cmd.Dir = t.WorkDir + "/" + params.WorkingDir
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("STDERR:\n")
		result.WriteString(stderr.String())
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return result.String(), fmt.Errorf("command timed out after %v", timeout)
		}
		// Include the error but still return output
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(fmt.Sprintf("Exit error: %v", err))
	}

	output := result.String()
	// Truncate very long output
	const maxOutput = 50000
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n\n... (output truncated)"
	}

	return output, nil
}

// Git implements a tool for git operations.
type Git struct {
	WorkDir string
}

func (t *Git) Name() string { return "git" }
func (t *Git) Description() string {
	return "Run git commands in the workspace. Supports status, diff, add, commit, log, and other git operations."
}

func (t *Git) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "Git subcommand and arguments (e.g. 'status', 'diff', 'add .', 'commit -m \"message\"')"
			}
		},
		"required": ["command"]
	}`)
}

func (t *Git) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	// Safety: block remote-modifying commands (push is handled by the framework)
	lower := strings.ToLower(strings.TrimSpace(params.Command))
	blocked := []string{"push", "remote add", "remote set-url", "remote remove", "fetch", "pull"}
	for _, b := range blocked {
		if strings.HasPrefix(lower, b) {
			return "", fmt.Errorf("git %s is not allowed — the framework handles remote operations", b)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", strings.Fields(params.Command)...)
	cmd.Dir = t.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(stderr.String())
	}

	if err != nil {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(fmt.Sprintf("git error: %v", err))
	}

	return result.String(), nil
}

// RunTests implements a tool for detecting and running tests.
type RunTests struct {
	WorkDir string
	Timeout time.Duration
}

func (t *RunTests) Name() string { return "run_tests" }
func (t *RunTests) Description() string {
	return "Detect the project's test framework and run tests. Automatically detects Go, Node.js (npm/yarn), Python (pytest/unittest), and Rust (cargo) projects."
}

func (t *RunTests) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Optional subdirectory or specific test file to run"
			},
			"filter": {
				"type": "string",
				"description": "Optional test name filter/pattern"
			}
		},
		"required": []
	}`)
}

func (t *RunTests) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Path   string `json:"path"`
		Filter string `json:"filter"`
	}
	if input != nil && string(input) != "null" && string(input) != "{}" {
		if err := json.Unmarshal(input, &params); err != nil {
			return "", fmt.Errorf("invalid input: %w", err)
		}
	}

	timeout := t.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	testCmd := t.detectTestCommand(params.Path, params.Filter)
	if testCmd == "" {
		return "", fmt.Errorf("could not detect test framework in workspace")
	}

	cmd := exec.CommandContext(ctx, "bash", "-c", testCmd)
	cmd.Dir = t.WorkDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Command: %s\n\n", testCmd))

	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if stdout.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(stderr.String())
	}

	if err != nil {
		result.WriteString(fmt.Sprintf("\n\nTests FAILED: %v", err))
	} else {
		result.WriteString("\n\nTests PASSED")
	}

	output := result.String()
	const maxOutput = 50000
	if len(output) > maxOutput {
		output = output[:maxOutput] + "\n\n... (output truncated)"
	}

	return output, nil
}

func (t *RunTests) detectTestCommand(path, filter string) string {
	// Check for Go
	if fileExists(t.WorkDir + "/go.mod") {
		cmd := "go test ./..."
		if path != "" {
			cmd = "go test ./" + path + "/..."
		}
		if filter != "" {
			cmd += " -run " + filter
		}
		return cmd
	}

	// Check for Node.js (package.json)
	if fileExists(t.WorkDir + "/package.json") {
		if fileExists(t.WorkDir + "/yarn.lock") {
			return "yarn test"
		}
		return "npm test"
	}

	// Check for Python
	if fileExists(t.WorkDir+"/pytest.ini") || fileExists(t.WorkDir+"/setup.py") || fileExists(t.WorkDir+"/pyproject.toml") {
		cmd := "pytest"
		if path != "" {
			cmd += " " + path
		}
		if filter != "" {
			cmd += " -k " + filter
		}
		return cmd
	}

	// Check for Rust
	if fileExists(t.WorkDir + "/Cargo.toml") {
		cmd := "cargo test"
		if filter != "" {
			cmd += " " + filter
		}
		return cmd
	}

	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
