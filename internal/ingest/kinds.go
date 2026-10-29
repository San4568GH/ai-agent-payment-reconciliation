package ingest

import (
	"fmt"
)

// Supported CSV kinds in dependency order
const (
	KindFeeSchedule       = "fee_schedule"
	KindOrders            = "orders"
	KindOrderRefunds      = "order_refunds"
	KindSettlementBatches = "settlement_batches"
	KindGatewayPayments   = "gateway_payments"
	KindGatewayRefunds    = "gateway_refunds"
	KindBankDeposits      = "bank_deposits"
)

var ImportOrder = []string{
	KindFeeSchedule,
	KindOrders,
	KindOrderRefunds,
	KindSettlementBatches,
	KindGatewayPayments,
	KindGatewayRefunds,
	KindBankDeposits,
}

var KindFilenames = map[string]string{
	KindFeeSchedule:       "fee_schedule.csv",
	KindOrders:            "orders.csv",
	KindOrderRefunds:      "order_refunds.csv",
	KindSettlementBatches: "settlement_batches.csv",
	KindGatewayPayments:   "gateway_payments.csv",
	KindGatewayRefunds:    "gateway_refunds.csv",
	KindBankDeposits:      "bank_deposits.csv",
}

type KindDef struct {
	Name            string
	RequiredHeaders []string
}

var KindDefinitions = map[string]KindDef{
	KindFeeSchedule: {
		Name:            KindFeeSchedule,
		RequiredHeaders: []string{"method", "rate_bps", "fixed_fee", "effective_from", "effective_to"},
	},
	KindOrders: {
		Name:            KindOrders,
		RequiredHeaders: []string{"order_ref", "amount", "currency", "status", "ordered_at"},
	},
	KindOrderRefunds: {
		Name:            KindOrderRefunds,
		RequiredHeaders: []string{"refund_ref", "order_ref", "amount", "refunded_at"},
	},
	KindSettlementBatches: {
		Name:            KindSettlementBatches,
		RequiredHeaders: []string{"batch_ref", "settled_on", "currency", "reported_gross", "reported_fee", "reported_refund", "reported_net"},
	},
	KindGatewayPayments: {
		Name:            KindGatewayPayments,
		RequiredHeaders: []string{"gateway_payment_id", "order_ref", "amount", "currency", "method", "status", "captured_at", "settlement_ref"},
	},
	KindGatewayRefunds: {
		Name:            KindGatewayRefunds,
		RequiredHeaders: []string{"gateway_refund_id", "gateway_payment_id", "refund_ref", "amount", "status", "refunded_at", "settlement_ref"},
	},
	KindBankDeposits: {
		Name:            KindBankDeposits,
		RequiredHeaders: []string{"bank_txn_id", "reference", "narration", "amount", "currency", "posted_on"},
	},
}

func ValidateKind(kind string) error {
	if _, ok := KindDefinitions[kind]; !ok {
		return fmt.Errorf("unknown import kind: %q", kind)
	}
	return nil
}
