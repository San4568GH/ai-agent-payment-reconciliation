package recon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) LoadInput(ctx context.Context, asOf time.Time, windowDays int) (*ReconInput, error) {
	in := &ReconInput{
		AsOf:                 asOf,
		SettlementWindowDays: windowDays,
	}

	// 1. Batches
	bRows, err := s.pool.Query(ctx, `
		SELECT id, batch_ref, batch_ref_norm, settled_on, currency,
		       reported_gross_minor, reported_fee_minor, reported_refund_minor, reported_net_minor
		FROM settlement_batches
		ORDER BY settled_on ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load batches: %w", err)
	}
	defer bRows.Close()

	for bRows.Next() {
		var b SettlementBatch
		var gross, fee, refnd, net int64
		if err := bRows.Scan(&b.ID, &b.BatchRef, &b.BatchRefNorm, &b.SettledOn, &b.Currency, &gross, &fee, &refnd, &net); err != nil {
			return nil, fmt.Errorf("scan batch: %w", err)
		}
		b.ReportedGrossMinor = money.Minor(gross)
		b.ReportedFeeMinor = money.Minor(fee)
		b.ReportedRefundMinor = money.Minor(refnd)
		b.ReportedNetMinor = money.Minor(net)
		in.Batches = append(in.Batches, b)
	}

	// 2. Orders
	oRows, err := s.pool.Query(ctx, `
		SELECT id, order_ref, order_ref_norm, amount_minor, currency, status, ordered_at
		FROM orders
	`)
	if err != nil {
		return nil, fmt.Errorf("load orders: %w", err)
	}
	defer oRows.Close()

	for oRows.Next() {
		var o Order
		var amt int64
		if err := oRows.Scan(&o.ID, &o.OrderRef, &o.OrderRefNorm, &amt, &o.Currency, &o.Status, &o.OrderedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		o.AmountMinor = money.Minor(amt)
		in.Orders = append(in.Orders, o)
	}

	// 3. Order Refunds
	orRows, err := s.pool.Query(ctx, `
		SELECT id, refund_ref, refund_ref_norm, order_id, amount_minor, refunded_at
		FROM order_refunds
	`)
	if err != nil {
		return nil, fmt.Errorf("load order refunds: %w", err)
	}
	defer orRows.Close()

	for orRows.Next() {
		var r OrderRefund
		var amt int64
		if err := orRows.Scan(&r.ID, &r.RefundRef, &r.RefundRefNorm, &r.OrderID, &amt, &r.RefundedAt); err != nil {
			return nil, fmt.Errorf("scan order refund: %w", err)
		}
		r.AmountMinor = money.Minor(amt)
		in.OrderRefunds = append(in.OrderRefunds, r)
	}

	// 4. Gateway Payments
	pRows, err := s.pool.Query(ctx, `
		SELECT id, gateway_payment_id, order_ref_norm, order_id, link_status, COALESCE(link_reason, ''),
		       amount_minor, currency, method, status, captured_at, settlement_ref_norm
		FROM gateway_payments
	`)
	if err != nil {
		return nil, fmt.Errorf("load gateway payments: %w", err)
	}
	defer pRows.Close()

	for pRows.Next() {
		var p GatewayPayment
		var amt int64
		if err := pRows.Scan(&p.ID, &p.GatewayPaymentID, &p.OrderRefNorm, &p.OrderID, &p.LinkStatus, &p.LinkReason,
			&amt, &p.Currency, &p.Method, &p.Status, &p.CapturedAt, &p.SettlementRefNorm); err != nil {
			return nil, fmt.Errorf("scan gateway payment: %w", err)
		}
		p.AmountMinor = money.Minor(amt)
		in.Payments = append(in.Payments, p)
	}

	// 5. Gateway Refunds
	grRows, err := s.pool.Query(ctx, `
		SELECT id, gateway_refund_id, gateway_payment_id, refund_ref_norm, amount_minor,
		       status, refunded_at, settlement_ref_norm, internal_refund_id
		FROM gateway_refunds
	`)
	if err != nil {
		return nil, fmt.Errorf("load gateway refunds: %w", err)
	}
	defer grRows.Close()

	for grRows.Next() {
		var gr GatewayRefund
		var amt int64
		if err := grRows.Scan(&gr.ID, &gr.GatewayRefundID, &gr.GatewayPaymentID, &gr.RefundRefNorm, &amt,
			&gr.Status, &gr.RefundedAt, &gr.SettlementRefNorm, &gr.InternalRefundID); err != nil {
			return nil, fmt.Errorf("scan gateway refund: %w", err)
		}
		gr.AmountMinor = money.Minor(amt)
		in.GatewayRefunds = append(in.GatewayRefunds, gr)
	}

	// 6. Bank Deposits
	dRows, err := s.pool.Query(ctx, `
		SELECT id, bank_txn_id, COALESCE(reference, ''), narration, amount_minor, currency, posted_on
		FROM bank_deposits
		ORDER BY posted_on ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load bank deposits: %w", err)
	}
	defer dRows.Close()

	for dRows.Next() {
		var d BankDeposit
		var amt int64
		if err := dRows.Scan(&d.ID, &d.BankTxnID, &d.Reference, &d.Narration, &amt, &d.Currency, &d.PostedOn); err != nil {
			return nil, fmt.Errorf("scan bank deposit: %w", err)
		}
		d.AmountMinor = money.Minor(amt)
		in.BankDeposits = append(in.BankDeposits, d)
	}

	// 7. Fee Schedules
	fsRows, err := s.pool.Query(ctx, `
		SELECT id, method, rate_bps, fixed_minor, effective_from, effective_to
		FROM gateway_fee_schedules
		ORDER BY effective_from ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("load fee schedules: %w", err)
	}
	defer fsRows.Close()

	for fsRows.Next() {
		var fs FeeSchedule
		var fixed int64
		if err := fsRows.Scan(&fs.ID, &fs.Method, &fs.RateBps, &fixed, &fs.EffectiveFrom, &fs.EffectiveTo); err != nil {
			return nil, fmt.Errorf("scan fee schedule: %w", err)
		}
		fs.FixedMinor = money.Minor(fixed)
		in.FeeSchedules = append(in.FeeSchedules, fs)
	}

	// 8. Existing Matches
	mRows, err := s.pool.Query(ctx, `
		SELECT id, batch_id, deposit_id, applied_amount_minor, method
		FROM matches
	`)
	if err != nil {
		return nil, fmt.Errorf("load matches: %w", err)
	}
	defer mRows.Close()

	for mRows.Next() {
		var m ExistingMatch
		var amt int64
		if err := mRows.Scan(&m.ID, &m.BatchID, &m.DepositID, &amt, &m.Method); err != nil {
			return nil, fmt.Errorf("scan match: %w", err)
		}
		m.AppliedAmountMinor = money.Minor(amt)
		in.ExistingMatches = append(in.ExistingMatches, m)
	}

	return in, nil
}

func (s *Store) SaveOutput(ctx context.Context, out *ReconOutput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Update Payment Links
	for _, p := range out.PaymentLinksToUpdate {
		_, err := tx.Exec(ctx, `
			UPDATE gateway_payments
			SET order_id = $1, link_status = $2, link_reason = $3
			WHERE id = $4
		`, p.OrderID, p.LinkStatus, p.LinkReason, p.PaymentID)
		if err != nil {
			return fmt.Errorf("update payment link for id %d: %w", p.PaymentID, err)
		}
	}

	// 2. Update Refund Links
	for _, r := range out.RefundLinksToUpdate {
		_, err := tx.Exec(ctx, `
			UPDATE gateway_refunds
			SET internal_refund_id = $1
			WHERE id = $2
		`, r.InternalRefundID, r.GatewayRefundID)
		if err != nil {
			return fmt.Errorf("update refund link for id %d: %w", r.GatewayRefundID, err)
		}
	}

	// 3. Upsert Recon Results
	for _, r := range out.BatchResults {
		_, err := tx.Exec(ctx, `
			INSERT INTO recon_results (
				batch_id, as_of, status,
				expected_gross_minor, expected_fee_minor, expected_refund_minor, expected_net_minor,
				reported_net_minor, bank_total_minor, variance_minor, computed_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
			ON CONFLICT (batch_id) DO UPDATE SET
				as_of = EXCLUDED.as_of,
				status = EXCLUDED.status,
				expected_gross_minor = EXCLUDED.expected_gross_minor,
				expected_fee_minor = EXCLUDED.expected_fee_minor,
				expected_refund_minor = EXCLUDED.expected_refund_minor,
				expected_net_minor = EXCLUDED.expected_net_minor,
				reported_net_minor = EXCLUDED.reported_net_minor,
				bank_total_minor = EXCLUDED.bank_total_minor,
				variance_minor = EXCLUDED.variance_minor,
				computed_at = now()
		`, r.BatchID, r.AsOf, r.Status,
			int64(r.ExpectedGrossMinor), int64(r.ExpectedFeeMinor), int64(r.ExpectedRefundMinor), int64(r.ExpectedNetMinor),
			int64(r.ReportedNetMinor), int64(r.BankTotalMinor), int64(r.VarianceMinor),
		)
		if err != nil {
			return fmt.Errorf("upsert recon result for batch %d: %w", r.BatchID, err)
		}
	}

	// 4. Insert Matches
	for _, m := range out.MatchesToInsert {
		_, err := tx.Exec(ctx, `
			INSERT INTO matches (batch_id, deposit_id, applied_amount_minor, method, created_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (deposit_id) DO NOTHING
		`, m.BatchID, m.DepositID, int64(m.AppliedAmountMinor), m.Method)
		if err != nil {
			return fmt.Errorf("insert match deposit %d: %w", m.DepositID, err)
		}
	}

	// 5. Upsert Exceptions
	for _, e := range out.ExceptionsToUpsert {
		contextBytes, err := json.Marshal(e.Context)
		if err != nil {
			return fmt.Errorf("marshal exception context: %w", err)
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO exceptions (
				batch_id, type, reason_code, severity, status, variance_minor, context, last_as_of, opened_at, updated_at
			) VALUES ($1, $2, $3, $4, 'open', $5, $6, $7, now(), now())
			ON CONFLICT (batch_id, type) DO UPDATE SET
				reason_code = EXCLUDED.reason_code,
				severity = EXCLUDED.severity,
				variance_minor = EXCLUDED.variance_minor,
				context = EXCLUDED.context,
				last_as_of = EXCLUDED.last_as_of,
				updated_at = now()
			WHERE exceptions.status IN ('open', 'investigating', 'proposal_ready', 'escalated')
		`, e.BatchID, e.Type, e.ReasonCode, e.Severity, int64(e.VarianceMinor), contextBytes, e.LastAsOf)
		if err != nil {
			return fmt.Errorf("upsert exception (batch %d, type %s): %w", e.BatchID, e.Type, err)
		}
	}

	// 6. Auto-Resolve Cleared Exceptions
	for _, key := range out.ExceptionsToResolve {
		_, err := tx.Exec(ctx, `
			UPDATE exceptions
			SET status = 'resolved', updated_at = now()
			WHERE batch_id = $1 AND type = $2 AND status IN ('open', 'investigating', 'proposal_ready', 'escalated')
		`, key.BatchID, key.Type)
		if err != nil {
			return fmt.Errorf("auto-resolve exception (batch %d, type %s): %w", key.BatchID, key.Type, err)
		}
	}

	return tx.Commit(ctx)
}
