package task

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Task is the single source of truth - input, questions, session state, and result.
type Task struct {
	// Input (set by user)
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Constraints    []string `json:"constraints,omitempty"`
	ScoreThreshold int      `json:"score_threshold,omitempty"`

	// Questions (written by agent, answered by user)
	Questions []Question `json:"questions,omitempty"`

	// Session (managed by agent - tracks continuation state)
	Session *Session `json:"session,omitempty"`

	// Result (written by agent on completion)
	Result *Result `json:"result,omitempty"`
}

// DefaultScoreThreshold is used when no threshold is specified.
const DefaultScoreThreshold = 80

// GetScoreThreshold returns the score threshold, defaulting to 80.
func (t *Task) GetScoreThreshold() int {
	if t.ScoreThreshold > 0 {
		return t.ScoreThreshold
	}
	return DefaultScoreThreshold
}

// Question represents a question the agent asks the user.
type Question struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Context  string   `json:"context,omitempty"`
	Options  []string `json:"options,omitempty"`
	Answer   string   `json:"answer,omitempty"`
}

// Session tracks the agent's progress for continuation.
type Session struct {
	Status      Status     `json:"status"`
	Branch      string     `json:"branch,omitempty"`
	Approach    string     `json:"approach,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	Iterations  int        `json:"iterations"`
	Decisions   []Decision `json:"decisions,omitempty"`
	LastUpdated time.Time  `json:"last_updated"`
}

// Result is the outcome of a completed agent run.
type Result struct {
	Status      Status         `json:"status"`
	Summary     string         `json:"summary,omitempty"`
	Branch      string         `json:"branch,omitempty"`
	Iterations  int            `json:"iterations"`
	Provider    string         `json:"provider,omitempty"`
	Skills      []string       `json:"skills,omitempty"`
	TotalTokens int            `json:"total_tokens"`
	Score       int            `json:"score,omitempty"`
	ScoreDetail map[string]int `json:"score_detail,omitempty"`
	PRURL       string         `json:"pr_url,omitempty"`
	Error       string         `json:"error,omitempty"`
}

// Status represents the task lifecycle state.
type Status string

const (
	StatusNew        Status = "new"
	StatusInProgress Status = "in_progress"
	StatusPaused     Status = "paused_for_questions"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

// Decision records a design decision from a question/answer pair.
type Decision struct {
	QuestionID string `json:"question_id"`
	Question   string `json:"question"`
	Answer     string `json:"answer"`
}

// LoadFromFile reads and parses a task from a JSON file.
func LoadFromFile(path string) (*Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read task file %s: %w", path, err)
	}

	var t Task
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to parse task file %s: %w", path, err)
	}

	if t.Title == "" {
		return nil, fmt.Errorf("task title is required in %s", path)
	}

	if t.Description == "" {
		return nil, fmt.Errorf("task description is required in %s", path)
	}

	return &t, nil
}

// SaveToFile writes the full task state back to the JSON file.
func (t *Task) SaveToFile(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// EnsureSession initializes the session if it doesn't exist.
func (t *Task) EnsureSession() {
	if t.Session == nil {
		t.Session = &Session{Status: StatusNew, LastUpdated: time.Now()}
	}
}

// ToPrompt formats the task as a user message for the AI.
func (t *Task) ToPrompt() string {
	prompt := fmt.Sprintf("## Task: %s\n\n%s", t.Title, t.Description)
	if len(t.Constraints) > 0 {
		prompt += "\n\n## Constraints\n"
		for _, c := range t.Constraints {
			prompt += fmt.Sprintf("- %s\n", c)
		}
	}
	return prompt
}

// PendingQuestions returns questions that have no answer yet.
func (t *Task) PendingQuestions() []Question {
	var pending []Question
	for _, q := range t.Questions {
		if q.Answer == "" {
			pending = append(pending, q)
		}
	}

	return pending
}

// AnsweredQuestions returns questions that have been answered.
func (t *Task) AnsweredQuestions() []Question {
	var answered []Question
	for _, q := range t.Questions {
		if q.Answer != "" {
			answered = append(answered, q)
		}
	}
	return answered
}

// AddQuestion appends a new question to the task.
func (t *Task) AddQuestion(id, question, context string, options []string) {
	t.Questions = append(t.Questions, Question{
		ID:       id,
		Question: question,
		Context:  context,
		Options:  options,
	})
}

// AnswersPrompt formats answered questions as context for the AI.
func (t *Task) AnswersPrompt() string {
	answered := t.AnsweredQuestions()
	if len(answered) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Previous Questions & User Answers\n\n")
	for _, q := range answered {
		b.WriteString(fmt.Sprintf(" ** Q: %s ** \n", q.Question))
		if len(q.Options) > 0 {
			b.WriteString(fmt.Sprintf("Options were: %s\n", strings.Join(q.Options, ", ")))
		}
		b.WriteString(fmt.Sprintf(" ** A: %s ** \n\n", q.Answer))
	}
	return b.String()
}

// ContinuationPrompt generates context for resuming from a prior session.
func (t *Task) ContinuationPrompt() string {
	if t.Session == nil || t.Session.Status == StatusNew {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Session Continuation\n\n")
	b.WriteString(fmt.Sprintf("This task was previously worked on (status: %s, branch: %s).\n\n", t.Session.Status, t.Session.Branch))

	if t.Session.Approach != "" {
		b.WriteString(fmt.Sprintf(" ** Previous approach :** %s\n\n", t.Session.Approach))
	}
	if t.Session.Summary != "" {
		b.WriteString(fmt.Sprintf(" ** Progress so far :** %s\n\n", t.Session.Summary))
	}
	if len(t.Session.Decisions) > 0 {
		b.WriteString(" ** Decisions made :** \n")
		for _, d := range t.Session.Decisions {
			b.WriteString(fmt.Sprintf("- Q: %s > A: %s\n", d.Question, d.Answer))
		}
		b.WriteString("\n")
	}
	b.WriteString(` ** Instructions for continuation :**
- Review the previous approach and the user's answers
- If the answers support the previous approach, continue where it left off
- If the answers suggest a different direction, pivot - but explain why
- Do NOT redo work already done correctly
- Verify prior work still compiles/passes before building on it
`)
	return b.String()
}

// RecordAnswersAsDecisions moves answered questions into session decisions.
func (t *Task) RecordAnswersAsDecisions() {
	t.EnsureSession()
	for _, q := range t.AnsweredQuestions() {
		alreadyRecorded := false
		for _, d := range t.Session.Decisions {
			if d.QuestionID == q.ID {
				alreadyRecorded = true
				break
			}
		}
		if !alreadyRecorded {
			t.Session.Decisions = append(t.Session.Decisions, Decision{
				QuestionID: q.ID,
				Question:   q.Question,
				Answer:     q.Answer,
			})
		}
	}
}
