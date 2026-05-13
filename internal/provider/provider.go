package provider

import (
	"context"

	"agent/internal/message"
	"agent/internal/tool"
)

// Provider defines the interface for AI providers.
type Provider interface {
	// Name returns the provider identifier.
	Name() string

	// SendMessage sends a conversation to the AI and returns a response.
	// The tools parameter provides the available tool definitions.
	SendMessage(ctx context.Context, conv *message.Conversation, tools []tool.Definition) (*message.Response, error)
}
