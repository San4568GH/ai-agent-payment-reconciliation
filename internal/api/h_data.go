package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/hackathon/settlement-agent/internal/recon"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DataHandler struct {
	pool       *pgxpool.Pool
	cfg        *config.Config
	importer   *ingest.Importer
	reconStore *recon.Store
	engine     *recon.Engine
}

func NewDataHandler(pool *pgxpool.Pool, cfg *config.Config) *DataHandler {
	return &DataHandler{
		pool:       pool,
		cfg:        cfg,
		importer:   ingest.NewImporter(pool),
		reconStore: recon.NewStore(pool),
		engine:     recon.NewEngine(),
	}
}

// POST /api/imports: multipart form with kind and file
func (h *DataHandler) HandleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "failed to parse multipart form", nil)
		return
	}

	kind := r.FormValue("kind")
	if kind == "" {
		WriteError(w, http.StatusBadRequest, "validation_error", "missing required 'kind' parameter", nil)
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "missing required 'file' upload", nil)
		return
	}
	defer file.Close()

	res, err := h.importer.ImportFile(r.Context(), kind, fileHeader.Filename, file)
	if err != nil {
		var valErr *ingest.ValidationError
		if errors.As(err, &valErr) {
			WriteError(w, http.StatusUnprocessableEntity, "validation_error", valErr.Message, map[string]any{
				"rows": valErr.Rows,
			})
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	if res.Status == "duplicate" {
		WriteJSON(w, http.StatusOK, res)
		return
	}

	WriteJSON(w, http.StatusCreated, res)
}

// POST /api/imports/seed: loads all CSV files in SEED_DIR in dependency order
func (h *DataHandler) HandleSeedImport(w http.ResponseWriter, r *http.Request) {
	seedDir := h.cfg.SeedDir
	var results []*ingest.ImportResult

	for _, kind := range ingest.ImportOrder {
		filename := ingest.KindFilenames[kind]
		filePath := filepath.Join(seedDir, filename)

		f, err := os.Open(filePath)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("cannot open seed file %s: %v", filename, err), nil)
			return
		}

		res, err := h.importer.ImportFile(r.Context(), kind, filename, f)
		f.Close()
		if err != nil {
			var valErr *ingest.ValidationError
			if errors.As(err, &valErr) {
				WriteError(w, http.StatusUnprocessableEntity, "validation_error", fmt.Sprintf("seed file %s invalid: %s", filename, valErr.Message), map[string]any{
					"file": filename,
					"rows": valErr.Rows,
				})
				return
			}
			WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("import seed %s failed: %v", filename, err), nil)
			return
		}

		results = append(results, res)
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "completed",
		"imports": results,
	})
}

// GET /api/imports: list import history
func (h *DataHandler) HandleListImports(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, kind, filename, sha256, rows_total, inserted, skipped, status, created_at
		FROM import_batches
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("query imports: %v", err), nil)
		return
	}
	defer rows.Close()

	var batches []map[string]any
	for rows.Next() {
		var id int64
		var kind, filename, sha256, status string
		var total, ins, skip int
		var created time.Time
		if err := rows.Scan(&id, &kind, &filename, &sha256, &total, &ins, &skip, &status, &created); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("scan import batch: %v", err), nil)
			return
		}
		batches = append(batches, map[string]any{
			"id":         id,
			"kind":       kind,
			"filename":   filename,
			"sha256":     sha256,
			"rows_total": total,
			"inserted":   ins,
			"skipped":    skip,
			"status":     status,
			"created_at": created.Format(time.RFC3339),
		})
	}

	if batches == nil {
		batches = []map[string]any{}
	}

	WriteJSON(w, http.StatusOK, batches)
}

