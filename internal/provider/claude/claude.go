package claude

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

const apiURL = "https://api.anthropic.com/v1/messages"

// Provider implements the AI provider interface for Anthropic's Claude.
type Provider struct {
	apiKey string
	model  string
	client *http.Client
}

// New creates a new Claude provider.
func New(apiKey, model string) *Provider {
	return &Provider{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
	}
}
func (p *Provider) Name() string { return "claude" }

// SendMessage sends a conversation to Claude and returns the response.
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
	req.Header.Set("anthropic-version", "2023-06-01")

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
		return nil, fmt.Errorf("claude API error (status %d): %s", resp.StatusCode, string(body))
	}
	return p.parseResponse(body)
}

// buildRequest constructs the Claude API request body.
func (p *Provider) buildRequest(conv *message.Conversation, tools []tool.Definition) apiRequest {
	req := apiRequest{
		Model:     p.model,
		MaxTokens: 8192,
		System:    conv.SystemPrompt,
	}

	// Convert tools to Claude format
	for _, t := range tools {
		req.Tools = append(req.Tools, apiTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	// Convert messages
	for _, msg := range conv.Messages {
		apiMsg := apiMessage{
			Role: string(msg.Role)}

		if len(msg.Blocks) > 0 {
			for _, block := range msg.Blocks {
				switch block.Type {
				case message.BlockText:
					apiMsg.Content = append(apiMsg.Content, apiContent{
						Type: "text",
						Text: block.Text,
					})
				case message.BlockToolUse:
					apiMsg.Content = append(apiMsg.Content, apiContent{
						Type:  "tool_use",
						ID:    block.ToolUseID,
						Name:  block.ToolName,
						Input: block.Input,
					})

				case message.BlockToolResult:
					apiMsg.Content = append(apiMsg.Content, apiContent{
						Type:      "tool_result",
						ToolUseID: block.ToolUseID,
						Text:      block.Content,
						IsError:   block.IsError,
					})
				}
			}
		} else if msg.Content != "" {
			apiMsg.Content = append(apiMsg.Content, apiContent{
				Type: "text",
				Text: msg.Content,
			})
		}
		req.Messages = append(req.Messages, apiMsg)
	}
	return req
}

// parseResponse parses the Claude API response into our internal format.
func (p *Provider) parseResponse(body []byte) (*message.Response, error) {
	var resp apiResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	result := &message.Response{
		Usage: message.Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		}}

	// Map stop reason
	switch resp.StopReason {
	case "end_turn":
		result.StopReason = message.StopEndTurn
	case "tool_use":
		result.StopReason = message.StopToolUse
	case "max_tokens":
		result.StopReason = message.StopMaxTokens
	default:
		result.StopReason = message.StopEndTurn
	}

	// Extract content and tool calls
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if result.Content != "" {
				result.Content += "\n"
			}
			result.Content += block.Text
		case "tool_use":
			result.ToolCalls = append(result.ToolCalls, message.ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}

	return result, nil
}
