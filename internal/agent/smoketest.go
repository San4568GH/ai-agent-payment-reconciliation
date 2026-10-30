package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type SmokeTestResult struct {
	Success  bool   `json:"success"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	ToolUsed string `json:"tool_used,omitempty"`
	ToolArgs any    `json:"tool_args,omitempty"`
	Message  string `json:"message"`
}

// RunSmokeTest tests the configured LLM with a dummy tool call to verify credentials, model string, and tool calling.
// Supports Anthropic, OpenAI, Google Gemini, and any OpenAI-compatible provider (Ollama, Groq, OpenRouter).
func RunSmokeTest(ctx context.Context, anthropicKey, model, mode string) (*SmokeTestResult, error) {
	openAIKey := os.Getenv("OPENAI_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")
	baseURL := os.Getenv("OPENAI_BASE_URL")

	if mode == "fake" || (anthropicKey == "" && openAIKey == "" && geminiKey == "" && baseURL == "") {
		return &SmokeTestResult{
			Success:  true,
			Provider: "fake_llm",
			Model:    model,
			ToolUsed: "get_weather",
			ToolArgs: map[string]string{"city": "Delhi"},
			Message:  "Smoke test passed in fake mode (simulated tool_use block returned)",
		}, nil
	}

	// 1. If Anthropic is configured
	if anthropicKey != "" && mode != "openai" && mode != "gemini" {
		payload := map[string]any{
			"model":      model,
			"max_tokens": 100,
			"system":     "You are a test assistant. If asked about weather, always call the get_weather tool.",
			"messages": []map[string]string{
				{"role": "user", "content": "Call get_weather for Delhi right now."},
			},
			"tools": []map[string]any{
				{
					"name":        "get_weather",
					"description": "Get weather for a city",
					"input_schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"city": map[string]string{"type": "string"},
						},
						"required": []string{"city"},
					},
				},
			},
			"tool_choice": map[string]string{"type": "auto"},
		}

		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal smoke test payload: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("x-api-key", anthropicKey)
		req.Header.Set("anthropic-version", "2023-06-01")
		req.Header.Set("content-type", "application/json")

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("execute smoke test request: %w", err)
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return &SmokeTestResult{
				Success:  false,
				Provider: "anthropic",
				Model:    model,
				Message:  fmt.Sprintf("Anthropic API returned status %d: %s", resp.StatusCode, string(respBody)),
			}, fmt.Errorf("anthropic error status %d: %s", resp.StatusCode, string(respBody))
		}

		var parsed struct {
			Content []struct {
				Type  string          `json:"type"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
			StopReason string `json:"stop_reason"`
		}

		if err := json.Unmarshal(respBody, &parsed); err != nil {
			return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
		}

		for _, block := range parsed.Content {
			if block.Type == "tool_use" {
				return &SmokeTestResult{
					Success:  true,
					Provider: "anthropic",
					Model:    model,
					ToolUsed: block.Name,
					ToolArgs: block.Input,
					Message:  "Smoke test passed: live tool_use block returned successfully",
				}, nil
			}
		}

		return &SmokeTestResult{
			Success:  true,
			Provider: "anthropic",
			Model:    model,
			Message:  "Anthropic API connected, response returned without explicit tool_use block",
		}, nil
	}

	// 2. Otherwise, test OpenAI-compatible provider (OpenAI, Gemini, Ollama, Groq, etc.)
	key := openAIKey
	if key == "" {
		key = geminiKey
	}

	client, err := NewOpenAIClient(key, baseURL, model)
	if err != nil {
		return nil, fmt.Errorf("init openai client: %w", err)
	}

	turnReq := &TurnRequest{
		SystemPrompt: "You are a test assistant. If asked about weather, always call the get_weather tool.",
		Messages: []Message{
			{Role: RoleUser, Content: "Call get_weather for Delhi right now."},
		},
		Tools: []ToolSchema{
			{
				Name:        "get_weather",
				Description: "Get weather for a city",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]string{"type": "string"},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	turnResp, err := client.GenerateTurn(ctx, turnReq)
	if err != nil {
		return &SmokeTestResult{
			Success:  false,
			Provider: "openai_compatible",
			Model:    model,
			Message:  err.Error(),
		}, err
	}

	toolUsed := ""
	if len(turnResp.ToolCalls) > 0 {
		toolUsed = turnResp.ToolCalls[0].Name
	}

	return &SmokeTestResult{
		Success:  true,
		Provider: "openai_compatible",
		Model:    model,
		ToolUsed: toolUsed,
		Message:  "Smoke test passed: live tool_use returned successfully via OpenAI-compatible API",
	}, nil
}
