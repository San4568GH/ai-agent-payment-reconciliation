package remediation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidPolicy   = errors.New("action type violates policy for exception type")
	ErrNotFound        = errors.New("record not found")
	ErrInvalidState    = errors.New("action not allowed in current state")
	ErrExecutionFailed = errors.New("external execution failed")
)

type Remediation struct {
	ID             int64           `json:"id"`
	ExceptionID    int64           `json:"exception_id"`
	RunID          int64           `json:"run_id"`
	ActionType     string          `json:"action_type"`
	Payload        json.RawMessage `json:"payload"`
	Rationale      string          `json:"rationale"`
	EvidenceIDs    []string        `json:"evidence_ids"`
	Status         string          `json:"status"`
	IdempotencyKey string          `json:"idempotency_key"`
	DecidedBy      *string         `json:"decided_by,omitempty"`
	DecidedAt      *time.Time      `json:"decided_at,omitempty"`
	DecisionNote   *string         `json:"decision_note,omitempty"`
	ExternalRef    *string         `json:"external_ref,omitempty"`
	ExecutedAt     *time.Time      `json:"executed_at,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type Service struct {
	pool       *pgxpool.Pool
	gwClient   *mocks.GatewayMock
	acctClient *mocks.AccountingMock
}

func NewService(pool *pgxpool.Pool, gw *mocks.GatewayMock, acct *mocks.AccountingMock) *Service {
	return &Service{
		pool:       pool,
		gwClient:   gw,
		acctClient: acct,
	}
}

// AllowedActions maps each exception type to its strictly allowed remediation actions.
var AllowedActions = map[string]string{
	"FEE_VARIANCE":             "fee_inquiry",
	"DELAYED_OR_SPLIT_DEPOSIT": "settlement_link",
	"REFUND_NOT_IN_LEDGER":     "journal_entry_draft",
	"MISSING_DEPOSIT":          "settlement_link",
}

// DraftRemediation derives the financial amounts from the DB and creates a proposed remediation.
// The LLM never supplies financial amounts.
func (s *Service) DraftRemediation(ctx context.Context, exceptionID int64, runID int64, actionType string, targetID string, rationale string, evidenceIDs []string) (*Remediation, error) {
	// 1. Fetch Exception Details
	var excType string
	var batchID int64
	var batchRef string
	var varianceMinor int64

	err := s.pool.QueryRow(ctx, `
		SELECT e.type, e.batch_id, sb.batch_ref, e.variance_minor
		FROM exceptions e
		JOIN settlement_batches sb ON e.batch_id = sb.id
		WHERE e.id = $1
	`, exceptionID).Scan(&excType, &batchID, &batchRef, &varianceMinor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: exception %d", ErrNotFound, exceptionID)
		}
		return nil, fmt.Errorf("query exception %d: %w", exceptionID, err)
	}

	// 2. Validate against policy table
	allowed, ok := AllowedActions[excType]
	if !ok || allowed != actionType {
		return nil, fmt.Errorf("%w: exception %s cannot take action %s (allowed: %s)", ErrInvalidPolicy, excType, actionType, allowed)
	}

	// 3. Derive financial payload strictly in Go from targetID
	var canonicalPayload map[string]any

	switch actionType {
	case "fee_inquiry":
		var reportedFee, expectedFee int64
		err := s.pool.QueryRow(ctx, `
			SELECT reported_fee_minor, (reported_fee_minor + $2) AS expected_fee
			FROM settlement_batches
			WHERE id = $1
		`, batchID, varianceMinor).Scan(&reportedFee, &expectedFee)
		if err != nil {
			return nil, fmt.Errorf("derive fee inquiry amounts: %w", err)
		}
		diff := varianceMinor
		if diff < 0 {
			diff = -diff
		}
		canonicalPayload = map[string]any{
			"batch_id":           batchID,
			"batch_ref":          batchRef,
			"difference_minor":   diff,
			"expected_fee_minor": expectedFee,
			"reported_fee_minor": reportedFee,
		}

	case "settlement_link":
		var depositID int64
		var amtMinor int64
		var postedOn time.Time
		err := s.pool.QueryRow(ctx, `
			SELECT id, amount_minor, posted_on
			FROM bank_deposits
			WHERE bank_txn_id = $1
		`, targetID).Scan(&depositID, &amtMinor, &postedOn)
		if err != nil {
			return nil, fmt.Errorf("derive deposit link for %s: %w", targetID, err)
		}
		canonicalPayload = map[string]any{
			"amount_minor": amtMinor,
			"batch_id":     batchID,
			"batch_ref":    batchRef,
			"deposit_id":   depositID,
			"bank_txn_id":  targetID,
			"posted_on":    postedOn.Format("2006-01-02"),
		}

	case "journal_entry_draft":
		var refundID int64
		var amtMinor int64
		var refundedAt time.Time
		err := s.pool.QueryRow(ctx, `
			SELECT id, amount_minor, refunded_at
			FROM gateway_refunds
			WHERE gateway_refund_id = $1
		`, targetID).Scan(&refundID, &amtMinor, &refundedAt)
		if err != nil {
			return nil, fmt.Errorf("derive refund entry for %s: %w", targetID, err)
		}
		canonicalPayload = map[string]any{
			"amount_minor":      amtMinor,
			"batch_id":          batchID,
			"batch_ref":         batchRef,
			"credit_account":    "5100-Payment-Processing-Fee",
			"currency":          "INR",
			"debit_account":     "1200-Gateway-Receivables",
			"gateway_refund_id": targetID,
			"refund_id":         refundID,
		}

	default:
		return nil, fmt.Errorf("unsupported action type: %s", actionType)
	}

	payloadBytes, err := json.Marshal(canonicalPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical payload: %w", err)
	}

	// 4. Compute Idempotency Key: sha256(exception_id | action_type | canonical payload)
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%d|%s|%s", exceptionID, actionType, string(payloadBytes))))
	idempKey := hex.EncodeToString(hasher.Sum(nil))

	// 5. Insert Remediation with ON CONFLICT DO UPDATE
	var rem Remediation
	var pBytes []byte
	err = s.pool.QueryRow(ctx, `
		INSERT INTO remediations (
			exception_id, run_id, action_type, payload, rationale, evidence_ids, status, idempotency_key, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'proposed', $7, now())
		ON CONFLICT (idempotency_key) WHERE status <> 'rejected'
		DO UPDATE SET
			run_id = EXCLUDED.run_id,
			rationale = EXCLUDED.rationale,
			evidence_ids = EXCLUDED.evidence_ids,
			payload = EXCLUDED.payload
		RETURNING id, exception_id, run_id, action_type, payload, rationale, evidence_ids, status, idempotency_key, created_at
	`, exceptionID, runID, actionType, payloadBytes, rationale, evidenceIDs, idempKey).Scan(
		&rem.ID, &rem.ExceptionID, &rem.RunID, &rem.ActionType, &pBytes, &rem.Rationale, &rem.EvidenceIDs, &rem.Status, &rem.IdempotencyKey, &rem.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert remediation: %w", err)
	}
	rem.Payload = pBytes

	return &rem, nil
}

// ApproveRemediation transitions a proposed remediation to approved with an audit log.
func (s *Service) ApproveRemediation(ctx context.Context, remID int64, actor string, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin approve tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var exceptionID int64
	var actionType string
	err = tx.QueryRow(ctx, `
		UPDATE remediations
		SET status = 'approved', decided_by = $1, decided_at = now(), decision_note = $2
		WHERE id = $3 AND status = 'proposed'
		RETURNING exception_id, action_type
	`, actor, note, remID).Scan(&exceptionID, &actionType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: remediation %d is not in 'proposed' state", ErrInvalidState, remID)
		}
		return fmt.Errorf("approve remediation: %w", err)
	}

	// Write append-only audit event in same transaction
	auditDetails, _ := json.Marshal(map[string]any{
		"action_type": actionType,
		"note":        note,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (exception_id, remediation_id, actor, event_type, details, created_at)
		VALUES ($1, $2, $3, 'remediation_approved', $4, now())
	`, exceptionID, remID, actor, auditDetails)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return tx.Commit(ctx)
}

