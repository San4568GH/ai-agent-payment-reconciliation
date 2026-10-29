package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/hackathon/settlement-agent/internal/remediation"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ToolRunner struct {
	pool        *pgxpool.Pool
	gwMock      *mocks.GatewayMock
	acctMock    *mocks.AccountingMock
	remService  *remediation.Service
	registry    *EvidenceRegistry
	exceptionID int64
	runID       int64
	draftDone   bool
}

func NewToolRunner(
	pool *pgxpool.Pool,
	gwMock *mocks.GatewayMock,
	acctMock *mocks.AccountingMock,
	remService *remediation.Service,
	registry *EvidenceRegistry,
	exceptionID int64,
	runID int64,
) *ToolRunner {
	return &ToolRunner{
		pool:        pool,
		gwMock:      gwMock,
		acctMock:    acctMock,
		remService:  remService,
		registry:    registry,
		exceptionID: exceptionID,
		runID:       runID,
	}
}

func (tr *ToolRunner) HasDraftSucceeded() bool {
	return tr.draftDone
}

// ExecuteTool dispatches a tool call by name with strict evidence registry validation.
func (tr *ToolRunner) ExecuteTool(ctx context.Context, name string, argsJSON string) (any, error) {
	switch name {
	case "inspect_gateway_config":
		var args struct {
			BatchRef      string `json:"batch_ref"`
			CheckDisputes bool   `json:"check_disputes"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("invalid args for inspect_gateway_config: %w", err)
		}
		if args.BatchRef == "" {
			return nil, fmt.Errorf("batch_ref is required")
		}

		tr.registry.Register(args.BatchRef)
		res := tr.gwMock.InspectGatewayConfig(args.BatchRef)
		return res, nil

	case "search_adjacent_ledger_entries":
		var args struct {
			Target   string `json:"target"`
			Query    string `json:"query"`
			DateFrom string `json:"date_from"`
			DateTo   string `json:"date_to"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("invalid args for search_adjacent_ledger_entries: %w", err)
		}

		return tr.searchLedger(ctx, args.Target, args.Query, args.DateFrom, args.DateTo)

	case "draft_remediation_entry":
		var args struct {
			ActionType string `json:"action_type"`
			TargetID   string `json:"target_id"`
			Rationale  string `json:"rationale"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("invalid args for draft_remediation_entry: %w", err)
		}

		// Evidence validation: target_id must be in the registry!
		if !tr.registry.Contains(args.TargetID) {
			return nil, fmt.Errorf("target_id %q is not found in run evidence registry", args.TargetID)
		}

		evidenceIDs := tr.registry.All()
		rem, err := tr.remService.DraftRemediation(ctx, tr.exceptionID, tr.runID, args.ActionType, args.TargetID, args.Rationale, evidenceIDs)
		if err != nil {
			return nil, fmt.Errorf("draft remediation failed: %w", err)
		}

		tr.draftDone = true
		tr.registry.Register(fmt.Sprintf("REM-%d", rem.ID))

		return map[string]any{
			"remediation_id":  rem.ID,
			"action_type":     rem.ActionType,
			"status":          rem.Status,
			"idempotency_key": rem.IdempotencyKey,
			"message":         "Remediation drafted successfully for operator review",
		}, nil

	case "submit_conclusion":
		var args struct {
			Conclusion       string   `json:"conclusion"`
			Summary          string   `json:"summary"`
			CitedEvidenceIDs []string `json:"cited_evidence_ids"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return nil, fmt.Errorf("invalid args for submit_conclusion: %w", err)
		}

		// Evidence validation: check all cited IDs
		if err := tr.registry.ValidateAll(args.CitedEvidenceIDs); err != nil {
			return nil, fmt.Errorf("evidence validation failed: %w", err)
		}

		// Loop Rule: proposal_ready requires a successful draft, otherwise downgrade to escalated
		finalConclusion := args.Conclusion
		if finalConclusion == "proposal_ready" && !tr.draftDone {
			finalConclusion = "escalated"
		}

		return map[string]any{
			"terminal":           true,
			"conclusion":         finalConclusion,
			"summary":            args.Summary,
			"cited_evidence_ids": args.CitedEvidenceIDs,
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool name: %s", name)
	}
}

func (tr *ToolRunner) searchLedger(ctx context.Context, target string, query string, dateFrom string, dateTo string) (any, error) {
	switch target {
	case "bank_deposits":
		rows, err := tr.pool.Query(ctx, `
			SELECT bank_txn_id, reference, narration, amount_minor, posted_on
			FROM bank_deposits
			ORDER BY posted_on DESC
			LIMIT 50
		`)
		if err != nil {
			return nil, fmt.Errorf("query bank_deposits: %w", err)
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var txnID, narration string
			var ref *string
			var amtMinor int64
			var posted time.Time
			if err := rows.Scan(&txnID, &ref, &narration, &amtMinor, &posted); err != nil {
				return nil, err
			}

			// Register valid deposit bank_txn_id in registry
			tr.registry.Register(txnID)
			if ref != nil && *ref != "" {
				tr.registry.Register(*ref)
			}

			results = append(results, map[string]any{
				"bank_txn_id":  txnID,
				"reference":    ref,
				"narration":    narration,
				"amount_minor": amtMinor,
				"posted_on":    posted.Format("2006-01-02"),
			})
		}
		return results, nil

	case "gateway_refunds":
		rows, err := tr.pool.Query(ctx, `
			SELECT gateway_refund_id, gateway_payment_id, amount_minor, refunded_at, settlement_ref_norm
			FROM gateway_refunds
			ORDER BY refunded_at DESC
			LIMIT 50
		`)
		if err != nil {
			return nil, fmt.Errorf("query gateway_refunds: %w", err)
		}
		defer rows.Close()

		var results []map[string]any
		for rows.Next() {
			var rfdID, setRef string
			var payID *string
			var amtMinor int64
			var rfdAt time.Time
			if err := rows.Scan(&rfdID, &payID, &amtMinor, &rfdAt, &setRef); err != nil {
				return nil, err
			}

			// Register gateway refund ID in registry
			tr.registry.Register(rfdID)

			results = append(results, map[string]any{
				"gateway_refund_id":   rfdID,
				"gateway_payment_id":  payID,
				"amount_minor":        amtMinor,
				"refunded_at":         rfdAt.Format(time.RFC3339),
				"settlement_ref_norm": setRef,
			})
		}
		return results, nil

	default:
		return nil, fmt.Errorf("unsupported target: %s", target)
	}
}

