package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIClient_GenerateTurn_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		resp := map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"role":    "assistant",
						"content": "Let me inspect the gateway contract.",
						"tool_calls": []map[string]any{
							{
								"id":   "call_001",
								"type": "function",
								"function": map[string]any{
									"name":      "inspect_gateway_config",
									"arguments": `{"batch_ref":"STL-20261012-F"}`,
								},
							},
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewOpenAIClient("test-key", server.URL, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := &TurnRequest{
		SystemPrompt: "You are a settlement assistant.",
		Messages: []Message{
			{Role: RoleUser, Content: "Investigate batch STL-20261012-F"},
		},
		Tools: []ToolSchema{
			{
				Name:        "inspect_gateway_config",
				Description: "Inspect config",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	turnRes, err := client.GenerateTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateTurn failed: %v", err)
	}

	if turnRes.StopReason != "tool_use" {
		t.Errorf("expected stop reason 'tool_use', got '%s'", turnRes.StopReason)
	}
	if len(turnRes.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(turnRes.ToolCalls))
	}
	if turnRes.ToolCalls[0].Name != "inspect_gateway_config" {
		t.Errorf("expected tool inspect_gateway_config, got %s", turnRes.ToolCalls[0].Name)
	}
}

func TestOpenAIClient_GenerateTurn_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": "Reconciliation complete.",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewOpenAIClient("test-key", server.URL, "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := &TurnRequest{
		Messages: []Message{
			{Role: RoleUser, Content: "Status check"},
		},
	}

	turnRes, err := client.GenerateTurn(context.Background(), req)
	if err != nil {
		t.Fatalf("GenerateTurn failed: %v", err)
	}

	if turnRes.StopReason != "end_turn" {
		t.Errorf("expected stop reason 'end_turn', got '%s'", turnRes.StopReason)
	}
	if turnRes.Content != "Reconciliation complete." {
		t.Errorf("expected content 'Reconciliation complete.', got '%s'", turnRes.Content)
	}
}
