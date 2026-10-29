package agent

import "context"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	InputJSON string `json:"input_json"`
}

type ToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type Message struct {
	Role        Role         `json:"role"`
	Content     string       `json:"content,omitempty"`
	ToolCalls   []ToolCall   `json:"tool_calls,omitempty"`
	ToolResults []ToolResult `json:"tool_results,omitempty"`
}

type TurnRequest struct {
	SystemPrompt string       `json:"system_prompt"`
	Messages     []Message    `json:"messages"`
	Tools        []ToolSchema `json:"tools"`
}

type TurnResponse struct {
	StopReason string     `json:"stop_reason"` // "tool_use", "end_turn", "max_tokens"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls"`
}

// LLM is the pluggable language model interface.
type LLM interface {
	GenerateTurn(ctx context.Context, req *TurnRequest) (*TurnResponse, error)
}