// POST /api/settlement-runs: Run reconciliation for as_of date
func (h *DataHandler) HandleSettlementRun(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AsOf string `json:"as_of"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid request body", nil)
		return
	}

	if body.AsOf == "" {
		WriteError(w, http.StatusBadRequest, "validation_error", "missing required 'as_of' date (YYYY-MM-DD)", nil)
		return
	}

	asOf, err := time.Parse("2006-01-02", body.AsOf)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "as_of must be in format YYYY-MM-DD", nil)
		return
	}

	// Check if batches are imported
	var batchCount int
	if err := h.pool.QueryRow(r.Context(), "SELECT count(*) FROM settlement_batches").Scan(&batchCount); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	if batchCount == 0 {
		WriteError(w, http.StatusConflict, "conflict", "no settlement batches imported yet", nil)
		return
	}

	// 1. Load input
	input, err := h.reconStore.LoadInput(r.Context(), asOf, h.cfg.SettlementWindowDays)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("load recon input: %v", err), nil)
		return
	}

	// 2. Pure Recon Run
	output := h.engine.Run(input)

	// 3. Save output
	if err := h.reconStore.SaveOutput(r.Context(), output); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("save recon output: %v", err), nil)
		return
	}

	// Count opened vs cleared
	var openExceptionsCount int
	h.pool.QueryRow(r.Context(), "SELECT count(*) FROM exceptions WHERE status = 'open'").Scan(&openExceptionsCount)

	// Record audit event
	actor := ActorFromContext(r.Context())
	auditDetails, _ := json.Marshal(map[string]any{
		"as_of":             body.AsOf,
		"batches":           len(output.BatchResults),
		"exceptions_upsert": len(output.ExceptionsToUpsert),
	})
	h.pool.Exec(r.Context(), `
		INSERT INTO audit_events (actor, event_type, details, created_at)
		VALUES ($1, 'recon_run', $2, now())
	`, actor, auditDetails)

	matched := 0
	pending := 0
	for _, br := range output.BatchResults {
		if br.Status == "matched" {
			matched++
		} else if br.Status == "pending" {
			pending++
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"as_of":              body.AsOf,
		"batches":            len(output.BatchResults),
		"matched":            matched,
		"pending":            pending,
		"exceptions_open":    openExceptionsCount,
		"exceptions_opened":  len(output.ExceptionsToUpsert),
		"exceptions_cleared": len(output.ExceptionsToResolve),
	})
}

// GET /api/summary: Dashboard KPI cards from latest recon results
func (h *DataHandler) HandleSummary(w http.ResponseWriter, r *http.Request) {
	var asOf time.Time
	err := h.pool.QueryRow(r.Context(), "SELECT COALESCE(max(as_of), now()) FROM recon_results").Scan(&asOf)
	if err != nil && err != pgx.ErrNoRows {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	var vol, expNet, bankRec, netVar int64
	var total, matched int

	h.pool.QueryRow(r.Context(), `
		SELECT 
			COALESCE(sum(expected_gross_minor), 0),
			COALESCE(sum(expected_net_minor), 0),
			COALESCE(sum(bank_total_minor), 0),
			COALESCE(sum(variance_minor), 0),
			count(*),
			count(*) FILTER (WHERE status = 'matched')
		FROM recon_results
	`).Scan(&vol, &expNet, &bankRec, &netVar, &total, &matched)

	var openEx int
	h.pool.QueryRow(r.Context(), "SELECT count(*) FROM exceptions WHERE status = 'open'").Scan(&openEx)

	WriteJSON(w, http.StatusOK, map[string]any{
		"as_of":                asOf.Format("2006-01-02"),
		"payment_volume_minor": vol,
		"expected_net_minor":   expNet,
		"bank_received_minor":  bankRec,
		"batches_total":        total,
		"batches_matched":      matched,
		"open_exceptions":      openEx,
		"net_variance_minor":   netVar,
	})
}

// GET /api/settlements: List settlement batches with results
func (h *DataHandler) HandleListSettlements(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	query := `
		SELECT b.id, b.batch_ref, b.settled_on,
		       b.reported_gross_minor, b.reported_fee_minor, b.reported_refund_minor, b.reported_net_minor,
		       COALESCE(r.expected_net_minor, 0), COALESCE(r.bank_total_minor, 0),
		       COALESCE(r.variance_minor, 0), COALESCE(r.status, 'unreconciled')
		FROM settlement_batches b
		LEFT JOIN recon_results r ON b.id = r.batch_id
	`
	var rows pgx.Rows
	var err error

	if statusFilter != "" {
		query += " WHERE r.status = $1 ORDER BY b.settled_on ASC"
		rows, err = h.pool.Query(r.Context(), query, statusFilter)
	} else {
		query += " ORDER BY b.settled_on ASC"
		rows, err = h.pool.Query(r.Context(), query)
	}

	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id int64
		var ref, st string
		var settledOn time.Time
		var rGross, rFee, rRef, rNet, eNet, bankTot, varMin int64

		if err := rows.Scan(&id, &ref, &settledOn, &rGross, &rFee, &rRef, &rNet, &eNet, &bankTot, &varMin, &st); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
			return
		}

		list = append(list, map[string]any{
			"id":                    id,
			"batch_ref":             ref,
			"settled_on":            settledOn.Format("2006-01-02"),
			"reported_gross_minor":  rGross,
			"reported_fee_minor":    rFee,
			"reported_refund_minor": rRef,
			"reported_net_minor":    rNet,
			"expected_net_minor":    eNet,
			"bank_total_minor":      bankTot,
			"variance_minor":        varMin,
			"status":                st,
		})
	}

	if list == nil {
		list = []map[string]any{}
	}

	WriteJSON(w, http.StatusOK, list)
}

// GET /api/settlements/{id}: Batch detail
func (h *DataHandler) HandleGetSettlement(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid settlement id", nil)
		return
	}

	var b struct {
		ID        int64
		Ref       string
		SettledOn time.Time
		Currency  string
		RGross    int64
		RFee      int64
		RRefund   int64
		RNet      int64
	}
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, batch_ref, settled_on, currency, reported_gross_minor, reported_fee_minor, reported_refund_minor, reported_net_minor
		FROM settlement_batches WHERE id = $1
	`, id).Scan(&b.ID, &b.Ref, &b.SettledOn, &b.Currency, &b.RGross, &b.RFee, &b.RRefund, &b.RNet)

	if errors.Is(err, pgx.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "not_found", "settlement batch not found", nil)
		return
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	// Get recon result
	var reconRes map[string]any
	var eGross, eFee, eRef, eNet, bankTot, varMin int64
	var st string
	var asOf time.Time
	err = h.pool.QueryRow(r.Context(), `
		SELECT as_of, status, expected_gross_minor, expected_fee_minor, expected_refund_minor, expected_net_minor, bank_total_minor, variance_minor
		FROM recon_results WHERE batch_id = $1
	`, id).Scan(&asOf, &st, &eGross, &eFee, &eRef, &eNet, &bankTot, &varMin)
	if err == nil {
		reconRes = map[string]any{
			"as_of":                 asOf.Format("2006-01-02"),
			"status":                st,
			"expected_gross_minor":  eGross,
			"expected_fee_minor":    eFee,
			"expected_refund_minor": eRef,
			"expected_net_minor":    eNet,
			"bank_total_minor":      bankTot,
			"variance_minor":        varMin,
		}
	}

	// Linked deposits via matches
	mRows, _ := h.pool.Query(r.Context(), `
		SELECT d.id, d.bank_txn_id, d.amount_minor, d.posted_on, d.narration, m.applied_amount_minor, m.method
		FROM matches m
		JOIN bank_deposits d ON m.deposit_id = d.id
		WHERE m.batch_id = $1
	`, id)
	defer mRows.Close()

	var linkedDeposits []map[string]any
	for mRows.Next() {
		var dID, amt, applied int64
		var txnID, narr, method string
		var posted time.Time
		mRows.Scan(&dID, &txnID, &amt, &posted, &narr, &applied, &method)
		linkedDeposits = append(linkedDeposits, map[string]any{
			"deposit_id":           dID,
			"bank_txn_id":          txnID,
			"amount_minor":         amt,
			"posted_on":            posted.Format("2006-01-02"),
			"narration":            narr,
			"applied_amount_minor": applied,
			"method":               method,
		})
	}
	if linkedDeposits == nil {
		linkedDeposits = []map[string]any{}
	}

	// Exceptions on this batch
	exRows, _ := h.pool.Query(r.Context(), `
		SELECT id, type, reason_code, severity, status, variance_minor
		FROM exceptions WHERE batch_id = $1
	`, id)
	defer exRows.Close()

	var batchExceptions []map[string]any
	for exRows.Next() {
		var exID, varM int64
		var typ, rc, sev, stat string
		exRows.Scan(&exID, &typ, &rc, &sev, &stat, &varM)
		batchExceptions = append(batchExceptions, map[string]any{
			"id":             exID,
			"type":           typ,
			"reason_code":    rc,
			"severity":       sev,
			"status":         stat,
			"variance_minor": varM,
		})
	}
	if batchExceptions == nil {
		batchExceptions = []map[string]any{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"id":                    b.ID,
		"batch_ref":             b.Ref,
		"settled_on":            b.SettledOn.Format("2006-01-02"),
		"currency":              b.Currency,
		"reported_gross_minor":  b.RGross,
		"reported_fee_minor":    b.RFee,
		"reported_refund_minor": b.RRefund,
		"reported_net_minor":    b.RNet,
		"recon_result":          reconRes,
		"linked_deposits":       linkedDeposits,
		"exceptions":            batchExceptions,
	})
}

