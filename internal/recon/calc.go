package recon

import (
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
)

var kolkataTz *time.Location

func init() {
	var err error
	kolkataTz, err = time.LoadLocation("Asia/Kolkata")
	if err != nil {
		kolkataTz = time.FixedZone("IST", 5*3600+30*60)
	}
}

type BatchCalculation struct {
	BatchID             int64
	BatchRef            string
	ExpectedGrossMinor  money.Minor
	ExpectedFeeMinor    money.Minor
	ExpectedRefundMinor money.Minor
	ExpectedNetMinor    money.Minor
	UnrecordedRefund    money.Minor
	FeeDiff             money.Minor // reported_fee - expected_fee
	NetVariance         money.Minor // reported_net - expected_net
	MethodCounts        map[string]int
	MethodTotals        map[string]money.Minor
	UnrecordedRefunds   []GatewayRefund
	ArithmeticError     bool
	NoFeeSchedule       bool
}

// FindScheduleRow finds the active fee schedule row for a payment method and capture date.
func FindScheduleRow(schedules []FeeSchedule, method string, capturedAt time.Time) (*FeeSchedule, error) {
	// Truncate to date in Asia/Kolkata
	inIST := capturedAt.In(kolkataTz)
	captureDate := time.Date(inIST.Year(), inIST.Month(), inIST.Day(), 0, 0, 0, 0, time.UTC)

	for _, s := range schedules {
		if s.Method != method {
			continue
		}
		from := time.Date(s.EffectiveFrom.Year(), s.EffectiveFrom.Month(), s.EffectiveFrom.Day(), 0, 0, 0, 0, time.UTC)
		if captureDate.Before(from) {
			continue
		}
		if s.EffectiveTo != nil {
			to := time.Date(s.EffectiveTo.Year(), s.EffectiveTo.Month(), s.EffectiveTo.Day(), 0, 0, 0, 0, time.UTC)
			if captureDate.After(to) {
				continue
			}
		}
		return &s, nil
	}

	return nil, fmt.Errorf("no fee schedule found for method %s on date %s", method, captureDate.Format("2006-01-02"))
}

// CalculateBatchExpected computes expected figures and verifies arithmetic invariants for one batch.
func CalculateBatchExpected(batch SettlementBatch, payments []GatewayPayment, refunds []GatewayRefund, schedules []FeeSchedule) (*BatchCalculation, error) {
	calc := &BatchCalculation{
		BatchID:           batch.ID,
		BatchRef:          batch.BatchRef,
		MethodCounts:      make(map[string]int),
		MethodTotals:      make(map[string]money.Minor),
		UnrecordedRefunds: []GatewayRefund{},
	}

	// 1. Process Payments
	for _, p := range payments {
		if p.SettlementRefNorm != batch.BatchRefNorm {
			continue
		}
		if p.Status != "captured" || p.LinkStatus != "linked" {
			continue
		}

		sched, err := FindScheduleRow(schedules, p.Method, p.CapturedAt)
		if err != nil {
			calc.NoFeeSchedule = true
			return calc, fmt.Errorf("NO_FEE_SCHEDULE: %w", err)
		}

		pFee := money.FeeFor(p.AmountMinor, sched.RateBps, sched.FixedMinor)
		calc.ExpectedGrossMinor += p.AmountMinor
		calc.ExpectedFeeMinor += pFee

		calc.MethodCounts[p.Method]++
		calc.MethodTotals[p.Method] += p.AmountMinor
	}

	// 2. Process Refunds
	var sumProcessedGatewayRefunds money.Minor
	for _, r := range refunds {
		if r.SettlementRefNorm != batch.BatchRefNorm {
			continue
		}
		if r.Status != "processed" {
			continue
		}

		sumProcessedGatewayRefunds += r.AmountMinor

		if r.InternalRefundID != nil {
			calc.ExpectedRefundMinor += r.AmountMinor
		} else {
			calc.UnrecordedRefund += r.AmountMinor
			calc.UnrecordedRefunds = append(calc.UnrecordedRefunds, r)
		}
	}

	// 3. Expected Net
	calc.ExpectedNetMinor = calc.ExpectedGrossMinor - calc.ExpectedFeeMinor - calc.ExpectedRefundMinor
	calc.FeeDiff = batch.ReportedFeeMinor - calc.ExpectedFeeMinor
	calc.NetVariance = batch.ReportedNetMinor - calc.ExpectedNetMinor

	// 4. Batch Arithmetic Invariant Checks
	reportedNetCheck := batch.ReportedGrossMinor - batch.ReportedFeeMinor - batch.ReportedRefundMinor
	if batch.ReportedNetMinor != reportedNetCheck {
		calc.ArithmeticError = true
	}
	if batch.ReportedRefundMinor != sumProcessedGatewayRefunds {
		calc.ArithmeticError = true
	}

	return calc, nil
}
