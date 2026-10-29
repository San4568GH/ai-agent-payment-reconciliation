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

type AnthropicClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewAnthropicClient(apiKey string, model string) (*AnthropicClient, error) {
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if model == "" {
		model = os.Getenv("LLM_MODEL")
	}
	if model == "" {
		return nil, fmt.Errorf("LLM_MODEL environment variable or configuration is required")
	}

	return &AnthropicClient{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// Format message content for Anthropic Messages API
func (c *AnthropicClient) formatMessages(msgs []Message) []map[string]any {
	var formatted []map[string]any

	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			if len(m.ToolResults) > 0 {
				var contentBlocks []map[string]any
				for _, tr := range m.ToolResults {
					block := map[string]any{
						"type":        "tool_result",
						"tool_use_id": tr.ToolUseID,
						"content":     tr.Content,
					}
					if tr.IsError {
						block["is_error"] = true
					}
					contentBlocks = append(contentBlocks, block)
				}
				if m.Content != "" {
					contentBlocks = append(contentBlocks, map[string]any{
						"type": "text",
						"text": m.Content,
					})
				}
				formatted = append(formatted, map[string]any{
					"role":    "user",
					"content": contentBlocks,
				})
			} else {
				formatted = append(formatted, map[string]any{
					"role":    "user",
					"content": m.Content,
				})
			}

		case RoleAssistant:
			var contentBlocks []map[string]any
			if m.Content != "" {
				contentBlocks = append(contentBlocks, map[string]any{
					"type": "text",
					"text": m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var inputMap map[string]any
				_ = json.Unmarshal([]byte(tc.InputJSON), &inputMap)
				contentBlocks = append(contentBlocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": inputMap,
				})
			}
			formatted = append(formatted, map[string]any{
				"role":    "assistant",
				"content": contentBlocks,
			})
		}
	}

	return formatted
}

func (c *AnthropicClient) GenerateTurn(ctx context.Context, req *TurnRequest) (*TurnResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not configured")
	}

	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": 1024,
		"system":     req.SystemPrompt,
		"messages":   c.formatMessages(req.Messages),
		"tools":      req.Tools,
	}

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	// Retry loop for rate-limits (429) and server errors (5xx) with 1s and 3s backoffs
	backoffs := []time.Duration{1 * time.Second, 3 * time.Second}
	var lastErr error

	for attempt := 0; attempt <= len(backoffs); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoffs[attempt-1]):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, fmt.Errorf("build anthropic request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("anthropic HTTP error: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read anthropic response body: %w", readErr)
			continue
		}

		// Handle transient status codes
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("anthropic transient error (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("anthropic API error (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		// Parse successful response
		var anthropicResp struct {
			StopReason string `json:"stop_reason"`
			Content    []struct {
				Type  string         `json:"type"`
				Text  string         `json:"text,omitempty"`
				ID    string         `json:"id,omitempty"`
				Name  string         `json:"name,omitempty"`
				Input map[string]any `json:"input,omitempty"`
			} `json:"content"`
		}

		if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
			return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
		}

		turnRes := &TurnResponse{
			StopReason: anthropicResp.StopReason,
		}

		for _, item := range anthropicResp.Content {
			if item.Type == "text" {
				turnRes.Content += item.Text
			} else if item.Type == "tool_use" {
				inputBytes, _ := json.Marshal(item.Input)
				turnRes.ToolCalls = append(turnRes.ToolCalls, ToolCall{
					ID:        item.ID,
					Name:      item.Name,
					InputJSON: string(inputBytes),
				})
			}
		}

		return turnRes, nil
	}

	return nil, fmt.Errorf("anthropic request failed after retries: %w", lastErr)
}

