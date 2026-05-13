package message

import "encoding/json"

// Role represents a message role in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation.
type Message struct {
	Role    Role    `json:"role"`
	Content string  `json:"content,omitempty"`
	Blocks  []Block `json:"blocks,omitempty"`
}

// Block represents a content block within a message (text, tool_use, tool_result).
type Block struct {
	Type      BlockType       `json:"type"`
	Text      string          `json:"text,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// BlockType identifies the type of content block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

// ToolCall represents a tool invocation request from the AI.
type ToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// Response represents a parsed response from an AI provider.
type Response struct {
	// Content is the text content of the response (if any).
	Content string

	// ToolCalls contains any tool invocations requested by the AI.
	ToolCalls []ToolCall

	// StopReason indicates why the AI stopped generating.
	StopReason StopReason

	// Usage tracks token consumption.
	Usage Usage
}

// StopReason indicates why generation stopped.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopError     StopReason = "error"
)

// HasToolCalls returns true if the response contains tool calls.
func (r *Response) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// Usage tracks token consumption for a response.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Conversation holds the full message history for an agent session.
type Conversation struct {
	SystemPrompt string
	Messages     []Message
}

// NewConversation creates a new conversation with a system prompt.
func NewConversation(systemPrompt string) *Conversation {
	return &Conversation{
		SystemPrompt: systemPrompt,
	}
}

// AddUserMessage appends a user message.
func (c *Conversation) AddUserMessage(content string) {
	c.Messages = append(c.Messages, Message{
		Role:    RoleUser,
		Content: content,
	})
}

// AddAssistantMessage appends an assistant message with optional blocks.
func (c *Conversation) AddAssistantMessage(content string, blocks []Block) {
	c.Messages = append(c.Messages, Message{
		Role:    RoleAssistant,
		Content: content,
		Blocks:  blocks,
	})
}

// AddToolResults appends tool results as a user message with tool_result blocks.
func (c *Conversation) AddToolResults(results []ToolResult) {
	var blocks []Block
	for _, r := range results {
		blocks = append(blocks, Block{
			Type:      BlockToolResult,
			ToolUseID: r.ToolUseID,
			Content:   r.Content,
			IsError:   r.IsError,
		})
	}

	c.Messages = append(c.Messages, Message{
		Role:   RoleUser,
		Blocks: blocks,
	})
}
