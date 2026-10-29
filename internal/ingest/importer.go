package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hackathon/settlement-agent/internal/money"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RowError struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

type ValidationError struct {
	Message string     `json:"message"`
	Rows    []RowError `json:"rows"`
}

func (v *ValidationError) Error() string {
	return fmt.Sprintf("%s (%d row errors)", v.Message, len(v.Rows))
}

type ImportResult struct {
	ImportID          int64  `json:"import_id"`
	Kind              string `json:"kind"`
	Status            string `json:"status"` // "completed" or "duplicate"
	RowsTotal         int    `json:"rows_total"`
	Inserted          int    `json:"inserted"`
	SkippedDuplicates int    `json:"skipped_duplicates"`
}

type Importer struct {
	pool *pgxpool.Pool
}

func NewImporter(pool *pgxpool.Pool) *Importer {
	return &Importer{pool: pool}
}

func (imp *Importer) ImportFile(ctx context.Context, kind, filename string, r io.Reader) (*ImportResult, error) {
	if err := ValidateKind(kind); err != nil {
		return nil, err
	}

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read file content: %w", err)
	}

	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// 1. Check duplicate file via SHA-256
	var existingID int64
	var existingRowsTotal int
	err = imp.pool.QueryRow(ctx, `
		SELECT id, rows_total FROM import_batches 
		WHERE kind = $1 AND sha256 = $2
	`, kind, hashStr).Scan(&existingID, &existingRowsTotal)
	if err == nil {
		return &ImportResult{
			ImportID:          existingID,
			Kind:              kind,
			Status:            "duplicate",
			RowsTotal:         existingRowsTotal,
			Inserted:          0,
			SkippedDuplicates: existingRowsTotal,
		}, nil
	} else if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("check duplicate file hash: %w", err)
	}

	// 2. Parse CSV
	reader := csv.NewReader(bytes.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse CSV: %w", err)
	}

	if len(records) < 1 {
		return nil, fmt.Errorf("CSV file is empty")
	}

	headers := records[0]
	headerMap := make(map[string]int)
	for i, h := range headers {
		headerMap[strings.TrimSpace(h)] = i
	}

	def := KindDefinitions[kind]
	for _, reqH := range def.RequiredHeaders {
		if _, ok := headerMap[reqH]; !ok {
			return nil, fmt.Errorf("missing required header column %q in %s", reqH, kind)
		}
	}

	dataRows := records[1:]
	var rowErrors []RowError

	// 3. Validate rows
	for idx, row := range dataRows {
		rowNum := idx + 2 // 1-based, header is line 1
		if len(rowErrors) >= 50 {
			break
		}

		imp.validateRow(kind, row, headerMap, rowNum, &rowErrors)
	}

	if len(rowErrors) > 0 {
		return nil, &ValidationError{
			Message: fmt.Sprintf("%d invalid rows; nothing imported", len(rowErrors)),
			Rows:    rowErrors,
		}
	}

	// 4. Insert rows inside transaction
	tx, err := imp.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	insertedCount := 0
	for _, row := range dataRows {
		inserted, err := imp.insertRow(ctx, tx, kind, row, headerMap, headers)
		if err != nil {
			return nil, fmt.Errorf("insert row into %s: %w", kind, err)
		}
		if inserted {
			insertedCount++
		}
	}

	skippedCount := len(dataRows) - insertedCount

	// Record in import_batches
	var importID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO import_batches (kind, filename, sha256, rows_total, inserted, skipped, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'completed')
		RETURNING id
	`, kind, filename, hashStr, len(dataRows), insertedCount, skippedCount).Scan(&importID)
	if err != nil {
		return nil, fmt.Errorf("record import batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit import transaction: %w", err)
	}

	return &ImportResult{
		ImportID:          importID,
		Kind:              kind,
		Status:            "completed",
		RowsTotal:         len(dataRows),
		Inserted:          insertedCount,
		SkippedDuplicates: skippedCount,
	}, nil
}

func (imp *Importer) validateRow(kind string, row []string, hmap map[string]int, rowNum int, errs *[]RowError) {
	get := func(col string) string {
		idx, ok := hmap[col]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	addErr := func(field, problem string) {
		if len(*errs) < 50 {
			*errs = append(*errs, RowError{Row: rowNum, Field: field, Problem: problem})
		}
	}

	switch kind {
	case KindFeeSchedule:
		method := get("method")
		if method != "card" && method != "upi" {
			addErr("method", "must be 'card' or 'upi'")
		}
		bpsStr := get("rate_bps")
		if bps, err := strconv.Atoi(bpsStr); err != nil || bps < 0 {
			addErr("rate_bps", "must be a non-negative integer")
		}
		if _, err := money.ParseMinor(get("fixed_fee")); err != nil {
			addErr("fixed_fee", err.Error())
		}
		if _, err := time.Parse("2006-01-02", get("effective_from")); err != nil {
			addErr("effective_from", "expected YYYY-MM-DD")
		}
		if effTo := get("effective_to"); effTo != "" {
			if _, err := time.Parse("2006-01-02", effTo); err != nil {
				addErr("effective_to", "expected YYYY-MM-DD")
			}
		}

	case KindOrders:
		if get("order_ref") == "" {
			addErr("order_ref", "cannot be empty")
		}
		if curr := get("currency"); curr != "INR" {
			addErr("currency", "only INR supported")
		}
		if _, err := money.ParseMinor(get("amount")); err != nil {
			addErr("amount", err.Error())
		}
		if _, err := ParseTimestamp(get("ordered_at")); err != nil {
			addErr("ordered_at", err.Error())
		}

	case KindOrderRefunds:
		if get("refund_ref") == "" {
			addErr("refund_ref", "cannot be empty")
		}
		if _, err := money.ParseMinor(get("amount")); err != nil {
			addErr("amount", err.Error())
		}
		if _, err := ParseTimestamp(get("refunded_at")); err != nil {
			addErr("refunded_at", err.Error())
		}

	case KindSettlementBatches:
		if get("batch_ref") == "" {
			addErr("batch_ref", "cannot be empty")
		}
		if curr := get("currency"); curr != "INR" {
			addErr("currency", "only INR supported")
		}
		if _, err := time.Parse("2006-01-02", get("settled_on")); err != nil {
			addErr("settled_on", "expected YYYY-MM-DD")
		}
		for _, f := range []string{"reported_gross", "reported_fee", "reported_refund", "reported_net"} {
			if _, err := money.ParseMinor(get(f)); err != nil {
				addErr(f, err.Error())
			}
		}

	case KindGatewayPayments:
		if get("gateway_payment_id") == "" {
			addErr("gateway_payment_id", "cannot be empty")
		}
		if curr := get("currency"); curr != "INR" {
			addErr("currency", "only INR supported")
		}
		if _, err := money.ParseMinor(get("amount")); err != nil {
			addErr("amount", err.Error())
		}
		method := get("method")
		if method != "card" && method != "upi" {
			addErr("method", "must be 'card' or 'upi'")
		}
		st := strings.ToLower(get("status"))
		if st == "success" {
			st = "captured"
		}
		if st != "captured" && st != "failed" && st != "authorized" {
			addErr("status", "must be captured, failed, or authorized")
		}
		if _, err := ParseTimestamp(get("captured_at")); err != nil {
			addErr("captured_at", err.Error())
		}

	case KindGatewayRefunds:
		if get("gateway_refund_id") == "" {
			addErr("gateway_refund_id", "cannot be empty")
		}
		if _, err := money.ParseMinor(get("amount")); err != nil {
			addErr("amount", err.Error())
		}
		st := strings.ToLower(get("status"))
		if st != "processed" && st != "pending" && st != "failed" {
			addErr("status", "must be processed, pending, or failed")
		}
		if _, err := ParseTimestamp(get("refunded_at")); err != nil {
			addErr("refunded_at", err.Error())
		}

	case KindBankDeposits:
		if get("bank_txn_id") == "" {
			addErr("bank_txn_id", "cannot be empty")
		}
		if curr := get("currency"); curr != "INR" {
			addErr("currency", "only INR supported")
		}
		if _, err := money.ParseMinor(get("amount")); err != nil {
			addErr("amount", err.Error())
		}
		if _, err := ParseBankDate(get("posted_on")); err != nil {
			addErr("posted_on", err.Error())
		}
	}
}

func (imp *Importer) insertRow(ctx context.Context, tx pgx.Tx, kind string, row []string, hmap map[string]int, headers []string) (bool, error) {
	get := func(col string) string {
		idx, ok := hmap[col]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	rawMap := make(map[string]string)
	for i, h := range headers {
		if i < len(row) {
			rawMap[h] = row[i]
		}
	}
	rawJSON, err := json.Marshal(rawMap)
	if err != nil {
		return false, fmt.Errorf("marshal raw JSON: %w", err)
	}

	switch kind {
	case KindFeeSchedule:
		method := get("method")
		bps, _ := strconv.Atoi(get("rate_bps"))
		fixed, _ := money.ParseMinor(get("fixed_fee"))
		from, _ := time.Parse("2006-01-02", get("effective_from"))
		var to *time.Time
		if effTo := get("effective_to"); effTo != "" {
			if t, err := time.Parse("2006-01-02", effTo); err == nil {
				to = &t
			}
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO gateway_fee_schedules (method, rate_bps, fixed_minor, effective_from, effective_to)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (method, effective_from) DO NOTHING
		`, method, bps, int64(fixed), from, to)
		return tag.RowsAffected() > 0, err

	case KindOrders:
		ref := get("order_ref")
		refNorm := NormalizeRef(ref)
		amt, _ := money.ParseMinor(get("amount"))
		status := get("status")
		orderedAt, _ := ParseTimestamp(get("ordered_at"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO orders (order_ref, order_ref_norm, amount_minor, currency, status, ordered_at, raw)
			VALUES ($1, $2, $3, 'INR', $4, $5, $6)
			ON CONFLICT (order_ref_norm) DO NOTHING
		`, ref, refNorm, int64(amt), status, orderedAt, rawJSON)
		return tag.RowsAffected() > 0, err

	case KindOrderRefunds:
		ref := get("refund_ref")
		refNorm := NormalizeRef(ref)
		orderRef := get("order_ref")
		orderRefNorm := NormalizeRef(orderRef)
		amt, _ := money.ParseMinor(get("amount"))
		refundedAt, _ := ParseTimestamp(get("refunded_at"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO order_refunds (refund_ref, refund_ref_norm, order_id, amount_minor, refunded_at, raw)
			VALUES ($1, $2, (SELECT id FROM orders WHERE order_ref_norm = $3), $4, $5, $6)
			ON CONFLICT (refund_ref_norm) DO NOTHING
		`, ref, refNorm, orderRefNorm, int64(amt), refundedAt, rawJSON)
		return tag.RowsAffected() > 0, err

	case KindSettlementBatches:
		batchRef := get("batch_ref")
		batchRefNorm := NormalizeRef(batchRef)
		settledOn, _ := time.Parse("2006-01-02", get("settled_on"))
		gross, _ := money.ParseMinor(get("reported_gross"))
		fee, _ := money.ParseMinor(get("reported_fee"))
		refnd, _ := money.ParseMinor(get("reported_refund"))
		net, _ := money.ParseMinor(get("reported_net"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO settlement_batches (batch_ref, batch_ref_norm, settled_on, currency, reported_gross_minor, reported_fee_minor, reported_refund_minor, reported_net_minor, raw)
			VALUES ($1, $2, $3, 'INR', $4, $5, $6, $7, $8)
			ON CONFLICT (batch_ref_norm) DO NOTHING
		`, batchRef, batchRefNorm, settledOn, int64(gross), int64(fee), int64(refnd), int64(net), rawJSON)
		return tag.RowsAffected() > 0, err

	case KindGatewayPayments:
		pmtID := get("gateway_payment_id")
		orderRefNorm := NormalizeRef(get("order_ref"))
		amt, _ := money.ParseMinor(get("amount"))
		method := get("method")
		status := strings.ToLower(get("status"))
		if status == "success" {
			status = "captured"
		}
		capturedAt, _ := ParseTimestamp(get("captured_at"))
		settlementRefNorm := NormalizeRef(get("settlement_ref"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO gateway_payments (gateway_payment_id, order_ref_norm, order_id, link_status, link_reason, amount_minor, currency, method, status, captured_at, settlement_ref_norm, raw)
			VALUES ($1, $2, (SELECT id FROM orders WHERE order_ref_norm = $2), 'unlinked', NULL, $3, 'INR', $4, $5, $6, $7, $8)
			ON CONFLICT (gateway_payment_id) DO NOTHING
		`, pmtID, orderRefNorm, int64(amt), method, status, capturedAt, settlementRefNorm, rawJSON)
		return tag.RowsAffected() > 0, err

	case KindGatewayRefunds:
		refundID := get("gateway_refund_id")
		pmtID := get("gateway_payment_id")
		refNorm := NormalizeRef(get("refund_ref"))
		amt, _ := money.ParseMinor(get("amount"))
		status := strings.ToLower(get("status"))
		refundedAt, _ := ParseTimestamp(get("refunded_at"))
		settlementRefNorm := NormalizeRef(get("settlement_ref"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO gateway_refunds (gateway_refund_id, gateway_payment_id, refund_ref_norm, amount_minor, status, refunded_at, settlement_ref_norm, raw)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (gateway_refund_id) DO NOTHING
		`, refundID, pmtID, refNorm, int64(amt), status, refundedAt, settlementRefNorm, rawJSON)
		return tag.RowsAffected() > 0, err

	case KindBankDeposits:
		txnID := get("bank_txn_id")
		ref := get("reference")
		narration := StripUnicodeJunk(get("narration"))
		amt, _ := money.ParseMinor(get("amount"))
		postedOn, _ := ParseBankDate(get("posted_on"))

		tag, err := tx.Exec(ctx, `
			INSERT INTO bank_deposits (bank_txn_id, reference, narration, amount_minor, currency, posted_on, raw)
			VALUES ($1, $2, $3, $4, 'INR', $5, $6)
			ON CONFLICT (bank_txn_id) DO NOTHING
		`, txnID, ref, narration, int64(amt), postedOn, rawJSON)
		return tag.RowsAffected() > 0, err
	}

	return false, fmt.Errorf("unhandled kind: %s", kind)
}
