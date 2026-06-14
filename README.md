# agent-playground

agent-playground is a Go-based coding-agent harness for running autonomous software tasks against a local workspace. It takes a task definition, sends it to an LLM provider, and lets the agent plan, edit files, run shell commands, test changes, ask follow-up questions, and persist its progress back into a task state file.

The project is designed as a playground for experimenting with agent-driven development workflows, including planning phases, tool use, self-evaluation, rework loops, Git integration, and optional pull-request creation.

## What this project does

At a high level, the agent performs the following workflow:

1. Loads a task from a JSON file.
2. Chooses an AI provider (Claude, OpenAI, or Ollama).
3. Builds a system prompt plus the task prompt.
4. Runs a planning phase with read-only tools.
5. Executes the task with a full tool set (file edits, shell commands, tests, Git operations, and more).
6. Records questions and answers for continuation.
7. Evaluates the result, optionally requests rework, and saves the final state.
8. Commits changes and, when configured, opens a pull request.

## Key features

- Autonomous task execution with tool calling
- Two-phase loop: planning first, execution second
- Built-in tools for reading, writing, searching, executing shell commands, running tests, and interacting with Git
- Pause-and-resume support via questions that require user answers
- Task state persistence in a JSON file so work can continue later
- Self-evaluation and rework loop based on a score threshold
- Support for Claude, OpenAI, and Ollama providers
- Docker support for repeatable runs

## Repository layout

- [cmd/agent](cmd/agent) contains the main entry point.
- [internal/agent](internal/agent) orchestrates the agent loop and tool execution.
- [internal/task](internal/task) defines the task model and persistence format.
- [internal/config](internal/config) loads environment-based configuration.
- [internal/provider](internal/provider) contains provider integrations.
- [internal/tool/builtin](internal/tool/builtin) defines the built-in tools available to the agent.
- [internal/skill](internal/skill) loads markdown-based skills.
- [internal/score](internal/score) evaluates results and supports rework.
- [examples](examples) contains sample task and skill files.
- [docs](docs) contains additional documentation and notes.

## Quick start

### Prerequisites

- Go 1.22 or newer
- Git
- An AI provider configured through environment variables
  - Claude: set ANTHROPIC_API_KEY
  - OpenAI: set OPENAI_API_KEY
  - Ollama: set OLLAMA_URL and optionally OLLAMA_MODEL

### 1. Clone and build

```bash
go mod tidy
make build
```

This produces a binary at bin/agent after the build step.

### 2. Prepare a workspace and task file

```bash
mkdir -p workspace skills
cp examples/task.json workspace/.task.json
cp -r examples/skills/. skills/
```

You can also point the agent at your own repository by changing WORKSPACE_PATH.

### 3. Configure environment variables

Example for Claude:

```bash
export ANTHROPIC_API_KEY="your-key"
export WORKSPACE_PATH="$PWD/workspace"
export SKILLS_PATH="$PWD/skills"
export TASK_FILE="$PWD/workspace/.task.json"
```

Example for OpenAI:

```bash
export OPENAI_API_KEY="your-key"
export WORKSPACE_PATH="$PWD/workspace"
export SKILLS_PATH="$PWD/skills"
export TASK_FILE="$PWD/workspace/.task.json"
```

Example for Ollama:

```bash
export OLLAMA_URL="http://localhost:11434"
export OLLAMA_MODEL="qwen2.5:3b"
export WORKSPACE_PATH="$PWD/workspace"
export SKILLS_PATH="$PWD/skills"
export TASK_FILE="$PWD/workspace/.task.json"
```

### 4. Run the agent

```bash
make run
```

Or run the binary directly:

```bash
./bin/agent
```

## How it works

### Task format

The agent reads a JSON task file with the following structure:

```json
{
  "title": "Example task",
  "description": "Describe the work to be done",
  "constraints": ["Keep it simple"],
  "score_threshold": 85,
  "questions": []
}
```

The task file is updated in place as the agent runs. It stores:

- the original task input
- any questions the agent asks
- the session state for pause/resume
- the final result, score, and optional PR link

A ready-to-run example is available at [examples/task.json](examples/task.json).

### Skills

Skills are markdown files placed in the skills directory. Each file can define:

- a persona section
- tool preferences
- knowledge bullets
- rules bullets

The agent uses these files to guide behavior and tool usage. Example skills are available under [examples/skills](examples/skills).

### Built-in tools

The agent can use built-in tools such as:

- read files
- write files
- edit files
- list directories
- search files
- execute shell commands
- run tests
- interact with Git
- ask the user questions when clarification is needed

## Configuration

The agent reads most settings from environment variables.

| Variable | Purpose | Default |
| --- | --- | --- |
| ANTHROPIC_API_KEY | Claude API key | none |
| OPENAI_API_KEY | OpenAI API key | none |
| OLLAMA_URL | Ollama base URL | none |
| CLAUDE_MODEL | Claude model name | claude-sonnet-4-20250514 |
| OPENAI_MODEL | OpenAI model name | gpt-40 |
| OLLAMA_MODEL | Ollama model name | qwen2.5:3b |
| WORKSPACE_PATH | Root directory the agent can edit | /workspace |
| SKILLS_PATH | Directory containing skill markdown files | /skills |
| TASK_FILE | Path to the task JSON file | /workspace/.task.json |
| MAX_ITERATIONS | Max planning/execution iterations | 50 |
| MAX_REWORK_ATTEMPTS | Number of rework cycles after evaluation | 2 |
| TOOL_TIMEOUT | Timeout for shell/tool execution in seconds | 60 |
| LOG_LEVEL | Logging verbosity | INFO |
| GIT_TOKEN | Token used for Git/PR actions | none |
| REPO_URL | Repository URL for PR creation | none |
| BASE_BRANCH | Base branch for Git setup | none |

## Running with Docker

A Docker image is included for containerized execution.

```bash
make docker-build
```

Example run:

```bash
docker run --rm \
  -e ANTHROPIC_API_KEY="your-key" \
  -v "$PWD/workspace:/workspace" \
  -v "$PWD/skills:/skills" \
  ai-agent:latest
```

## Development commands

Useful commands during development:

```bash
make test
make fmt
make lint
```

## Notes

- The agent saves progress and results back into the task JSON file, so a run can be resumed later.
- If the agent pauses because it needs clarification, it will record questions in the task state until you answer them.
- The project includes a scoring and rework loop, but you can tune the threshold to fit your workflow.
- If you want to automatically open pull requests, provide a Git token and repository URL.
