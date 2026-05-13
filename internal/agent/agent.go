package agent

import (
	"agent/internal/message"
	"agent/internal/provider"
	"agent/internal/task"
	"agent/internal/tool"
	"agent/internal/tool/builtin"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Agent orchestrates the AI coding Loop.
type Agent struct {
	provider      provider.Provider
	registry      *tool.Registry
	tools         *builtin.Tools
	task          *task.Task
	maxIterations int
	conversation  *message.Conversation
}

// New creates a new Agent.
func New(p provider.Provider, registry *tool.Registry, tools *builtin.Tools, t *task.Task, maxIterations int) *Agent {
	return &Agent{
		provider:      p,
		registry:      registry,
		tools:         tools,
		task:          t,
		maxIterations: maxIterations,
	}
}

// Run executes the agent loop with the given system and user task.
// It enforces a two-phase approach: planning (read-only tools) then execution (all tools).
func (a *Agent) Run(ctx context.Context, systemPrompt, taskPrompt string) error {
	a.conversation = message.NewConversation(systemPrompt)
	a.conversation.AddUserMessage(taskPrompt)

	a.task.EnsureSession()
	a.task.Session.Status = task.StatusInProgress

	var totalUsage message.Usage
	log.Printf("[agent] Starting with provider: %s, max iterations: %d", a.provider.Name(), a.maxIterations)

	// Phase 1: Planning - inject planning instruction and run with read-only tools.
	// The AI structurally cannot call write/execute tools here because they are not passed.
	if err := a.runPlanningPhase(ctx, &totalUsage); err != nil {
		return err
	}

	if a.task.Session.Status == task.StatusPaused || a.task.Session.Status == task.StatusFailed {
		return nil
	}

	// Phase 2: Execution - inject handoff message and run with full tool access.
	a.conversation.AddUserMessage(ExecutionHandoffPrompt)
	log.Printf("[agent] Starting execution phase")

	for iteration := 1; iteration <= a.maxIterations; iteration++ {
		log.Printf("[agent] === Iteration %d/%d === ", iteration, a.maxIterations)

		resp, err := a.provider.SendMessage(ctx, a.conversation, a.registry.Definitions())
		if err != nil {
			a.setFailed(iteration, totalUsage, fmt.Sprintf("provider error: %v", err))
			return err
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		// No tool calls - agent is done
		if !resp.HasToolCalls() {
			a.setCompleted(iteration, totalUsage, resp.Content)
			return nil
		}

		// Record assistant message and execute tools
		a.recordAssistantBlocks(resp)
		a.executeToolCalls(ctx, resp.ToolCalls)

		// Check if agent asked a question - pause
		if a.tools.AskQuestion.HasPendingQuestions() {
			a.setPaused(iteration, totalUsage)
			return nil
		}

		a.task.Session.Iterations = iteration
	}

	a.setFailed(a.maxIterations, totalUsage, "max iterations reached")
	return nil
}

// runPLanningPhase runs the agent with read-only tools until it produces a text plan.
// This structurally prevents any writes during planning - the tools are not offered to the provider.
func (a *Agent) runPlanningPhase(ctx context.Context, totalUsage *message.Usage) error {
	readonlyDefs := a.registry.DefinitionsFor(tool.ReadOnlyToolNames)
	a.conversation.AddUserMessage(PlanningInstructionPrompt)
	log.Printf("[agent] Starting planning phase (read-only tools: %d available)", len(readonlyDefs))

	maxPlanIter := a.maxIterations / 3
	if maxPlanIter < 5 {
		maxPlanIter = 5
	}

	for iteration := 1; iteration <= maxPlanIter; iteration++ {
		log.Printf("[agent] === Planning iteration %d/%d === ", iteration, maxPlanIter)

		resp, err := a.provider.SendMessage(ctx, a.conversation, readonlyDefs)
		if err != nil {
			a.setFailed(iteration, *totalUsage, fmt.Sprintf("provider error in planning: %v", err))
			return err
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		// No tool calls means the AI has written its plan - phase complete
		if !resp.HasToolCalls() {
			a.conversation.AddAssistantMessage(resp.Content, nil)
			log.Printf("[agent] Plan produced after %d planning iterations", iteration)
			return nil
		}

		a.recordAssistantBlocks(resp)
		a.executeToolCalls(ctx, resp.ToolCalls)

		if a.tools.AskQuestion.HasPendingQuestions() {
			a.setPaused(iteration, *totalUsage)
			return nil
		}
	}
	// Planning timed out - Log a warning but continue to execution rather than hard-failing
	log.Printf("[agent] Warning: planning phase reached max iterations (%d) without producing a plan - proceeding to execution", maxPlanIter)
	return nil
}

func (a *Agent) setCompleted(iterations int, usage message.Usage, summary string) {
	a.task.Session.Status = task.StatusCompleted
	a.task.Session.Iterations = iterations
	a.task.Result = &task.Result{
		Status:      "success",
		Summary:     summary,
		Branch:      a.task.Session.Branch,
		Iterations:  iterations,
		Provider:    a.provider.Name(),
		TotalTokens: usage.InputTokens + usage.OutputTokens,
	}
	log.Printf("[agent] Completed in %d iterations", iterations)
}

func (a *Agent) setFailed(iterations int, usage message.Usage, reason string) {
	a.task.EnsureSession()
	a.task.Session.Status = task.StatusFailed
	a.task.Session.Iterations = iterations
	a.task.Result = &task.Result{
		Status:      "failure",
		Summary:     reason,
		Branch:      a.task.Session.Branch,
		Iterations:  iterations,
		Provider:    a.provider.Name(),
		TotalTokens: usage.InputTokens + usage.OutputTokens,
		Error:       reason,
	}
}

func (a *Agent) setPaused(iterations int, usage message.Usage) {
	questions := a.tools.AskQuestion.DrainQuestions()
	for _, q := range questions {
		a.task.AddQuestion(q.ID, q.Question, q.Context, q.Options)
	}
	a.task.Session.Status = task.StatusPaused
	a.task.Session.Iterations = iterations
	a.task.Result = &task.Result{
		Status:      "paused",
		Summary:     "Waiting for user answers to questions",
		Branch:      a.task.Session.Branch,
		Iterations:  iterations,
		Provider:    a.provider.Name(),
		TotalTokens: usage.InputTokens + usage.OutputTokens,
	}
	log.Printf("[agent] Paused: %d questions pending", len(questions))
}
func (a *Agent) recordAssistantBlocks(resp *message.Response) {
	var blocks []message.Block
	if resp.Content != "" {
		blocks = append(blocks, message.Block{Type: message.BlockText, Text: resp.Content})
	}
	for _, tc := range resp.ToolCalls {
		blocks = append(blocks, message.Block{
			Type:      message.BlockToolUse,
			ToolUseID: tc.ID,
			ToolName:  tc.Name,
			Input:     tc.Input,
		})
	}
	a.conversation.AddAssistantMessage(resp.Content, blocks)
}

func (a *Agent) executeToolCalls(ctx context.Context, calls []message.ToolCall) {
	var results []message.ToolResult
	for _, tc := range calls {
		log.Printf("[agent] Tool: %s", tc.Name)

		toolCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		output, err := a.registry.Execute(toolCtx, tc.Name, tc.Input)
		cancel()

		tr := message.ToolResult{ToolUseID: tc.ID}
		if err != nil {
			tr.Content = fmt.Sprintf("Error: %v", err)
			tr.IsError = true
		} else {
			tr.Content = output
		}
		results = append(results, tr)
	}
	a.conversation.AddToolResults(results)
}

// ResultJSON returns the task result as formatted JSON.
func ResultJSON(t *task.Task) ([]byte, error) {
	return json.MarshalIndent(t, "", " ")
}
