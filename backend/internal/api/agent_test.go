package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/api"
	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/hackathon/settlement-agent/internal/recon"
	"github.com/hackathon/settlement-agent/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestRouter(t *testing.T) (http.Handler, *pgxpool.Pool, *config.Config) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test")
		return nil, nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("cannot connect to TEST_DATABASE_URL (%s): %v; skipping", dbURL, err)
		return nil, nil, nil
	}

	// Run migrations
	if err := db.Migrate(database.Pool, migrations.FS, "."); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	// Truncate tables for clean test
	cleanQuery := `
		TRUNCATE TABLE audit_events, remediations, workflow_steps, workflow_runs,
		               exceptions, matches, recon_results, gateway_fee_schedules,
		               bank_deposits, settlement_batches, gateway_refunds,
		               gateway_payments, order_refunds, orders, import_batches
		CASCADE;
	`
	if _, err := database.Pool.Exec(ctx, cleanQuery); err != nil {
		t.Fatalf("failed to truncate test tables: %v", err)
	}

	cfg := &config.Config{
		OperatorToken:        "test-op-tok",
		ViewerToken:          "test-vw-tok",
		OperatorName:         "Alice",
		SettlementWindowDays: 2,
	}

	r := api.NewRouter(api.RouterOptions{
		Config:   cfg,
		Database: database,
		LLM:      agent.NewFakeLLM(),
	})

	return r, database.Pool, cfg
}

func seedAndReconcile(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	importer := ingest.NewImporter(pool)

	seedDir := "../../seed"
	if _, err := os.Stat(seedDir); os.IsNotExist(err) {
		seedDir = "seed"
	}

	for _, kind := range ingest.ImportOrder {
		filename := ingest.KindFilenames[kind]
		filePath := filepath.Join(seedDir, filename)

		f, err := os.Open(filePath)
		if err != nil {
			t.Fatalf("cannot open seed file %s: %v", filename, err)
		}

		res, err := importer.ImportFile(ctx, kind, filename, f)
		f.Close()
		if err != nil {
			t.Fatalf("import seed file %s failed: %v", filename, err)
		}
		if res.Status != "completed" {
			t.Fatalf("unexpected import status: %s", res.Status)
		}
	}

	reconStore := recon.NewStore(pool)
	engine := recon.NewEngine()
	asOf, _ := time.Parse("2006-01-02", "2026-10-20")

	input, err := reconStore.LoadInput(ctx, asOf, 2)
	if err != nil {
		t.Fatalf("LoadInput failed: %v", err)
	}
	output := engine.Run(input)
	if err := reconStore.SaveOutput(ctx, output); err != nil {
		t.Fatalf("SaveOutput failed: %v", err)
	}
}

