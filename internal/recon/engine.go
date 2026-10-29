package recon

import (
	"github.com/hackathon/settlement-agent/internal/money"
)

// Engine performs pure deterministic reconciliation without DB or network access.
type Engine struct{}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Run(in *ReconInput) *ReconOutput {
	out := &ReconOutput{
		AsOf:                 in.AsOf,
		BatchResults:         []BatchResult{},
		MatchesToInsert:      []MatchRecord{},
		ExceptionsToUpsert:   []ExceptionRecord{},
		ExceptionsToResolve:  []ExceptionKey{},
		PaymentLinksToUpdate: []PaymentLinkUpdate{},
		RefundLinksToUpdate:  []RefundLinkUpdate{},
	}

	// 1. Link Payments and Refunds
	pUpdates, rUpdates := LinkRecords(in)
	out.PaymentLinksToUpdate = pUpdates
	out.RefundLinksToUpdate = rUpdates

	// Apply link updates to local copies
	payments := make([]GatewayPayment, len(in.Payments))
	copy(payments, in.Payments)
	pLinkMap := make(map[int64]PaymentLinkUpdate)
	for _, u := range pUpdates {
		pLinkMap[u.PaymentID] = u
	}
	for i := range payments {
		if u, ok := pLinkMap[payments[i].ID]; ok {
			payments[i].OrderID = u.OrderID
			payments[i].LinkStatus = u.LinkStatus
			payments[i].LinkReason = u.LinkReason
		}
	}

	refunds := make([]GatewayRefund, len(in.GatewayRefunds))
	copy(refunds, in.GatewayRefunds)
	rLinkMap := make(map[int64]RefundLinkUpdate)
	for _, u := range rUpdates {
		rLinkMap[u.GatewayRefundID] = u
	}
	for i := range refunds {
		if u, ok := rLinkMap[refunds[i].ID]; ok {
			refunds[i].InternalRefundID = u.InternalRefundID
		}
	}

	// 2. Process each batch
	for _, b := range in.Batches {
		calc, _ := CalculateBatchExpected(b, payments, refunds, in.FeeSchedules)
		depRes := LinkBatchDeposits(b, in.BankDeposits, in.ExistingMatches, in.SettlementWindowDays, in.AsOf)

		out.MatchesToInsert = append(out.MatchesToInsert, depRes.Matches...)

		B := depRes.BankTotal
		R := b.ReportedNetMinor
		E := calc.ExpectedNetMinor
		totalVariance := (B - R) + (R - E)

		exceptions := ClassifyBatch(b, calc, depRes, in.AsOf)
		out.ExceptionsToUpsert = append(out.ExceptionsToUpsert, exceptions...)

		// Determine batch status
		batchStatus := "exception"
		if depRes.IsPending {
			batchStatus = "pending"
		} else if len(exceptions) == 0 && B == R && R == E {
			batchStatus = "matched"
		}

		bResult := BatchResult{
			BatchID:             b.ID,
			BatchRef:            b.BatchRef,
			AsOf:                in.AsOf,
			Status:              batchStatus,
			ExpectedGrossMinor:  calc.ExpectedGrossMinor,
			ExpectedFeeMinor:    calc.ExpectedFeeMinor,
			ExpectedRefundMinor: calc.ExpectedRefundMinor,
			ExpectedNetMinor:    calc.ExpectedNetMinor,
			ReportedNetMinor:    b.ReportedNetMinor,
			BankTotalMinor:      depRes.BankTotal,
			VarianceMinor:       totalVariance,
		}
		out.BatchResults = append(out.BatchResults, bResult)

		// Auto-resolve check:
		// If batch is now matched or pending, any previously open exception is cleared
		if batchStatus == "matched" || batchStatus == "pending" {
			for _, typ := range []string{"FEE_VARIANCE", "DELAYED_OR_SPLIT_DEPOSIT", "REFUND_NOT_IN_LEDGER", "MISSING_DEPOSIT", "UNEXPLAINED_VARIANCE"} {
				out.ExceptionsToResolve = append(out.ExceptionsToResolve, ExceptionKey{
					BatchID: b.ID,
					Type:    typ,
				})
			}
		} else {
			// Resolve specific types that are no longer present
			raisedTypes := make(map[string]bool)
			for _, ex := range exceptions {
				raisedTypes[ex.Type] = true
			}
			for _, typ := range []string{"FEE_VARIANCE", "DELAYED_OR_SPLIT_DEPOSIT", "REFUND_NOT_IN_LEDGER", "MISSING_DEPOSIT"} {
				if !raisedTypes[typ] {
					out.ExceptionsToResolve = append(out.ExceptionsToResolve, ExceptionKey{
						BatchID: b.ID,
						Type:    typ,
					})
				}
			}
		}
	}

	return out
}

func (e *Engine) CalculateSummary(results []BatchResult) (volume, expNet, bankRec, netVar money.Minor, total, matched, openEx int) {
	total = len(results)
	for _, r := range results {
		volume += r.ExpectedGrossMinor
		expNet += r.ExpectedNetMinor
		bankRec += r.BankTotalMinor
		netVar += r.VarianceMinor
		if r.Status == "matched" {
			matched++
		} else if r.Status == "exception" {
			openEx++
		}
	}
	return
}

