package recon_test

import (
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/recon"
)

func TestLinkBatchDeposits_WindowEdges(t *testing.T) {
	// settled_on: 2026-10-10 -> exp: 2026-10-11. windowDays=2 -> windowEnd: 2026-10-13
	batch := recon.SettlementBatch{
		ID:                 1,
		BatchRefNorm:       "STL101",
		SettledOn:          time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		ReportedNetMinor:   10000000,
	}

	deposits := []recon.BankDeposit{
		// exp - 1 (10-10): Excluded
		{ID: 1, Narration: "SETTLEMENT STL101", AmountMinor: 1000000, Currency: "INR", PostedOn: time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)},
		// exp (10-11): Included
		{ID: 2, Narration: "SETTLEMENT STL101", AmountMinor: 2000000, Currency: "INR", PostedOn: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)},
		// exp + 1 (10-12): Included
		{ID: 3, Narration: "SETTLEMENT STL101", AmountMinor: 3000000, Currency: "INR", PostedOn: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)},
		// exp + 2 (10-13 = windowEnd): Included
		{ID: 4, Narration: "SETTLEMENT STL101", AmountMinor: 4000000, Currency: "INR", PostedOn: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)},
		// exp + 3 (10-14 = windowEnd + 1): Excluded
		{ID: 5, Narration: "SETTLEMENT STL101", AmountMinor: 5000000, Currency: "INR", PostedOn: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)},
	}

	asOf := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	res := recon.LinkBatchDeposits(batch, deposits, nil, 2, asOf)

	if len(res.Matches) != 3 {
		t.Fatalf("expected 3 linked deposits (IDs 2, 3, 4), got %d", len(res.Matches))
	}
	// Bank total: 2000000 + 3000000 + 4000000 = 9000000
	if res.BankTotal != 9000000 {
		t.Errorf("expected bank total 9000000, got %d", res.BankTotal)
	}
}

func TestLinkBatchDeposits_BatchS(t *testing.T) {
	// Scenario S: settled 2026-10-13, reported net 98,000 INR (9800000 paise).
	// Window: exp=10-14, windowEnd=10-16.
	// Deposit 1: 60,000 on 10-14 ("PART 1", contains batch ref) -> inside window, links.
	// Deposit 2: 38,000 on 10-19 ("PART 2", no batch ref, outside window) -> does NOT link.
	batchS := recon.SettlementBatch{
		ID:               4,
		BatchRef:         "STL-20261013-S",
		BatchRefNorm:     "STL20261013S",
		SettledOn:        time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		ReportedNetMinor: 9800000,
	}

	deposits := []recon.BankDeposit{
		{
			ID:          10,
			Narration:   "SETTLEMENT STL-20261013-S PART 1",
			AmountMinor: 6000000,
			Currency:    "INR",
			PostedOn:    time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:          11,
			Narration:   "PAYOUT PART 2",
			AmountMinor: 3800000,
			Currency:    "INR",
			PostedOn:    time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC),
		},
	}

	asOf := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	res := recon.LinkBatchDeposits(batchS, deposits, nil, 2, asOf)

	if len(res.Matches) != 1 {
		t.Fatalf("expected 1 match for Batch S, got %d", len(res.Matches))
	}
	if res.Matches[0].DepositID != 10 {
		t.Errorf("expected deposit ID 10 to match, got %d", res.Matches[0].DepositID)
	}
	if res.BankTotal != 6000000 {
		t.Errorf("expected bank total 6000000 paise, got %d", res.BankTotal)
	}
	if res.IsPending {
		t.Errorf("expected IsPending=false on as_of 2026-10-20 (overdue)")
	}
}

func TestLinkBatchDeposits_PendingVsOverdue(t *testing.T) {
	// settled 2026-10-13 -> windowEnd = 10-16.
	batch := recon.SettlementBatch{
		ID:               4,
		BatchRefNorm:     "STL20261013S",
		SettledOn:        time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		ReportedNetMinor: 9800000,
	}

	// as_of on 10-15 (inside window) -> should be pending!
	asOfInside := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	resInside := recon.LinkBatchDeposits(batch, nil, nil, 2, asOfInside)
	if !resInside.IsPending {
		t.Errorf("expected pending=true when as_of is within window")
	}

	// as_of on 10-20 (past windowEnd 10-16) -> overdue!
	asOfOverdue := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	resOverdue := recon.LinkBatchDeposits(batch, nil, nil, 2, asOfOverdue)
	if resOverdue.IsPending {
		t.Errorf("expected pending=false when as_of is overdue")
	}
}
