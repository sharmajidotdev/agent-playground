package claude

import "encoding/json"

// apiRequest represents the Claude Messages API request body.
type apiRequest struct {
	Model     string       `json:"model"`
	MaxTokens int          `json:"max_tokens"`
	System    string       `json:"system,omitempty"`
	Tools     []apiTool    `json:"tools,omitempty"`
	Messages  []apiMessage `json:"messages"`
}

// apiTool represents a tool definition in the Claude API.
type apiTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// apiMessage represents a message in the Claude API.
type apiMessage struct {
	Role    string       `json:"role"`
	Content []apiContent `json:"content"`
}

// apiContent represents a content block in the Claude API.
type apiContent struct {
	Type string `json:"type"`

	// For text blocks
	Text string `json:"text,omitempty"`

	// For tool_use blocks
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// For tool_result blocks
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// apiResponse represents the Claude Messages API response.
type apiResponse struct {
	ID         string       `json:"id"`
	Type       string       `json:"type"`
	Role       string       `json:"role"`
	Content    []apiContent `json:"content"`
	Model      string       `json:"model"`
	StopReason string       `json:"stop_reason"`
	Usage      apiUsage     `json:"usage"`
}

// apiUsage represents token usage in the response.
type apiUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}
