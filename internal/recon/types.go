package recon

import (
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
)

// Domain types used by the pure reconciliation engine
type Order struct {
	ID           int64
	OrderRef     string
	OrderRefNorm string
	AmountMinor  money.Minor
	Currency     string
	Status       string
	OrderedAt    time.Time
}

type OrderRefund struct {
	ID            int64
	RefundRef     string
	RefundRefNorm string
	OrderID       *int64
	AmountMinor   money.Minor
	RefundedAt    time.Time
}

type GatewayPayment struct {
	ID                int64
	GatewayPaymentID  string
	OrderRefNorm      string
	OrderID           *int64
	LinkStatus        string // "linked" or "unlinked"
	LinkReason        string
	AmountMinor       money.Minor
	Currency          string
	Method            string // "card" or "upi"
	Status            string // "captured", "failed", "authorized"
	CapturedAt        time.Time
	SettlementRefNorm string
}

type GatewayRefund struct {
	ID                int64
	GatewayRefundID   string
	GatewayPaymentID  string
	RefundRefNorm     string
	AmountMinor       money.Minor
	Status            string // "processed", "pending", "failed"
	RefundedAt        time.Time
	SettlementRefNorm string
	InternalRefundID  *int64
}

type SettlementBatch struct {
	ID                  int64
	BatchRef            string
	BatchRefNorm        string
	SettledOn           time.Time
	Currency            string
	ReportedGrossMinor  money.Minor
	ReportedFeeMinor    money.Minor
	ReportedRefundMinor money.Minor
	ReportedNetMinor    money.Minor
}

type BankDeposit struct {
	ID          int64
	BankTxnID   string
	Reference   string
	Narration   string
	AmountMinor money.Minor
	Currency    string
	PostedOn    time.Time
}

type FeeSchedule struct {
	ID            int64
	Method        string // "card" or "upi"
	RateBps       int
	FixedMinor    money.Minor
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

type ExistingMatch struct {
	ID                 int64
	BatchID            int64
	DepositID          int64
	AppliedAmountMinor money.Minor
	Method             string
}

// Input for the pure reconciliation engine
type ReconInput struct {
	AsOf                 time.Time
	SettlementWindowDays int
	Batches              []SettlementBatch
	Payments             []GatewayPayment
	Orders               []Order
	GatewayRefunds       []GatewayRefund
	OrderRefunds         []OrderRefund
	BankDeposits         []BankDeposit
	FeeSchedules         []FeeSchedule
	ExistingMatches      []ExistingMatch
}

// Output from the pure reconciliation engine
type ReconOutput struct {
	AsOf                 time.Time
	BatchResults         []BatchResult
	MatchesToInsert      []MatchRecord
	ExceptionsToUpsert   []ExceptionRecord
	ExceptionsToResolve  []ExceptionKey
	PaymentLinksToUpdate []PaymentLinkUpdate
	RefundLinksToUpdate  []RefundLinkUpdate
}

type BatchResult struct {
	BatchID             int64
	BatchRef            string
	AsOf                time.Time
	Status              string // "matched", "pending", "exception"
	ExpectedGrossMinor  money.Minor
	ExpectedFeeMinor    money.Minor
	ExpectedRefundMinor money.Minor
	ExpectedNetMinor    money.Minor
	ReportedNetMinor    money.Minor
	BankTotalMinor      money.Minor
	VarianceMinor       money.Minor
}

type MatchRecord struct {
	BatchID            int64
	DepositID          int64
	AppliedAmountMinor money.Minor
	Method             string // "reference", "amount_date", "remediation"
}

type ExceptionRecord struct {
	BatchID       int64
	Type          string // "FEE_VARIANCE", "DELAYED_OR_SPLIT_DEPOSIT", "REFUND_NOT_IN_LEDGER", "MISSING_DEPOSIT", "UNEXPLAINED_VARIANCE"
	ReasonCode    string
	Severity      string // "low", "medium", "high"
	Status        string // "open", "resolved"
	VarianceMinor money.Minor
	Context       map[string]any
	LastAsOf      time.Time
}

type ExceptionKey struct {
	BatchID int64
	Type    string
}

type PaymentLinkUpdate struct {
	PaymentID  int64
	OrderID    *int64
	LinkStatus string
	LinkReason string
}

type RefundLinkUpdate struct {
	GatewayRefundID  int64
	InternalRefundID *int64
}
