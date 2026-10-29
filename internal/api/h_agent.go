package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/remediation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentHandler struct {
	pool        *pgxpool.Pool
	agentEngine *agent.Engine
	remService  *remediation.Service
}

func NewAgentHandler(pool *pgxpool.Pool, agentEngine *agent.Engine, remService *remediation.Service) *AgentHandler {
	return &AgentHandler{
		pool:        pool,
		agentEngine: agentEngine,
		remService:  remService,
	}
}

// POST /api/exceptions/{id}/investigate
func (h *AgentHandler) HandleInvestigate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	excID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid exception id", nil)
		return
	}

	// Check if synchronous execution requested (for test/scripted convenience)
	sync := r.URL.Query().Get("sync") == "true"

	if sync {
		run, err := h.agentEngine.Investigate(r.Context(), excID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "investigation_failed", err.Error(), nil)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":   "completed",
			"workflow": run,
		})
		return
	}

	// Asynchronous execution
	go func() {
		_, _ = h.agentEngine.Investigate(r.Context(), excID)
	}()

	WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":       "started",
		"exception_id": excID,
		"message":      "Investigation initiated in background",
	})
}

// POST /api/exceptions/{id}/escalate
func (h *AgentHandler) HandleEscalate(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	excID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid exception id", nil)
		return
	}

	actor := ActorFromContext(r.Context())

	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "Manual escalation by operator"
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	defer tx.Rollback(r.Context())

	// Rule: Escalate is allowed only if nothing has executed, and marks any proposed remediation as rejected
	var executedCount int
	err = tx.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM remediations WHERE exception_id = $1 AND status = 'executed'
	`, excID).Scan(&executedCount)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	if executedCount > 0 {
		WriteError(w, http.StatusConflict, "invalid_state", "cannot escalate exception after remediation has already executed", nil)
		return
	}

	// Update exception to escalated
	res, err := tx.Exec(r.Context(), `
		UPDATE exceptions
		SET status = 'escalated', updated_at = now()
		WHERE id = $1
	`, excID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	if res.RowsAffected() == 0 {
		WriteError(w, http.StatusNotFound, "not_found", "exception not found", nil)
		return
	}

	// Mark any proposed remediation as rejected
	_, _ = tx.Exec(r.Context(), `
		UPDATE remediations
		SET status = 'rejected', decided_by = $1, decided_at = now(), decision_note = 'Auto-rejected on manual exception escalation'
		WHERE exception_id = $2 AND status = 'proposed'
	`, actor, excID)

	// Audit event
	auditDetails, _ := json.Marshal(map[string]any{"reason": req.Reason})
	_, _ = tx.Exec(r.Context(), `
		INSERT INTO audit_events (exception_id, actor, event_type, details, created_at)
		VALUES ($1, $2, 'exception_escalated', $3, now())
	`, excID, actor, auditDetails)

	if err := tx.Commit(r.Context()); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "escalated",
		"exception_id": excID,
	})
}

// GET /api/workflows/{id}
func (h *AgentHandler) HandleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	runID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid workflow run id", nil)
		return
	}

	var run agent.WorkflowRun
	var conclusionBytes []byte
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, exception_id, attempt, status, tool_calls, llm_turns, conclusion, error, started_at, finished_at
		FROM workflow_runs
		WHERE id = $1
	`, runID).Scan(
		&run.ID, &run.ExceptionID, &run.Attempt, &run.Status, &run.ToolCalls, &run.LLMTurns,
		&conclusionBytes, &run.Error, &run.StartedAt, &run.FinishedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			WriteError(w, http.StatusNotFound, "not_found", "workflow run not found", nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	run.Conclusion = conclusionBytes

	// Fetch ordered steps
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, seq, kind, tool_name, args, result, summary, status, error, duration_ms, evidence_ids, created_at
		FROM workflow_steps
		WHERE run_id = $1
		ORDER BY seq ASC
	`, runID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	defer rows.Close()

	var steps []map[string]any
	for rows.Next() {
		var sID int64
		var seq, durMs int
		var kind, toolName, summary, status string
		var argsBytes, resBytes []byte
		var errStr *string
		var evidenceIDs []string
		var createdAt time.Time

		if err := rows.Scan(&sID, &seq, &kind, &toolName, &argsBytes, &resBytes, &summary, &status, &errStr, &durMs, &evidenceIDs, &createdAt); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
			return
		}

		var argsMap, resMap any
		_ = json.Unmarshal(argsBytes, &argsMap)
		_ = json.Unmarshal(resBytes, &resMap)

		steps = append(steps, map[string]any{
			"id":           sID,
			"seq":          seq,
			"kind":         kind,
			"tool_name":    toolName,
			"args":         argsMap,
			"result":       resMap,
			"summary":      summary,
			"status":       status,
			"error":        errStr,
			"duration_ms":  durMs,
			"evidence_ids": evidenceIDs,
			"created_at":   createdAt,
		})
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"workflow": run,
		"steps":    steps,
	})
}

// POST /api/remediations/{id}/approve
func (h *AgentHandler) HandleApproveRemediation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	remID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid remediation id", nil)
		return
	}

	actor := ActorFromContext(r.Context())

	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.remService.ApproveRemediation(r.Context(), remID, actor, req.Note); err != nil {
		if errors.Is(err, remediation.ErrInvalidState) {
			WriteError(w, http.StatusConflict, "invalid_state", err.Error(), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	// Automatically attempt execution upon approval
	execErr := h.remService.ExecuteRemediation(r.Context(), remID, actor)

	// Fetch updated remediation
	var status string
	var lastErr *string
	_ = h.pool.QueryRow(r.Context(), "SELECT status, last_error FROM remediations WHERE id = $1", remID).Scan(&status, &lastErr)

	resp := map[string]any{
		"id":     remID,
		"status": status,
	}
	if execErr != nil {
		resp["execution_error"] = execErr.Error()
	}

	WriteJSON(w, http.StatusOK, resp)
}

// POST /api/remediations/{id}/reject
func (h *AgentHandler) HandleRejectRemediation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	remID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid remediation id", nil)
		return
	}

	actor := ActorFromContext(r.Context())

	var req struct {
		Note string `json:"note"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if err := h.remService.RejectRemediation(r.Context(), remID, actor, req.Note); err != nil {
		if errors.Is(err, remediation.ErrInvalidState) {
			WriteError(w, http.StatusConflict, "invalid_state", err.Error(), nil)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"id":     remID,
		"status": "rejected",
	})
}

// POST /api/remediations/{id}/execute: retry-only or manual execute
func (h *AgentHandler) HandleExecuteRemediation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	remID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "validation_error", "invalid remediation id", nil)
		return
	}

	actor := ActorFromContext(r.Context())

	if err := h.remService.ExecuteRemediation(r.Context(), remID, actor); err != nil {
		if errors.Is(err, remediation.ErrInvalidState) {
			WriteError(w, http.StatusConflict, "invalid_state", err.Error(), nil)
			return
		}
		// Return 200 with error details or 502 Bad Gateway
		WriteJSON(w, http.StatusOK, map[string]any{
			"id":     remID,
			"status": "execution_failed",
			"error":  err.Error(),
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"id":     remID,
		"status": "executed",
	})
}

// GET /api/audit-events
func (h *AgentHandler) HandleListAuditEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, exception_id, remediation_id, actor, event_type, details, created_at
		FROM audit_events
		ORDER BY created_at DESC
		LIMIT 100
	`)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var id int64
		var excID, remID *int64
		var actor, eventType string
		var detailsBytes []byte
		var createdAt time.Time

		if err := rows.Scan(&id, &excID, &remID, &actor, &eventType, &detailsBytes, &createdAt); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
			return
		}

		var detailsMap any
		_ = json.Unmarshal(detailsBytes, &detailsMap)

		events = append(events, map[string]any{
			"id":             id,
			"exception_id":   excID,
			"remediation_id": remID,
			"actor":          actor,
			"event_type":     eventType,
			"details":        detailsMap,
			"created_at":     createdAt,
		})
	}

	WriteJSON(w, http.StatusOK, events)
}
