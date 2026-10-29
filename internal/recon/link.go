package recon

// LinkRecords performs pure linking of gateway payments to orders,
// and gateway refunds to internal order refunds according to F.2 rules.
func LinkRecords(in *ReconInput) ([]PaymentLinkUpdate, []RefundLinkUpdate) {
	// Build order lookup maps
	// map of order_ref_norm -> *Order
	ordersByRef := make(map[string]*Order)
	for i := range in.Orders {
		o := &in.Orders[i]
		ordersByRef[o.OrderRefNorm] = o
	}

	var paymentUpdates []PaymentLinkUpdate
	paymentOrderMap := make(map[string]int64) // gateway_payment_id -> order_id

	for _, p := range in.Payments {
		update := PaymentLinkUpdate{
			PaymentID:  p.ID,
			LinkStatus: "unlinked",
		}

		if p.OrderRefNorm == "" {
			update.LinkReason = "missing_order_reference"
			paymentUpdates = append(paymentUpdates, update)
			continue
		}

		order, exists := ordersByRef[p.OrderRefNorm]
		if !exists {
			update.LinkReason = "order_not_found"
			paymentUpdates = append(paymentUpdates, update)
			continue
		}

		if p.AmountMinor != order.AmountMinor {
			update.LinkReason = "amount_mismatch"
			paymentUpdates = append(paymentUpdates, update)
			continue
		}

		if p.Currency != order.Currency {
			update.LinkReason = "currency_mismatch"
			paymentUpdates = append(paymentUpdates, update)
			continue
		}

		// Exact match found!
		update.OrderID = &order.ID
		update.LinkStatus = "linked"
		update.LinkReason = ""
		paymentOrderMap[p.GatewayPaymentID] = order.ID
		paymentUpdates = append(paymentUpdates, update)
	}

	// Internal refund lookup maps
	refundsByRef := make(map[string]*OrderRefund)
	refundsByOrderAndAmount := make(map[int64][]*OrderRefund)
	for i := range in.OrderRefunds {
		r := &in.OrderRefunds[i]
		refundsByRef[r.RefundRefNorm] = r
		if r.OrderID != nil {
			refundsByOrderAndAmount[*r.OrderID] = append(refundsByOrderAndAmount[*r.OrderID], r)
		}
	}

	var refundUpdates []RefundLinkUpdate
	for _, gr := range in.GatewayRefunds {
		update := RefundLinkUpdate{
			GatewayRefundID: gr.ID,
		}

		// Strategy 1: match by refund_ref_norm
		if gr.RefundRefNorm != "" {
			if matched, exists := refundsByRef[gr.RefundRefNorm]; exists {
				update.InternalRefundID = &matched.ID
				refundUpdates = append(refundUpdates, update)
				continue
			}
		}

		// Strategy 2: same order and amount when exactly one candidate exists
		orderID, hasOrder := paymentOrderMap[gr.GatewayPaymentID]
		if hasOrder {
			candidates := refundsByOrderAndAmount[orderID]
			var matchingCandidates []*OrderRefund
			for _, c := range candidates {
				if c.AmountMinor == gr.AmountMinor {
					matchingCandidates = append(matchingCandidates, c)
				}
			}
			if len(matchingCandidates) == 1 {
				update.InternalRefundID = &matchingCandidates[0].ID
				refundUpdates = append(refundUpdates, update)
				continue
			}
		}

		// Not matched: internal_refund_id stays nil (unrecorded refund)
		refundUpdates = append(refundUpdates, update)
	}

	return paymentUpdates, refundUpdates
}
