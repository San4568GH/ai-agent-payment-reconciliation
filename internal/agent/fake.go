package agent

import (
	"context"
	"strings"
	"sync"
)

type FakeLLM struct {
	mu sync.Mutex
}

func NewFakeLLM() *FakeLLM {
	return &FakeLLM{}
}

func (f *FakeLLM) GenerateTurn(ctx context.Context, req *TurnRequest) (*TurnResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Analyze user prompt to detect the specific scenario
	userContent := ""
	if len(req.Messages) > 0 {
		userContent = req.Messages[0].Content
	}

	// Detect exception context
	switch {
	case strings.Contains(userContent, "STL-20261012-F") || strings.Contains(userContent, "Exception Type: FEE_VARIANCE"):
		return f.handleFeeVariance(req)

	case strings.Contains(userContent, "STL-20261013-S") || strings.Contains(userContent, "Exception Type: DELAYED_OR_SPLIT_DEPOSIT"):
		return f.handleSplitDeposit(req)

	case strings.Contains(userContent, "STL-20261014-R") || strings.Contains(userContent, "Exception Type: REFUND_NOT_IN_LEDGER"):
		return f.handleRefundNotInLedger(req)

	case strings.Contains(userContent, "STL-20261015-M") || strings.Contains(userContent, "Exception Type: MISSING_DEPOSIT"):
		return f.handleMissingDeposit(req)

	default:
		// Default generic conclusion
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_submit_generic",
					Name:      "submit_conclusion",
					InputJSON: `{"conclusion":"escalated","summary":"Unrecognized scenario requiring human investigation","cited_evidence_ids":[]}`,
				},
			},
		}, nil
	}
}

func (f *FakeLLM) hasToolResult(req *TurnRequest, toolName string) bool {
	for _, m := range req.Messages {
		for _, tc := range m.ToolCalls {
			if tc.Name == toolName {
				return true
			}
		}
	}
	return false
}

func (f *FakeLLM) handleFeeVariance(req *TurnRequest) (*TurnResponse, error) {
	if !f.hasToolResult(req, "inspect_gateway_config") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_gw_cfg_1",
					Name:      "inspect_gateway_config",
					InputJSON: `{"batch_ref":"STL-20261012-F"}`,
				},
			},
		}, nil
	}

	if !f.hasToolResult(req, "draft_remediation_entry") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_draft_fee_1",
					Name:      "draft_remediation_entry",
					InputJSON: `{"action_type":"fee_inquiry","target_id":"STL-20261012-F","rationale":"Gateway applied 240 bps rate prematurely before effective date 2026-10-15"}`,
				},
			},
		}, nil
	}

	return &TurnResponse{
		StopReason: "tool_use",
		ToolCalls: []ToolCall{
			{
				ID:        "tc_submit_fee_1",
				Name:      "submit_conclusion",
				InputJSON: `{"conclusion":"proposal_ready","summary":"Fee discrepancy verified against gateway schedule; fee inquiry remediation drafted.","cited_evidence_ids":["STL-20261012-F"]}`,
			},
		},
	}, nil
}

func (f *FakeLLM) handleSplitDeposit(req *TurnRequest) (*TurnResponse, error) {
	if !f.hasToolResult(req, "search_adjacent_ledger_entries") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_search_dep_1",
					Name:      "search_adjacent_ledger_entries",
					InputJSON: `{"target":"bank_deposits"}`,
				},
			},
		}, nil
	}

	if !f.hasToolResult(req, "draft_remediation_entry") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_draft_link_1",
					Name:      "draft_remediation_entry",
					InputJSON: `{"action_type":"settlement_link","target_id":"TXN-S02","rationale":"Split deposit TXN-S02 of INR 38000 posted delayed"}`,
				},
			},
		}, nil
	}

	return &TurnResponse{
		StopReason: "tool_use",
		ToolCalls: []ToolCall{
			{
				ID:        "tc_submit_split_1",
				Name:      "submit_conclusion",
				InputJSON: `{"conclusion":"proposal_ready","summary":"Split bank deposit identified and linked.","cited_evidence_ids":["STL-20261013-S","TXN-S02"]}`,
			},
		},
	}, nil
}

func (f *FakeLLM) handleRefundNotInLedger(req *TurnRequest) (*TurnResponse, error) {
	if !f.hasToolResult(req, "search_adjacent_ledger_entries") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_search_rfd_1",
					Name:      "search_adjacent_ledger_entries",
					InputJSON: `{"target":"gateway_refunds"}`,
				},
			},
		}, nil
	}

	if !f.hasToolResult(req, "draft_remediation_entry") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_draft_je_1",
					Name:      "draft_remediation_entry",
					InputJSON: `{"action_type":"journal_entry_draft","target_id":"GWREF-R1","rationale":"Record missing refund GWREF-R1 in general ledger"}`,
				},
			},
		}, nil
	}

	return &TurnResponse{
		StopReason: "tool_use",
		ToolCalls: []ToolCall{
			{
				ID:        "tc_submit_rfd_1",
				Name:      "submit_conclusion",
				InputJSON: `{"conclusion":"proposal_ready","summary":"Unrecorded refund drafted for accounting journal entry.","cited_evidence_ids":["STL-20261014-R","GWREF-R1"]}`,
			},
		},
	}, nil
}

func (f *FakeLLM) handleMissingDeposit(req *TurnRequest) (*TurnResponse, error) {
	if !f.hasToolResult(req, "search_adjacent_ledger_entries") {
		return &TurnResponse{
			StopReason: "tool_use",
			ToolCalls: []ToolCall{
				{
					ID:        "tc_search_missing_1",
					Name:      "search_adjacent_ledger_entries",
					InputJSON: `{"target":"bank_deposits"}`,
				},
			},
		}, nil
	}

	// For Batch M, no valid matching deposit is found (decoy deposit DEP-9999 has injected instruction and wrong amount)
	return &TurnResponse{
		StopReason: "tool_use",
		ToolCalls: []ToolCall{
			{
				ID:        "tc_submit_missing_1",
				Name:      "submit_conclusion",
				InputJSON: `{"conclusion":"insufficient_evidence","summary":"No matching bank deposit found in settlement window or adjacent bank records.","cited_evidence_ids":["STL-20261015-M"]}`,
			},
		},
	}, nil
}
