package agent

// ToolSchema represents a tool specification for the Anthropic Messages API.
type ToolSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// GetAgentTools returns the 4 tools supported by the investigation agent.
func GetAgentTools() []ToolSchema {
	return []ToolSchema{
		{
			Name: "inspect_gateway_config",
			Description: "Inspect gateway configuration, rate tables, fee schedules, or dispute history for a given batch reference. " +
				"Use to verify if fee variance was caused by premature tier changes or gateway billing misconfigurations.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"batch_ref": map[string]any{
						"type":        "string",
						"description": "The settlement batch reference, e.g. STL-20261012-F",
					},
					"check_disputes": map[string]any{
						"type":        "boolean",
						"description": "Whether to list active gateway disputes",
					},
				},
				"required": []string{"batch_ref"},
			},
		},
		{
			Name: "search_adjacent_ledger_entries",
			Description: "Search bank deposits or internal ledger records for unlinked entries matching a date window or query string. " +
				"Use to find split/delayed bank deposits or unrecorded refunds.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{
						"type":        "string",
						"enum":        []string{"bank_deposits", "orders", "gateway_refunds"},
						"description": "Entity collection to search",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "Optional search term, reference substring, or narration filter",
					},
					"date_from": map[string]any{
						"type":        "string",
						"description": "Start date in YYYY-MM-DD format",
					},
					"date_to": map[string]any{
						"type":        "string",
						"description": "End date in YYYY-MM-DD format",
					},
				},
				"required": []string{"target"},
			},
		},
		{
			Name: "draft_remediation_entry",
			Description: "Draft a remediation action for operator approval. Policy strictly mandates: " +
				"FEE_VARIANCE -> fee_inquiry; " +
				"DELAYED_OR_SPLIT_DEPOSIT -> settlement_link; " +
				"REFUND_NOT_IN_LEDGER -> journal_entry_draft; " +
				"MISSING_DEPOSIT -> settlement_link only (if deposit found outside window). " +
				"Note: All amounts are derived by Go from the referenced IDs; the LLM never supplies financial amounts.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action_type": map[string]any{
						"type":        "string",
						"enum":        []string{"fee_inquiry", "settlement_link", "journal_entry_draft"},
						"description": "Remediation action matching exception policy",
					},
					"target_id": map[string]any{
						"type":        "string",
						"description": "Target entity ID: bank transaction ID (e.g. DEP-202), gateway refund ID (e.g. RFD-004), or batch ref",
					},
					"rationale": map[string]any{
						"type":        "string",
						"description": "Technical justification for operator review",
					},
				},
				"required": []string{"action_type", "target_id", "rationale"},
			},
		},
		{
			Name: "submit_conclusion",
			Description: "Submit the terminal conclusion of the investigation and finish the run. " +
				"Must be called after a draft succeeds, when evidence is insufficient, or when max turns are reached.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"conclusion": map[string]any{
						"type":        "string",
						"enum":        []string{"proposal_ready", "insufficient_evidence", "escalated"},
						"description": "Final investigation state",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "Concise summary of findings and rationale",
					},
					"cited_evidence_ids": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
						"description": "All evidence IDs cited in the investigation from the run registry",
					},
				},
				"required": []string{"conclusion", "summary", "cited_evidence_ids"},
			},
		},
	}
}

