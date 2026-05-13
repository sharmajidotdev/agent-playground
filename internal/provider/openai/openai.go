package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"agent/internal/message"
	"agent/internal/tool"
)

const apiURL = "https://api.openai.com/v1/chat/completions"

// Provider implements the AI provider interface for OpenAI's GPT.
type Provider struct {
	apiKey string
	model  string
	client *http.Client
}

// New creates a new OpenAI provider.
func New(apiKey, model string) *Provider {
	return &Provider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}
func (p *Provider) Name() string { return "openai" }

// SendMessage sends a conversation to OpenAI and returns the response.
func (p *Provider) SendMessage(ctx context.Context, conv *message.Conversation, tools []tool.Definition) (*message.Response, error) {
	reqBody := p.buildRequest(conv, tools)

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)

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
		return nil, fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, string(body))
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
						"role":         "tool",
						"tool_call_id": block.ToolUseID,
						"content":      block.Content,
					})
				}
			}
		} else if msg.Role == message.RoleAssistant && len(msg.Blocks) > 0 {
			// Assistant with tool calls
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
					toolCalls = append(toolCalls, map[string]interface{}{
						"id":   block.ToolUseID,
						"type": "function",
						"function": map[string]interface{}{
							"name":      block.ToolName,
							"arguments": string(block.Input),
						},
					})
				}
			}
			if textContent != "" {
				m["content"] = textContent
			}
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

	// Build tools
	var oaiTools []map[string]interface{}
	for _, t := range tools {
		oaiTools = append(oaiTools, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  json.RawMessage(t.InputSchema),
			},
		})
	}

	req := map[string]interface{}{
		"model":    p.model,
		"messages": messages,
	}
	if len(oaiTools) > 0 {
		req["tools"] = oaiTools
	}

	return req
}

func (p *Provider) parseResponse(body []byte) (*message.Response, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	result := &message.Response{
		Content: choice.Message.Content,
		Usage: message.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	switch choice.FinishReason {
	case "tool_calls":
		result.StopReason = message.StopToolUse
	case "stop":
		result.StopReason = message.StopEndTurn
	case "length":
		result.StopReason = message.StopMaxTokens
	default:
		result.StopReason = message.StopEndTurn
	}

	for _, tc := range choice.Message.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, message.ToolCall{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: []byte(tc.Function.Arguments),
		})
	}

	return result, nil
}
