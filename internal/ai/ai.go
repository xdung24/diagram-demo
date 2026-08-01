package ai

import (
	"context"
	"os"
	"strings"
)

// AIClient holds the provider configuration.
type AIClient struct {
	Provider     string // "openai" | "gemini" | "github"
	APIKey       string
	Model        string
	SystemPrompt string
	LogFunc      func(string)
}

// IsConfigured reports whether the AI provider is usable.
func (cfg AIClient) IsConfigured() bool {
	return cfg.APIKey != "" && cfg.Model != "" && cfg.Provider != ""
}

// NewClient creates a new AIClient with configuration from environment variables.
func NewClient() *AIClient {
	return NewClientWithLogger(nil)
}

// NewClientWithLogger creates an AI client and accepts an optional logger
// function that will receive LLM request/response messages for streaming.
func NewClientWithLogger(logFn func(string)) *AIClient {
	client := AIClient{
		Provider:     strings.ToLower(os.Getenv("AI_PROVIDER")),
		APIKey:       strings.TrimSpace(os.Getenv("AI_API_KEY")),
		Model:        strings.TrimSpace(os.Getenv("AI_MODEL")),
		SystemPrompt: "You generate only valid Mermaid diagram syntax and return just the diagram body.",
		LogFunc:      logFn,
	}
	if !client.IsConfigured() {
		panic("AI provider, API key, and model must be configured")
	}

	return &client
}

func (c *AIClient) GenerateDiagramCode(ctx context.Context, prompt string) (string, error) {
	history := []ChatMessage{{Role: "system", Content: c.SystemPrompt}}
	response, err := c.GetAIReply(history, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(response), nil
}
