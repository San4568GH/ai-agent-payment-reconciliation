package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
func RunSmokeTest(ctx context.Context, apiKey, model, mode string) (*SmokeTestResult, error) {
	if mode == "fake" || apiKey == "" {
		return &SmokeTestResult{
			Success:  true,
			Provider: "fake_llm",
			Model:    model,
			ToolUsed: "get_weather",
			ToolArgs: map[string]string{"city": "Delhi"},
			Message:  "Smoke test passed in fake mode (simulated tool_use block returned)",
		}, nil
	}

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

	req.Header.Set("x-api-key", apiKey)
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
		Success:  false,
		Provider: "anthropic",
		Model:    model,
		Message:  "Anthropic returned 200 OK but did not output a tool_use block",
	}, fmt.Errorf("no tool_use block found in response")
}
