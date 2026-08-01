package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var aiHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ChatMessage is a single message in the conversation history.
type ChatMessage struct {
	Role    string // "user" | "assistant" | "system"
	Content string
}

// GetAIReply sends the conversation history to the configured AI provider and returns the reply.
// Returns empty string + error on failure.
func (c *AIClient) GetAIReply(history []ChatMessage, userMessage string) (string, error) {
	log.Printf("[ai] GetAIReply: provider=%q model=%q apiKeySet=%v historyLen=%d",
		c.Provider, c.Model, c.APIKey != "", len(history))

	switch strings.ToLower(c.Provider) {
	case "gemini":
		return geminiReply(*c, history, userMessage)
	case "github":
		return githubModelsReply(*c, history, userMessage)
	case "openai":
		return openAIReply(*c, history, userMessage)
	default:
		return openAIReply(*c, history, userMessage)
	}
}

// -------------------------------------------------------------------
// GitHub Models  (OpenAI-compatible, no subprocess required)
// Docs: https://docs.github.com/en/github-models
// Endpoint: https://models.inference.ai.azure.com/chat/completions
// Auth: fine-grained PAT with Models: Read permission
// -------------------------------------------------------------------

func githubModelsReply(cfg AIClient, history []ChatMessage, userMessage string) (string, error) {
	msgs := []openAIMessage{{Role: "system", Content: cfg.SystemPrompt}}
	for _, h := range history {
		msgs = append(msgs, openAIMessage{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, openAIMessage{Role: "user", Content: userMessage})

	body, _ := json.Marshal(openAIRequest{Model: cfg.Model, Messages: msgs})
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM -> GitHubModels: request prepared")
	}
	req, _ := http.NewRequest("POST", "https://models.inference.ai.azure.com/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result openAIResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		if cfg.LogFunc != nil {
			cfg.LogFunc("LLM -> GitHubModels: decode error: " + err.Error())
		}
		return "", err
	}
	if result.Error != nil {
		if cfg.LogFunc != nil {
			cfg.LogFunc("LLM -> GitHubModels: api error: " + result.Error.Message)
		}
		return "", fmt.Errorf("GitHub Models: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("GitHub Models returned no choices")
	}
	reply := strings.TrimSpace(result.Choices[0].Message.Content)
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM <- GitHubModels: reply: " + reply)
	}
	return reply, nil
}

// -------------------------------------------------------------------
// OpenAI
// -------------------------------------------------------------------

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func openAIReply(cfg AIClient, history []ChatMessage, userMessage string) (string, error) {
	msgs := []openAIMessage{{Role: "system", Content: cfg.SystemPrompt}}

	for _, h := range history {
		msgs = append(msgs, openAIMessage{Role: h.Role, Content: h.Content})
	}

	msgs = append(msgs, openAIMessage{Role: "user", Content: userMessage})

	body, _ := json.Marshal(openAIRequest{Model: cfg.Model, Messages: msgs})
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM -> OpenAI: request prepared")
	}
	req, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result openAIResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		if cfg.LogFunc != nil {
			cfg.LogFunc("LLM -> OpenAI: decode error: " + err.Error())
		}
		return "", err
	}

	if result.Error != nil {
		return "", fmt.Errorf("OpenAI: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned no choices")
	}

	reply := strings.TrimSpace(result.Choices[0].Message.Content)
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM <- OpenAI: reply: " + reply)
	}
	return reply, nil
}

// -------------------------------------------------------------------
// Google Gemini
// -------------------------------------------------------------------

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiRequest struct {
	SystemInstruction geminiContent   `json:"system_instruction"`
	Contents          []geminiContent `json:"contents"`
}

type geminiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func geminiReply(cfg AIClient, history []ChatMessage, userMessage string) (string, error) {
	contents := make([]geminiContent, 0, len(history)+1)

	for _, h := range history {
		role := "user"
		if h.Role == "assistant" {
			role = "model"
		}

		contents = append(contents, geminiContent{Role: role, Parts: []geminiPart{{Text: h.Content}}})
	}

	contents = append(contents, geminiContent{Role: "user", Parts: []geminiPart{{Text: userMessage}}})

	reqBody := geminiRequest{
		SystemInstruction: geminiContent{Role: "user", Parts: []geminiPart{{Text: cfg.SystemPrompt}}},
		Contents:          contents,
	}

	body, _ := json.Marshal(reqBody)
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM -> Gemini: request prepared")
	}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", cfg.Model, cfg.APIKey)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var result geminiResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		if cfg.LogFunc != nil {
			cfg.LogFunc("LLM -> Gemini: decode error: " + err.Error())
		}
		return "", err
	}

	if result.Error != nil {
		return "", fmt.Errorf("Gemini: %s", result.Error.Message)
	}

	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned empty response")
	}

	reply := strings.TrimSpace(result.Candidates[0].Content.Parts[0].Text)
	if cfg.LogFunc != nil {
		cfg.LogFunc("LLM <- Gemini: reply: " + reply)
	}
	return reply, nil
}
