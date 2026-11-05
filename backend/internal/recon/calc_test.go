package recon_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/recon"
)

func TestCalculateBatchExpected_BatchF(t *testing.T) {
	effectiveTo := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	schedules := []recon.FeeSchedule{
		{Method: "card", RateBps: 200, FixedMinor: 0, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &effectiveTo},
		{Method: "card", RateBps: 240, FixedMinor: 0, EffectiveFrom: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), EffectiveTo: nil},
	}

	batchF := recon.SettlementBatch{
		ID:                  3,
		BatchRef:            "STL-20261012-F",
		BatchRefNorm:        "STL20261012F",
		SettledOn:           time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC),
		ReportedGrossMinor:  25000000,
		ReportedFeeMinor:    600000, // Gateway applied 240 bps early -> 6,000.00
		ReportedRefundMinor: 0,
		ReportedNetMinor:    24400000,
	}

	// 5 card payments of 50,000 each, captured on 2026-10-10 at 200 bps
	var payments []recon.GatewayPayment
	orderID := int64(1)
	capturedAt := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	for i := 1; i <= 5; i++ {
		payments = append(payments, recon.GatewayPayment{
			ID:                int64(i),
			OrderID:           &orderID,
			LinkStatus:        "linked",
			AmountMinor:       5000000,
			Method:            "card",
			Status:            "captured",
			CapturedAt:        capturedAt,
			SettlementRefNorm: "STL20261012F",
		})
	}

	calc, err := recon.CalculateBatchExpected(batchF, payments, nil, schedules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected gross: 25,000,000 paise (2,50,000 INR)
	if calc.ExpectedGrossMinor != 25000000 {
		t.Errorf("expected gross 25000000, got %d", calc.ExpectedGrossMinor)
	}

	// Expected fee: 200 bps on 25,000,000 = 500,000 paise (5,000 INR)
	if calc.ExpectedFeeMinor != 500000 {
		t.Errorf("expected fee 500000, got %d", calc.ExpectedFeeMinor)
	}

	// Expected net: 24,500,000 paise (2,45,000 INR)
	if calc.ExpectedNetMinor != 24500000 {
		t.Errorf("expected net 24500000, got %d", calc.ExpectedNetMinor)
	}

	// Fee difference: reported (600000) - expected (500000) = 100000 (1,000 INR overcharge)
	if calc.FeeDiff != 100000 {
		t.Errorf("expected fee diff 100000, got %d", calc.FeeDiff)
	}
}

func TestCalculateBatchExpected_CaptureDateBoundary(t *testing.T) {
	effectiveTo := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	schedules := []recon.FeeSchedule{
		{Method: "card", RateBps: 200, FixedMinor: 0, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), EffectiveTo: &effectiveTo},
		{Method: "card", RateBps: 240, FixedMinor: 0, EffectiveFrom: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), EffectiveTo: nil},
	}

	// Capture on 10-14 -> 200 bps
	sched1, err := recon.FindScheduleRow(schedules, "card", time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("find schedule row 10-14: %v", err)
	}
	if sched1.RateBps != 200 {
		t.Errorf("expected 200 bps on 10-14, got %d", sched1.RateBps)
	}

	// Capture on 10-15 -> 240 bps
	sched2, err := recon.FindScheduleRow(schedules, "card", time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("find schedule row 10-15: %v", err)
	}
	if sched2.RateBps != 240 {
		t.Errorf("expected 240 bps on 10-15, got %d", sched2.RateBps)
	}
}

func TestCalculateBatchExpected_MissingSchedule(t *testing.T) {
	schedules := []recon.FeeSchedule{
		{Method: "card", RateBps: 200, FixedMinor: 0, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	batch := recon.SettlementBatch{
		BatchRefNorm: "BATCH1",
	}

	payments := []recon.GatewayPayment{
		{
			AmountMinor:       100000,
			Method:            "upi", // UPI has no schedule!
			Status:            "captured",
			LinkStatus:        "linked",
			CapturedAt:        time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
			SettlementRefNorm: "BATCH1",
		},
	}

	calc, err := recon.CalculateBatchExpected(batch, payments, nil, schedules)
	if err == nil {
		t.Fatalf("expected error for missing schedule, got nil")
	}
	if !strings.Contains(err.Error(), "NO_FEE_SCHEDULE") {
		t.Errorf("expected NO_FEE_SCHEDULE error, got %v", err)
	}
	if !calc.NoFeeSchedule {
		t.Errorf("expected calc.NoFeeSchedule to be true")
	}
}

func TestCalculateBatchExpected_ArithmeticError(t *testing.T) {
	schedules := []recon.FeeSchedule{
		{Method: "card", RateBps: 200, FixedMinor: 0, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	// Bad batch row: reported net doesn't equal gross - fee - refund
	badBatch := recon.SettlementBatch{
		BatchRefNorm:        "BADBATCH",
		ReportedGrossMinor:  10000000,
		ReportedFeeMinor:    200000,
		ReportedRefundMinor: 0,
		ReportedNetMinor:    9000000, // Should be 9800000!
	}

	calc, err := recon.CalculateBatchExpected(badBatch, nil, nil, schedules)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !calc.ArithmeticError {
		t.Errorf("expected ArithmeticError=true for corrupted reported net")
	}
}