func TestAgentEndpoints_FullWorkflow(t *testing.T) {
	router, pool, _ := setupTestRouter(t)
	seedAndReconcile(t, pool)

	ctx := context.Background()

	// Get Fee Variance Exception (STL-20261012-F)
	var excID int64
	err := pool.QueryRow(ctx, `
		SELECT e.id
		FROM exceptions e
		JOIN settlement_batches sb ON e.batch_id = sb.id
		WHERE sb.batch_ref = 'STL-20261012-F'
	`).Scan(&excID)
	if err != nil {
		t.Fatalf("find fee variance exception: %v", err)
	}

	// 1. Viewer cannot trigger investigate (403)
	reqVw := httptest.NewRequest(http.MethodPost, "/api/exceptions/1/investigate", nil)
	reqVw.Header.Set("Authorization", "Bearer test-vw-tok")
	wVw := httptest.NewRecorder()
	router.ServeHTTP(wVw, reqVw)
	if wVw.Code != http.StatusForbidden {
		t.Errorf("viewer expected 403 Forbidden on investigate, got %d", wVw.Code)
	}

	// 2. Operator triggers investigate synchronously
	investigateURL := fmt.Sprintf("/api/exceptions/%d/investigate?sync=true", excID)
	reqOp := httptest.NewRequest(http.MethodPost, investigateURL, nil)
	reqOp.Header.Set("Authorization", "Bearer test-op-tok")
	wOp := httptest.NewRecorder()
	router.ServeHTTP(wOp, reqOp)

	if wOp.Code != http.StatusOK {
		t.Fatalf("operator investigate expected 200 OK, got %d: %s", wOp.Code, wOp.Body.String())
	}

	var invResp struct {
		Status   string `json:"status"`
		Workflow struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"workflow"`
	}
	if err := json.NewDecoder(wOp.Body).Decode(&invResp); err != nil {
		t.Fatalf("decode investigate response: %v", err)
	}
	if invResp.Workflow.Status != "completed" {
		t.Errorf("expected workflow status 'completed', got '%s'", invResp.Workflow.Status)
	}

	// 3. Viewer can view workflow run and steps
	workflowURL := fmt.Sprintf("/api/workflows/%d", invResp.Workflow.ID)
	reqWf := httptest.NewRequest(http.MethodGet, workflowURL, nil)
	reqWf.Header.Set("Authorization", "Bearer test-vw-tok")
	wWf := httptest.NewRecorder()
	router.ServeHTTP(wWf, reqWf)

	if wWf.Code != http.StatusOK {
		t.Fatalf("get workflow expected 200 OK, got %d", wWf.Code)
	}
	var wfResp struct {
		Workflow map[string]any   `json:"workflow"`
		Steps    []map[string]any `json:"steps"`
	}
	json.NewDecoder(wWf.Body).Decode(&wfResp)
	if len(wfResp.Steps) < 2 {
		t.Errorf("expected at least 2 steps, got %d", len(wfResp.Steps))
	}

	// 4. Fetch the created remediation
	var remID int64
	var remStatus string
	err = pool.QueryRow(ctx, "SELECT id, status FROM remediations WHERE exception_id = $1", excID).Scan(&remID, &remStatus)
	if err != nil {
		t.Fatalf("query remediation for exception %d: %v", excID, err)
	}
	if remStatus != "proposed" {
		t.Fatalf("expected proposed remediation, got %s", remStatus)
	}

	// 5. Operator approves remediation
	remURL := fmt.Sprintf("/api/remediations/%d/approve", remID)
	reqApprove := httptest.NewRequest(http.MethodPost, remURL, bytes.NewBufferString(`{"note":"Approved after reviewing gateway contract"}`))
	reqApprove.Header.Set("Authorization", "Bearer test-op-tok")
	reqApprove.Header.Set("Content-Type", "application/json")
	wApprove := httptest.NewRecorder()
	router.ServeHTTP(wApprove, reqApprove)

	if wApprove.Code != http.StatusOK {
		t.Fatalf("approve remediation expected 200 OK, got %d: %s", wApprove.Code, wApprove.Body.String())
	}
	t.Logf("Approve Response: %s", wApprove.Body.String())

	// Verify remediation is executed
	var finalRemStatus string
	pool.QueryRow(ctx, "SELECT status FROM remediations WHERE id = $1", remID).Scan(&finalRemStatus)
	if finalRemStatus != "executed" {
		t.Errorf("expected executed remediation, got %s", finalRemStatus)
	}

	// Verify exception is now action_taken
	var finalExcStatus string
	pool.QueryRow(ctx, "SELECT status FROM exceptions WHERE id = $1", excID).Scan(&finalExcStatus)
	if finalExcStatus != "action_taken" {
		t.Errorf("expected exception status 'action_taken', got %s", finalExcStatus)
	}

	// 6. Viewer checks audit events
	reqAudit := httptest.NewRequest(http.MethodGet, "/api/audit-events", nil)
	reqAudit.Header.Set("Authorization", "Bearer test-vw-tok")
	wAudit := httptest.NewRecorder()
	router.ServeHTTP(wAudit, reqAudit)

	if wAudit.Code != http.StatusOK {
		t.Fatalf("list audit events expected 200 OK, got %d", wAudit.Code)
	}
	var auditList []map[string]any
	json.NewDecoder(wAudit.Body).Decode(&auditList)
	if len(auditList) == 0 {
		t.Errorf("expected audit events to be recorded, got 0")
	}

	// 7. Test Manual Escalation on Missing Deposit Exception (STL-20261015-M)
	var excMID int64
	err = pool.QueryRow(ctx, `
		SELECT e.id
		FROM exceptions e
		JOIN settlement_batches sb ON e.batch_id = sb.id
		WHERE sb.batch_ref = 'STL-20261015-M'
	`).Scan(&excMID)
	if err != nil {
		t.Fatalf("find missing deposit exception: %v", err)
	}

	escalateURL := fmt.Sprintf("/api/exceptions/%d/escalate", excMID)
	reqEsc := httptest.NewRequest(http.MethodPost, escalateURL, bytes.NewBufferString(`{"reason":"Requires banking partner intervention"}`))
	reqEsc.Header.Set("Authorization", "Bearer test-op-tok")
	reqEsc.Header.Set("Content-Type", "application/json")
	wEsc := httptest.NewRecorder()
	router.ServeHTTP(wEsc, reqEsc)

	if wEsc.Code != http.StatusOK {
		t.Fatalf("escalate exception expected 200 OK, got %d: %s", wEsc.Code, wEsc.Body.String())
	}

	var mStatus string
	pool.QueryRow(ctx, "SELECT status FROM exceptions WHERE id = $1", excMID).Scan(&mStatus)
	if mStatus != "escalated" {
		t.Errorf("expected exception status 'escalated', got %s", mStatus)
	}
}