// GET /api/exceptions: List exceptions with filters
func (h *DataHandler) HandleListExceptions(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")
	severityFilter := r.URL.Query().Get("severity")

	query := `
		SELECT e.id, e.batch_id, b.batch_ref, b.settled_on,
		       e.type, e.reason_code, e.severity, e.status, e.variance_minor,
		       e.opened_at, e.updated_at, e.last_as_of
		FROM exceptions e
		JOIN settlement_batches b ON e.batch_id = b.id
		WHERE 1=1
	`
	var args []any
	argIdx := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND e.status = $%d", argIdx)
		args = append(args, statusFilter)
		argIdx++
	}
	if typeFilter != "" {
		query += fmt.Sprintf(" AND e.type = $%d", argIdx)
		args = append(args, typeFilter)
		argIdx++
	}
	if severityFilter != "" {
		query += fmt.Sprintf(" AND e.severity = $%d", argIdx)
		args = append(args, severityFilter)
		argIdx++
	}

	query += " ORDER BY e.opened_at ASC"

	rows, err := h.pool.Query(r.Context(), query, args...)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	defer rows.Close()

	var list []map[string]any
	for rows.Next() {
		var id, bID, varM int64
		var bRef, typ, rc, sev, st string
		var settledOn, openedAt, updatedAt, lastAsOf time.Time

		if err := rows.Scan(&id, &bID, &bRef, &settledOn, &typ, &rc, &sev, &st, &varM, &openedAt, &updatedAt, &lastAsOf); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
			return
		}

		list = append(list, map[string]any{
			"id":             id,
			"batch_id":       bID,
			"batch_ref":      bRef,
			"settled_on":     settledOn.Format("2006-01-02"),
			"type":           typ,
			"reason_code":    rc,
			"severity":       sev,
			"status":         st,
			"variance_minor": varM,
			"opened_at":      openedAt.Format(time.RFC3339),
			"updated_at":     updatedAt.Format(time.RFC3339),
			"last_as_of":     lastAsOf.Format("2006-01-02"),
		})
	}

	if list == nil {
		list = []map[string]any{}
	}

	WriteJSON(w, http.StatusOK, list)
}

