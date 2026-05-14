package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"agent/internal/message"
	"agent/internal/tool"
)

// Provider implements the AI provider interface for Ollama.
type Provider struct {
	baseURL string
	model   string
	client  *http.Client
}

// New creates a new Ollama provider.
func New(baseURL, model string) *Provider {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Provider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string { return "ollama" }

// SendMessage sends a conversation to Ollama and returns the response.
func (p *Provider) SendMessage(ctx context.Context, conv *message.Conversation, tools []tool.Definition) (*message.Response, error) {
	reqBody := p.buildRequest(conv, tools)

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/api/chat"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(body))
	}

	return p.parseResponse(body)

}

func (p *Provider) buildRequest(conv *message.Conversation, tools []tool.Definition) map[string]interface{} {
	messages := []map[string]interface{}{}

	// System message
	if conv.SystemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": conv.SystemPrompt,
		})
	}
	// Conversation messages
	for _, msg := range conv.Messages {
		if msg.Role == message.RoleUser && len(msg.Blocks) > 0 {
			// Tool results
			for _, block := range msg.Blocks {
				if block.Type == message.BlockToolResult {
					messages = append(messages, map[string]interface{}{
						"role":      "tool",
						"content":   block.Content,
						"tool_name": block.ToolName,
					})
				}
			}
		} else if msg.Role == message.RoleAssistant && len(msg.Blocks) > 0 {
			m := map[string]interface{}{
				"role": "assistant",
			}
			var toolCalls []map[string]interface{}
			var textContent string
			for _, block := range msg.Blocks {
				switch block.Type {
				case message.BlockText:
					textContent += block.Text
				case message.BlockToolUse:
					var args interface{}
					json.Unmarshal(block.Input, &args)
					toolCalls = append(toolCalls, map[string]interface{}{
						"function": map[string]interface{}{
							"name":      block.ToolName,
							"arguments": args,
						},
					})
				}
			}
			m["content"] = textContent
			if len(toolCalls) > 0 {
				m["tool_calls"] = toolCalls
			}
			messages = append(messages, m)
		} else {
			messages = append(messages, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}

	// Build tools in Ollama format
	var ollamaTools []map[string]interface{}
	for _, t := range tools {
		var params interface{}
		json.Unmarshal(t.InputSchema, &params)
		ollamaTools = append(ollamaTools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  params,
			}})
	}

	req := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
		"stream":   false,
	}
	if len(ollamaTools) > 0 {
		req["tools"] = ollamaTools
	}
	return req
}

func (p *Provider) parseResponse(body []byte) (*message.Response, error) {
	var resp struct {
		Message struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			ToolCalls []struct {
				Function struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	result := &message.Response{
		Content: resp.Message.Content,
	}

	if len(resp.Message.ToolCalls) > 0 {
		result.StopReason = message.StopToolUse
		for i, tc := range resp.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, message.ToolCall{
				ID:    fmt.Sprintf("ollama_%d", i),
				Name:  tc.Function.Name,
				Input: tc.Function.Arguments,
			})
		}
	} else {
		result.StopReason = message.StopEndTurn
	}
	return result, nil
}
