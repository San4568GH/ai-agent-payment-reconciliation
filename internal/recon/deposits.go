package recon

import (
	"strings"
	"time"

	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/hackathon/settlement-agent/internal/money"
)

type BatchDepositResult struct {
	BatchID      int64
	WindowStart  time.Time
	WindowEnd    time.Time
	BankTotal    money.Minor
	Matches      []MatchRecord
	LinkedDeposits []BankDeposit
	IsPending    bool
}

// LinkBatchDeposits links bank deposits to settlement batches by reference within the candidate window.
func LinkBatchDeposits(batch SettlementBatch, deposits []BankDeposit, existingMatches []ExistingMatch, windowDays int, asOf time.Time) BatchDepositResult {
	// 1. Calculate window
	// exp = settled_on + 1 day
	settledDate := time.Date(batch.SettledOn.Year(), batch.SettledOn.Month(), batch.SettledOn.Day(), 0, 0, 0, 0, time.UTC)
	exp := settledDate.AddDate(0, 0, 1)
	windowEnd := exp.AddDate(0, 0, windowDays)

	res := BatchDepositResult{
		BatchID:     batch.ID,
		WindowStart: exp,
		WindowEnd:   windowEnd,
		Matches:     []MatchRecord{},
	}

	// Track deposits already matched elsewhere
	matchedDepositIDs := make(map[int64]bool)
	for _, m := range existingMatches {
		matchedDepositIDs[m.DepositID] = true
		if m.BatchID == batch.ID {
			res.BankTotal += m.AppliedAmountMinor
		}
	}

	// 2. Link eligible deposits
	for _, d := range deposits {
		if d.Currency != "INR" {
			continue
		}
		if matchedDepositIDs[d.ID] {
			continue
		}

		posted := time.Date(d.PostedOn.Year(), d.PostedOn.Month(), d.PostedOn.Day(), 0, 0, 0, 0, time.UTC)
		if posted.Before(exp) || posted.After(windowEnd) {
			continue
		}

		normRef := ingest.NormalizeRef(d.Reference)
		normNarr := ingest.NormalizeRef(d.Narration)

		if strings.Contains(normRef, batch.BatchRefNorm) || strings.Contains(normNarr, batch.BatchRefNorm) {
			// Found a candidate match!
			match := MatchRecord{
				BatchID:            batch.ID,
				DepositID:          d.ID,
				AppliedAmountMinor: d.AmountMinor,
				Method:             "reference",
			}
			res.Matches = append(res.Matches, match)
			res.LinkedDeposits = append(res.LinkedDeposits, d)
			res.BankTotal += d.AmountMinor
			matchedDepositIDs[d.ID] = true
		}
	}

	// 3. Pending rule (F.4):
	// If as_of <= windowEnd and bank_total < reported_net -> pending (no exception yet)
	asOfDate := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, time.UTC)
	if !asOfDate.After(windowEnd) && res.BankTotal < batch.ReportedNetMinor {
		res.IsPending = true
	}

	return res
}