// GET /api/exceptions/{id}: Exception detail with context JSON and remediation
func (h *DataHandler) HandleGetException(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid exception id", nil)
		return
	}

	var ex struct {
		ID        int64
		BatchID   int64
		BatchRef  string
		SettledOn time.Time
		Type      string
		RC        string
		Severity  string
		Status    string
		Variance  int64
		Context   []byte
		OpenedAt  time.Time
		UpdatedAt time.Time
		LastAsOf  time.Time
	}

	err = h.pool.QueryRow(r.Context(), `
		SELECT e.id, e.batch_id, b.batch_ref, b.settled_on,
		       e.type, e.reason_code, e.severity, e.status, e.variance_minor, e.context,
		       e.opened_at, e.updated_at, e.last_as_of
		FROM exceptions e
		JOIN settlement_batches b ON e.batch_id = b.id
		WHERE e.id = $1
	`, id).Scan(&ex.ID, &ex.BatchID, &ex.BatchRef, &ex.SettledOn, &ex.Type, &ex.RC, &ex.Severity, &ex.Status, &ex.Variance, &ex.Context, &ex.OpenedAt, &ex.UpdatedAt, &ex.LastAsOf)

	if errors.Is(err, pgx.ErrNoRows) {
		WriteError(w, http.StatusNotFound, "not_found", "exception not found", nil)
		return
	} else if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	var ctxObj any
	_ = json.Unmarshal(ex.Context, &ctxObj)

	// Fetch latest workflow run id if any
	var latestWorkflowID *int64
	var wfID int64
	err = h.pool.QueryRow(r.Context(), `
		SELECT id FROM workflow_runs WHERE exception_id = $1 ORDER BY attempt DESC LIMIT 1
	`, id).Scan(&wfID)
	if err == nil {
		latestWorkflowID = &wfID
	}

	// Fetch remediation if any
	var remediationObj map[string]any
	var rem struct {
		ID             int64
		ActionType     string
		Payload        []byte
		Rationale      string
		EvidenceIDs    []string
		Status         string
		IdempotencyKey string
		DecidedBy      *string
		DecidedAt      *time.Time
		DecisionNote   *string
		ExternalRef    *string
		ExecutedAt     *time.Time
		LastError      *string
	}
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, action_type, payload, rationale, evidence_ids, status, idempotency_key,
		       decided_by, decided_at, decision_note, external_ref, executed_at, last_error
		FROM remediations
		WHERE exception_id = $1
		ORDER BY id DESC LIMIT 1
	`, id).Scan(&rem.ID, &rem.ActionType, &rem.Payload, &rem.Rationale, &rem.EvidenceIDs, &rem.Status, &rem.IdempotencyKey,
		&rem.DecidedBy, &rem.DecidedAt, &rem.DecisionNote, &rem.ExternalRef, &rem.ExecutedAt, &rem.LastError)

	if err == nil {
		var payload any
		json.Unmarshal(rem.Payload, &payload)
		remediationObj = map[string]any{
			"id":              rem.ID,
			"action_type":     rem.ActionType,
			"payload":         payload,
			"rationale":       rem.Rationale,
			"evidence_ids":    rem.EvidenceIDs,
			"status":          rem.Status,
			"idempotency_key": rem.IdempotencyKey,
			"decided_by":      rem.DecidedBy,
			"decided_at":      rem.DecidedAt,
			"decision_note":   rem.DecisionNote,
			"external_ref":    rem.ExternalRef,
			"executed_at":     rem.ExecutedAt,
			"last_error":      rem.LastError,
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"id":             ex.ID,
		"batch_id":       ex.BatchID,
		"type":           ex.Type,
		"reason_code":    ex.RC,
		"severity":       ex.Severity,
		"status":         ex.Status,
		"variance_minor": ex.Variance,
		"context":        ctxObj,
		"batch": map[string]any{
			"id":         ex.BatchID,
			"batch_ref":  ex.BatchRef,
			"settled_on": ex.SettledOn.Format("2006-01-02"),
		},
		"latest_workflow_id": latestWorkflowID,
		"remediation":        remediationObj,
		"opened_at":          ex.OpenedAt.Format(time.RFC3339),
		"updated_at":         ex.UpdatedAt.Format(time.RFC3339),
		"last_as_of":         ex.LastAsOf.Format("2006-01-02"),
	})
}
