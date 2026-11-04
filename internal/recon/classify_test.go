package recon_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
	"github.com/hackathon/settlement-agent/internal/recon"
)

func TestDetermineSeverity(t *testing.T) {
	tests := []struct {
		name     string
		variance money.Minor
		expected string
	}{
		{"zero variance", 0, "low"},
		{"under 100 INR positive", 9999, "low"},
		{"under 100 INR negative", -9999, "low"},
		{"exactly 100 INR threshold", 10000, "medium"},
		{"medium variance positive", 500000, "medium"},
		{"medium variance negative", -500000, "medium"},
		{"just under 10000 INR", 999999, "medium"},
		{"exactly 10000 INR threshold", 1000000, "high"},
		{"large variance positive", 5880000, "high"},
		{"large variance negative", -5880000, "high"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := recon.DetermineSeverity(tc.variance)
			if got != tc.expected {
				t.Errorf("DetermineSeverity(%d) = %s; want %s", tc.variance, got, tc.expected)
			}
		})
	}
}

func TestBuildInvestigationContext_BoundedSize(t *testing.T) {
	b := recon.SettlementBatch{
		ID:                  1,
		BatchRef:            "STL-20261012-F",
		SettledOn:           time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		Currency:            "INR",
		ReportedGrossMinor:  25000000,
		ReportedFeeMinor:    600000,
		ReportedRefundMinor: 0,
		ReportedNetMinor:    24400000,
	}

	calc := &recon.BatchCalculation{
		ExpectedGrossMinor:  25000000,
		ExpectedFeeMinor:    500000,
		ExpectedRefundMinor: 0,
		ExpectedNetMinor:    24500000,
		FeeDiff:             100000,
		MethodCounts:        map[string]int{"card": 1},
		MethodTotals:        map[string]money.Minor{"card": 25000000},
	}

	depRes := recon.BatchDepositResult{
		BankTotal:   24400000,
		WindowStart: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		WindowEnd:   time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		LinkedDeposits: []recon.BankDeposit{
			{
				ID:          3,
				BankTxnID:   "TXN-20261013-01",
				AmountMinor: 24400000,
				PostedOn:    time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
				Narration:   "SETTLEMENT STL-20261012-F",
			},
		},
	}

	asOf := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	totalVariance := money.Minor(-100000)

	ctx := recon.BuildInvestigationContext(b, calc, depRes, asOf, totalVariance)

	data, err := json.Marshal(ctx)
	if err != nil {
		t.Fatalf("failed to marshal context: %v", err)
	}

	// Must be bounded <= 8 KB (8192 bytes)
	if len(data) > 8192 {
		t.Errorf("context JSON exceeded 8 KB limit: %d bytes", len(data))
	}

	// Verify required sections and evidence IDs
	if ctx["batch"] == nil || ctx["dates"] == nil || ctx["comparison"] == nil {
		t.Errorf("missing core context keys")
	}
}

func TestEngine_AutoResolveWhenBatchClears(t *testing.T) {
	asOf := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	effectiveTo := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	schedules := []recon.FeeSchedule{
		{ID: 1, Method: "card", RateBps: 200, FixedMinor: 0, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &effectiveTo},
	}

	// Batch is now perfectly matched (bank deposit received)
	in := &recon.ReconInput{
		AsOf:                 asOf,
		SettlementWindowDays: 2,
		FeeSchedules:         schedules,
		Batches: []recon.SettlementBatch{
			{ID: 1, BatchRef: "STL-20261005-A", BatchRefNorm: "STL20261005A", SettledOn: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Currency: "INR", ReportedGrossMinor: 10000000, ReportedFeeMinor: 200000, ReportedRefundMinor: 0, ReportedNetMinor: 9800000},
		},
		Orders: []recon.Order{
			{ID: 1, OrderRefNorm: "ORDA1", AmountMinor: 10000000, Currency: "INR"},
		},
		Payments: []recon.GatewayPayment{
			{ID: 1, OrderRefNorm: "ORDA1", AmountMinor: 10000000, Currency: "INR", Method: "card", Status: "captured", CapturedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC), SettlementRefNorm: "STL20261005A"},
		},
		BankDeposits: []recon.BankDeposit{
			{ID: 1, Narration: "SETTLEMENT STL-20261005-A", AmountMinor: 9800000, Currency: "INR", PostedOn: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)},
		},
	}

	engine := recon.NewEngine()
	out := engine.Run(in)

	if len(out.BatchResults) != 1 || out.BatchResults[0].Status != "matched" {
		t.Fatalf("expected batch to be matched, got: %+v", out.BatchResults)
	}

	if len(out.ExceptionsToUpsert) != 0 {
		t.Fatalf("expected 0 new exceptions, got %d", len(out.ExceptionsToUpsert))
	}

	// Check that all previously open exception types are in ExceptionsToResolve
	if len(out.ExceptionsToResolve) != 5 {
		t.Fatalf("expected 5 exceptions to resolve, got %d", len(out.ExceptionsToResolve))
	}

	expectedTypes := map[string]bool{
		"FEE_VARIANCE":              true,
		"DELAYED_OR_SPLIT_DEPOSIT": true,
		"REFUND_NOT_IN_LEDGER":      true,
		"MISSING_DEPOSIT":           true,
		"UNEXPLAINED_VARIANCE":      true,
	}

	for _, key := range out.ExceptionsToResolve {
		if key.BatchID != 1 {
			t.Errorf("expected BatchID 1, got %d", key.BatchID)
		}
		if !expectedTypes[key.Type] {
			t.Errorf("unexpected resolve type: %s", key.Type)
		}
	}
}

