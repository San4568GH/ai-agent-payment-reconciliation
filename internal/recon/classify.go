package recon

import (
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
)

// DetermineSeverity returns low, medium, or high based on absolute variance in paise.
func DetermineSeverity(varianceMinor money.Minor) string {
	absVal := varianceMinor
	if absVal < 0 {
		absVal = -absVal
	}
	if absVal < 10000 { // < ₹100
		return "low"
	}
	if absVal < 1000000 { // < ₹10,000
		return "medium"
	}
	return "high"
}

// BuildInvestigationContext creates the structured F.6 context JSON dictionary (<= 8 KB)
// with explicit evidence IDs for the LLM agent to cite.
func BuildInvestigationContext(
	b SettlementBatch,
	calc *BatchCalculation,
	depRes BatchDepositResult,
	asOf time.Time,
	totalVariance money.Minor,
) map[string]any {
	ctx := make(map[string]any)

	// Batch identity
	ctx["batch"] = map[string]any{
		"id":          b.ID,
		"batch_ref":   b.BatchRef,
		"settled_on":  b.SettledOn.Format("2006-01-02"),
		"currency":    b.Currency,
		"evidence_id": "ctx:batch",
	}

	// Dates & Window
	ctx["dates"] = map[string]any{
		"as_of":        asOf.Format("2006-01-02"),
		"window_start": depRes.WindowStart.Format("2006-01-02"),
		"window_end":   depRes.WindowEnd.Format("2006-01-02"),
		"evidence_id":  "ctx:dates",
	}

	// Comparison table
	ctx["comparison"] = map[string]any{
		"gross": map[string]any{
			"expected_minor": int64(calc.ExpectedGrossMinor),
			"reported_minor": int64(b.ReportedGrossMinor),
		},
		"fee": map[string]any{
			"expected_minor": int64(calc.ExpectedFeeMinor),
			"reported_minor": int64(b.ReportedFeeMinor),
		},
		"refund": map[string]any{
			"expected_minor": int64(calc.ExpectedRefundMinor),
			"reported_minor": int64(b.ReportedRefundMinor),
		},
		"net": map[string]any{
			"expected_minor": int64(calc.ExpectedNetMinor),
			"reported_minor": int64(b.ReportedNetMinor),
			"bank_minor":     int64(depRes.BankTotal),
		},
		"variance_minor": int64(totalVariance),
		"evidence_id":    "ctx:calc",
	}

	// Payment Methods summary
	methods := make([]map[string]any, 0)
	for m, cnt := range calc.MethodCounts {
		methods = append(methods, map[string]any{
			"method":       m,
			"count":        cnt,
			"amount_minor": int64(calc.MethodTotals[m]),
		})
	}
	ctx["payment_methods"] = methods

	// Unrecorded Refunds
	unrecordedList := make([]map[string]any, 0)
	for _, r := range calc.UnrecordedRefunds {
		unrecordedList = append(unrecordedList, map[string]any{
			"evidence_id":       fmt.Sprintf("gw_refund:%d", r.ID),
			"id":                r.ID,
			"gateway_refund_id": r.GatewayRefundID,
			"refund_ref_norm":   r.RefundRefNorm,
			"amount_minor":      int64(r.AmountMinor),
			"status":            r.Status,
			"refunded_at":       r.RefundedAt.Format(time.RFC3339),
		})
	}
	ctx["unrecorded_refunds"] = unrecordedList

	// Linked Deposits
	linkedList := make([]map[string]any, 0)
	for _, d := range depRes.LinkedDeposits {
		linkedList = append(linkedList, map[string]any{
			"evidence_id":  fmt.Sprintf("deposit:%d", d.ID),
			"id":           d.ID,
			"bank_txn_id":  d.BankTxnID,
			"amount_minor": int64(d.AmountMinor),
			"posted_on":    d.PostedOn.Format("2006-01-02"),
			"narration":    d.Narration,
		})
	}
	ctx["linked_deposits"] = linkedList

	return ctx
}

// ClassifyBatch applies F.5 variance decomposition and returns the list of exceptions.
func ClassifyBatch(
	b SettlementBatch,
	calc *BatchCalculation,
	depRes BatchDepositResult,
	asOf time.Time,
) []ExceptionRecord {
	var exceptions []ExceptionRecord

	// If the batch is pending within its window, no deposit exceptions are raised
	if depRes.IsPending {
		return exceptions
	}

	B := depRes.BankTotal
	R := b.ReportedNetMinor
	E := calc.ExpectedNetMinor

	totalVariance := (B - R) + (R - E)
	contextMap := BuildInvestigationContext(b, calc, depRes, asOf, totalVariance)

	// Layer 1: Deposit vs Reported (B vs R)
	if B == 0 {
		varMin := -R
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "MISSING_DEPOSIT",
			ReasonCode:    "DEPOSIT_NOT_RECEIVED",
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	} else if B > 0 && B < R {
		varMin := B - R
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "DELAYED_OR_SPLIT_DEPOSIT",
			ReasonCode:    "PARTIAL_DEPOSIT_RECEIVED",
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	} else if B > R {
		varMin := B - R
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "UNEXPLAINED_VARIANCE",
			ReasonCode:    "OVER_DEPOSIT",
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	}

	// Layer 2: Reported vs Expected (R vs E)
	if calc.FeeDiff != 0 {
		varMin := -calc.FeeDiff
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "FEE_VARIANCE",
			ReasonCode:    "GATEWAY_FEE_MISMATCH",
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	}

	if calc.UnrecordedRefund != 0 {
		varMin := -calc.UnrecordedRefund
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "REFUND_NOT_IN_LEDGER",
			ReasonCode:    "REFUND_OMITTED_FROM_LEDGER",
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	}

	// Remainder checks
	remainder := (R - E) + calc.FeeDiff + calc.UnrecordedRefund
	if remainder != 0 || calc.ArithmeticError {
		reason := "GROSS_VARIANCE"
		if calc.ArithmeticError {
			reason = "BATCH_ARITHMETIC"
		}
		varMin := remainder
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "UNEXPLAINED_VARIANCE",
			ReasonCode:    reason,
			Severity:      DetermineSeverity(varMin),
			Status:        "open",
			VarianceMinor: varMin,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	}

	if calc.NoFeeSchedule {
		exceptions = append(exceptions, ExceptionRecord{
			BatchID:       b.ID,
			Type:          "UNEXPLAINED_VARIANCE",
			ReasonCode:    "NO_FEE_SCHEDULE",
			Severity:      "high",
			Status:        "open",
			VarianceMinor: 0,
			Context:       contextMap,
			LastAsOf:      asOf,
		})
	}

	return exceptions
}
