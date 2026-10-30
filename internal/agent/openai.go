package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAIClient implements LLM for any OpenAI-compatible Chat Completions API.
// This natively supports OpenAI (GPT-4o), Google Gemini (via OpenAI compatibility endpoint),
// Groq, OpenRouter, DeepSeek, Together AI, and local runners like Ollama, vLLM, and LiteLLM.
type OpenAIClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func NewOpenAIClient(apiKey, baseURL, model string) (*OpenAIClient, error) {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
	}

	if baseURL == "" {
		baseURL = os.Getenv("OPENAI_BASE_URL")
	}

	if model == "" {
		model = os.Getenv("LLM_MODEL")
	}

	// Auto-configure Gemini OpenAI endpoint if Gemini key/model is detected and baseURL not set
	if baseURL == "" {
		if strings.HasPrefix(strings.ToLower(model), "gemini") || os.Getenv("GEMINI_API_KEY") != "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if model == "" {
		if strings.Contains(baseURL, "googleapis.com") {
			model = "gemini-1.5-pro"
		} else {
			model = "gpt-4o"
		}
	}

	return &OpenAIClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

func (c *OpenAIClient) formatMessages(sysPrompt string, msgs []Message) []map[string]any {
	var formatted []map[string]any

	if sysPrompt != "" {
		formatted = append(formatted, map[string]any{
			"role":    "system",
			"content": sysPrompt,
		})
	}

	for _, m := range msgs {
		switch m.Role {
		case RoleUser:
			if len(m.ToolResults) > 0 {
				for _, tr := range m.ToolResults {
					formatted = append(formatted, map[string]any{
						"role":         "tool",
						"tool_call_id": tr.ToolUseID,
						"content":      tr.Content,
					})
				}
			}
			if m.Content != "" {
				formatted = append(formatted, map[string]any{
					"role":    "user",
					"content": m.Content,
				})
			}

		case RoleAssistant:
			asst := map[string]any{
				"role": "assistant",
			}
			if m.Content != "" {
				asst["content"] = m.Content
			}
			if len(m.ToolCalls) > 0 {
				var tcList []map[string]any
				for _, tc := range m.ToolCalls {
					tcList = append(tcList, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": tc.InputJSON,
						},
					})
				}
				asst["tool_calls"] = tcList
			}
			formatted = append(formatted, asst)
		}
	}

	return formatted
}

func (c *OpenAIClient) formatTools(tools []ToolSchema) []map[string]any {
	var formatted []map[string]any
	for _, t := range tools {
		formatted = append(formatted, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
	}
	return formatted
}

func (c *OpenAIClient) GenerateTurn(ctx context.Context, req *TurnRequest) (*TurnResponse, error) {
	reqBody := map[string]any{
		"model":    c.model,
		"messages": c.formatMessages(req.SystemPrompt, req.Messages),
	}

	if len(req.Tools) > 0 {
		reqBody["tools"] = c.formatTools(req.Tools)
		reqBody["tool_choice"] = "auto"
	}

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", c.baseURL)

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

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payloadBytes))
		if err != nil {
			return nil, fmt.Errorf("build openai request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("openai HTTP error: %w", err)
			continue
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read openai response body: %w", readErr)
			continue
		}

		// Handle transient status codes
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("openai transient error (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("openai API error (HTTP %d): %s", resp.StatusCode, string(respBody))
		}

		var openAIResp struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Role      string `json:"role"`
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
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}

		if err := json.Unmarshal(respBody, &openAIResp); err != nil {
			return nil, fmt.Errorf("unmarshal openai response: %w", err)
		}

		if openAIResp.Error != nil {
			return nil, fmt.Errorf("openai returned error: %s (%s)", openAIResp.Error.Message, openAIResp.Error.Type)
		}

		if len(openAIResp.Choices) == 0 {
			return nil, fmt.Errorf("openai returned no choices")
		}

		choice := openAIResp.Choices[0]
		turnRes := &TurnResponse{
			Content: choice.Message.Content,
		}

		if len(choice.Message.ToolCalls) > 0 {
			turnRes.StopReason = "tool_use"
			for _, tc := range choice.Message.ToolCalls {
				turnRes.ToolCalls = append(turnRes.ToolCalls, ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					InputJSON: tc.Function.Arguments,
				})
			}
		} else {
			turnRes.StopReason = "end_turn"
		}

		return turnRes, nil
	}

	return nil, fmt.Errorf("openai request failed after retries: %w", lastErr)
}