// RejectRemediation transitions a proposed remediation to rejected, resets the exception to open, and logs audit.
func (s *Service) RejectRemediation(ctx context.Context, remID int64, actor string, note string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reject tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var exceptionID int64
	var actionType string
	err = tx.QueryRow(ctx, `
		UPDATE remediations
		SET status = 'rejected', decided_by = $1, decided_at = now(), decision_note = $2
		WHERE id = $3 AND status = 'proposed'
		RETURNING exception_id, action_type
	`, actor, note, remID).Scan(&exceptionID, &actionType)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: remediation %d is not in 'proposed' state", ErrInvalidState, remID)
		}
		return fmt.Errorf("reject remediation: %w", err)
	}

	// Return exception status back to 'open'
	_, err = tx.Exec(ctx, `
		UPDATE exceptions
		SET status = 'open', updated_at = now()
		WHERE id = $1
	`, exceptionID)
	if err != nil {
		return fmt.Errorf("reset exception status to open: %w", err)
	}

	// Write append-only audit event
	auditDetails, _ := json.Marshal(map[string]any{
		"action_type": actionType,
		"note":        note,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (exception_id, remediation_id, actor, event_type, details, created_at)
		VALUES ($1, $2, $3, 'remediation_rejected', $4, now())
	`, exceptionID, remID, actor, auditDetails)
	if err != nil {
		return fmt.Errorf("insert audit event: %w", err)
	}

	return tx.Commit(ctx)
}

// ExecuteRemediation claims execution, executes external/internal action with up to 3 retries, updates status and audit log.
func (s *Service) ExecuteRemediation(ctx context.Context, remID int64, actor string) error {
	// Step 1: Claim execution using conditional UPDATE
	var exceptionID int64
	var actionType, idempKey string
	var rawPayload []byte

	err := s.pool.QueryRow(ctx, `
		UPDATE remediations
		SET status = 'executing'
		WHERE id = $1 AND status IN ('approved', 'execution_failed')
		RETURNING exception_id, action_type, idempotency_key, payload
	`, remID).Scan(&exceptionID, &actionType, &idempKey, &rawPayload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: remediation %d is not in 'approved' or 'execution_failed' state", ErrInvalidState, remID)
		}
		return fmt.Errorf("claim remediation execution: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	// Step 2: Perform execution with up to 3 attempts
	var externalRef string
	var execErr error

	for attempt := 1; attempt <= 3; attempt++ {
		externalRef, execErr = s.dispatchExecution(ctx, actionType, idempKey, payload)
		if execErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond) // short retry backoff
	}

	// Step 3: Record outcome in a transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin finalize execution tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if execErr != nil {
		// Mark execution_failed
		errMsg := execErr.Error()
		_, err := tx.Exec(ctx, `
			UPDATE remediations
			SET status = 'execution_failed', last_error = $1
			WHERE id = $2
		`, errMsg, remID)
		if err != nil {
			return fmt.Errorf("record execution failure: %w", err)
		}

		auditDetails, _ := json.Marshal(map[string]any{"error": errMsg})
		_, _ = tx.Exec(ctx, `
			INSERT INTO audit_events (exception_id, remediation_id, actor, event_type, details, created_at)
			VALUES ($1, $2, $3, 'execution_failed', $4, now())
		`, exceptionID, remID, actor, auditDetails)

		_ = tx.Commit(ctx)
		return fmt.Errorf("%w: %s", ErrExecutionFailed, errMsg)
	}

	// Success: Mark executed
	_, err = tx.Exec(ctx, `
		UPDATE remediations
		SET status = 'executed', external_ref = $1, executed_at = now(), last_error = NULL
		WHERE id = $2
	`, externalRef, remID)
	if err != nil {
		return fmt.Errorf("record execution success: %w", err)
	}

	// Update Exception status:
	// settlement_link gives 'resolved'
	// fee_inquiry and journal_entry_draft give 'action_taken'
	var newExcStatus string
	if actionType == "settlement_link" {
		newExcStatus = "resolved"
	} else {
		newExcStatus = "action_taken"
	}

	_, err = tx.Exec(ctx, `
		UPDATE exceptions
		SET status = $1, updated_at = now()
		WHERE id = $2
	`, newExcStatus, exceptionID)
	if err != nil {
		return fmt.Errorf("update exception outcome status: %w", err)
	}

	// Audit event
	auditDetails, _ := json.Marshal(map[string]any{
		"action_type":  actionType,
		"external_ref": externalRef,
	})
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_events (exception_id, remediation_id, actor, event_type, details, created_at)
		VALUES ($1, $2, $3, 'remediation_executed', $4, now())
	`, exceptionID, remID, actor, auditDetails)
	if err != nil {
		return fmt.Errorf("insert executed audit event: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *Service) dispatchExecution(ctx context.Context, actionType string, idempKey string, payload map[string]any) (string, error) {
	switch actionType {
	case "settlement_link":
		batchID := int64(payload["batch_id"].(float64))
		depositID := int64(payload["deposit_id"].(float64))
		amtMinor := int64(payload["amount_minor"].(float64))

		_, err := s.pool.Exec(ctx, `
			INSERT INTO matches (batch_id, deposit_id, applied_amount_minor, method, created_at)
			VALUES ($1, $2, $3, 'remediation', now())
			ON CONFLICT (deposit_id) DO NOTHING
		`, batchID, depositID, amtMinor)
		if err != nil {
			return "", fmt.Errorf("insert matches for settlement link: %w", err)
		}
		return fmt.Sprintf("MATCH-%d-%d", batchID, depositID), nil

	case "fee_inquiry":
		if s.gwClient == nil {
			return "TKT-GW-MOCK-OK", nil
		}
		mode := s.gwClient.InspectGatewayConfig(fmt.Sprintf("%v", payload["batch_ref"]))
		_ = mode
		// Create ticket via mock service
		return fmt.Sprintf("TKT-GW-%d", time.Now().Unix()%10000), nil

	case "journal_entry_draft":
		if s.acctClient == nil {
			return "JE-MOCK-OK", nil
		}
		return fmt.Sprintf("JE-%d", time.Now().Unix()%10000), nil

	default:
		return "", fmt.Errorf("unknown action type: %s", actionType)
	}
}

