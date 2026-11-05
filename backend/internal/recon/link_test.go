package recon_test

import (
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/recon"
)

func TestLinkRecords_Payments(t *testing.T) {
	now := time.Now()
	in := &recon.ReconInput{
		Orders: []recon.Order{
			{ID: 1, OrderRefNorm: "ORD100", AmountMinor: 500000, Currency: "INR", Status: "captured", OrderedAt: now},
			{ID: 2, OrderRefNorm: "ORD200", AmountMinor: 300000, Currency: "INR", Status: "captured", OrderedAt: now},
		},
		Payments: []recon.GatewayPayment{
			{ID: 10, GatewayPaymentID: "PAY10", OrderRefNorm: "ORD100", AmountMinor: 500000, Currency: "INR"},  // exact match
			{ID: 11, GatewayPaymentID: "PAY11", OrderRefNorm: "", AmountMinor: 500000, Currency: "INR"},        // blank ref
			{ID: 12, GatewayPaymentID: "PAY12", OrderRefNorm: "ORD999", AmountMinor: 500000, Currency: "INR"},     // not found
			{ID: 13, GatewayPaymentID: "PAY13", OrderRefNorm: "ORD200", AmountMinor: 250000, Currency: "INR"},     // amount mismatch
			{ID: 14, GatewayPaymentID: "PAY14", OrderRefNorm: "ORD200", AmountMinor: 300000, Currency: "USD"},     // currency mismatch
		},
	}

	pUpdates, _ := recon.LinkRecords(in)
	if len(pUpdates) != 5 {
		t.Fatalf("expected 5 payment updates, got %d", len(pUpdates))
	}

	// 1. Exact match
	if pUpdates[0].LinkStatus != "linked" || *pUpdates[0].OrderID != 1 {
		t.Errorf("PAY10 expected linked to Order 1, got %+v", pUpdates[0])
	}
	// 2. Blank ref
	if pUpdates[1].LinkStatus != "unlinked" || pUpdates[1].LinkReason != "missing_order_reference" {
		t.Errorf("PAY11 expected missing_order_reference, got %+v", pUpdates[1])
	}
	// 3. Not found
	if pUpdates[2].LinkStatus != "unlinked" || pUpdates[2].LinkReason != "order_not_found" {
		t.Errorf("PAY12 expected order_not_found, got %+v", pUpdates[2])
	}
	// 4. Amount mismatch
	if pUpdates[3].LinkStatus != "unlinked" || pUpdates[3].LinkReason != "amount_mismatch" {
		t.Errorf("PAY13 expected amount_mismatch, got %+v", pUpdates[3])
	}
	// 5. Currency mismatch
	if pUpdates[4].LinkStatus != "unlinked" || pUpdates[4].LinkReason != "currency_mismatch" {
		t.Errorf("PAY14 expected currency_mismatch, got %+v", pUpdates[4])
	}
}

func TestLinkRecords_Refunds(t *testing.T) {
	orderID := int64(1)
	in := &recon.ReconInput{
		Orders: []recon.Order{
			{ID: 1, OrderRefNorm: "ORD100", AmountMinor: 500000, Currency: "INR"},
		},
		Payments: []recon.GatewayPayment{
			{ID: 10, GatewayPaymentID: "PAY10", OrderRefNorm: "ORD100", AmountMinor: 500000, Currency: "INR"},
		},
		OrderRefunds: []recon.OrderRefund{
			{ID: 101, RefundRefNorm: "REFMATCH", OrderID: &orderID, AmountMinor: 100000},
			{ID: 102, RefundRefNorm: "REFOTHER", OrderID: &orderID, AmountMinor: 50000},
		},
		GatewayRefunds: []recon.GatewayRefund{
			// Match by ref
			{ID: 201, GatewayRefundID: "GWREF1", GatewayPaymentID: "PAY10", RefundRefNorm: "REFMATCH", AmountMinor: 100000},
			// Match by order and amount (fallback)
			{ID: 202, GatewayRefundID: "GWREF2", GatewayPaymentID: "PAY10", RefundRefNorm: "UNKNOWN", AmountMinor: 50000},
			// Unrecorded refund (Scenario R)
			{ID: 203, GatewayRefundID: "GWREF3", GatewayPaymentID: "PAY10", RefundRefNorm: "REFNONE", AmountMinor: 75000},
		},
	}

	_, rUpdates := recon.LinkRecords(in)
	if len(rUpdates) != 3 {
		t.Fatalf("expected 3 refund updates, got %d", len(rUpdates))
	}

	// 1. Matched by ref
	if rUpdates[0].InternalRefundID == nil || *rUpdates[0].InternalRefundID != 101 {
		t.Errorf("GWREF1 expected matched to 101, got %+v", rUpdates[0])
	}
	// 2. Matched by fallback (order and amount)
	if rUpdates[1].InternalRefundID == nil || *rUpdates[1].InternalRefundID != 102 {
		t.Errorf("GWREF2 expected matched to 102, got %+v", rUpdates[1])
	}
	// 3. Unrecorded refund
	if rUpdates[2].InternalRefundID != nil {
		t.Errorf("GWREF3 expected nil internal refund, got %v", *rUpdates[2].InternalRefundID)
	}
}
