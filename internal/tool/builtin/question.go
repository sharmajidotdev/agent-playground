package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"agent/internal/logging"
)

// PendingQuestion holds a question the agent wants to ask.
type PendingQuestion struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Context  string   `json:"context"`
	Options  []string `json:"options,omitempty"`
}

// AskQuestion implements a tool that lets the agent ask the user a question.
// When invoked, it records the question and signals the agent loop to pause.
type AskQuestion struct {
	mu        sync.Mutex
	questions []PendingQuestion
	counter   int
}

func (t *AskQuestion) Name() string { return "ask_question" }
func (t *AskQuestion) Description() string {
	return "Ask the user a question when you need clarification or a decision. The container will pause and the user will provide an answer in the task file. Use this when there are multiple valid approaches and the choice matters for the outcome."
}

func (t *AskQuestion) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "The question to ask the user. Be specific and concise."
			},
			"context": {
				"type": "string",
				"description": "Brief context explaining why this question matters and what you've found so far."
			},
			"options": {
				"type": "array",
				"items": {"type": "string"},
				"description": "Optional list of suggested options/choices for the user."
			}
		},
		"required": ["question", "context"]
	}`)
}

func (t *AskQuestion) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Question string   `json:"question"`
		Context  string   `json:"context"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	// Log the received parameters for debugging
	logging.Debugf("[ask_question] Received question='%s', context='%s', options=%v", params.Question, params.Context, params.Options)

	t.mu.Lock()
	t.counter++
	q := PendingQuestion{
		ID:       fmt.Sprintf("q%d", t.counter),
		Question: params.Question,
		Context:  params.Context,
		Options:  params.Options,
	}
	t.questions = append(t.questions, q)
	t.mu.Unlock()

	return fmt.Sprintf("Question recorded (id: %s). The container will pause after this iteration so the user can answer.", q.ID), nil
}

// HasPendingQuestions returns true if any questions were asked.
func (t *AskQuestion) HasPendingQuestions() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.questions) > 0
}

// DrainQuestions returns all pending questions and clears the buffer.
func (t *AskQuestion) DrainQuestions() []PendingQuestion {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := t.questions
	t.questions = nil
	return q
}
