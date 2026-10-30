package agent_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/hackathon/settlement-agent/internal/recon"
	"github.com/hackathon/settlement-agent/internal/remediation"
	"github.com/hackathon/settlement-agent/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping integration test")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Connect(ctx, dbURL)
	if err != nil {
		t.Skipf("cannot connect to TEST_DATABASE_URL (%s): %v; skipping", dbURL, err)
		return nil
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

	return database.Pool
}

func loadSeedAndRunRecon(t *testing.T, pool *pgxpool.Pool) {
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
			t.Fatalf("import seed file %s unexpected status: %s", filename, res.Status)
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

func getExceptionID(t *testing.T, pool *pgxpool.Pool, batchRef string) int64 {
	t.Helper()
	var excID int64
	err := pool.QueryRow(context.Background(), `
		SELECT e.id
		FROM exceptions e
		JOIN settlement_batches sb ON e.batch_id = sb.id
		WHERE sb.batch_ref = $1
	`, batchRef).Scan(&excID)
	if err != nil {
		t.Fatalf("find exception for %s: %v", batchRef, err)
	}
	return excID
}

func TestEngine_FeeVariance_ProposalReady(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	excID := getExceptionID(t, pool, "STL-20261012-F")

	ctx := context.Background()
	run, err := engine.Investigate(ctx, excID)
	if err != nil {
		t.Fatalf("investigate failed: %v", err)
	}

	if run.Status != "completed" {
		t.Errorf("expected run status 'completed', got '%s'", run.Status)
	}
	if run.ToolCalls < 2 || run.ToolCalls > 6 {
		t.Errorf("unexpected tool call count: %d", run.ToolCalls)
	}

	// Check exception status in DB
	var excStatus string
	pool.QueryRow(ctx, "SELECT status FROM exceptions WHERE id = $1", excID).Scan(&excStatus)
	if excStatus != "proposal_ready" {
		t.Errorf("expected exception status 'proposal_ready', got '%s'", excStatus)
	}

	// Check remediation drafted
	var remStatus, actionType string
	err = pool.QueryRow(ctx, "SELECT status, action_type FROM remediations WHERE exception_id = $1", excID).Scan(&remStatus, &actionType)
	if err != nil {
		t.Fatalf("query remediation: %v", err)
	}
	if remStatus != "proposed" || actionType != "fee_inquiry" {
		t.Errorf("expected proposed fee_inquiry remediation, got status=%s, action=%s", remStatus, actionType)
	}

	// Verify workflow steps trace recorded
	var stepCount int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM workflow_steps WHERE run_id = $1", run.ID).Scan(&stepCount)
	if stepCount < 2 {
		t.Errorf("expected at least 2 workflow steps, got %d", stepCount)
	}
}

func TestEngine_SplitDeposit_ProposalReady(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	excID := getExceptionID(t, pool, "STL-20261013-S")

	ctx := context.Background()
	run, err := engine.Investigate(ctx, excID)
	if err != nil {
		t.Fatalf("investigate failed: %v", err)
	}

	if run.Status != "completed" {
		t.Errorf("expected run status 'completed', got '%s'", run.Status)
	}

	var remStatus, actionType string
	err = pool.QueryRow(ctx, "SELECT status, action_type FROM remediations WHERE exception_id = $1", excID).Scan(&remStatus, &actionType)
	if err != nil {
		t.Fatalf("query remediation: %v", err)
	}
	if remStatus != "proposed" || actionType != "settlement_link" {
		t.Errorf("expected proposed settlement_link remediation, got status=%s, action=%s", remStatus, actionType)
	}
}

func TestEngine_RefundNotInLedger_ProposalReady(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	excID := getExceptionID(t, pool, "STL-20261014-R")

	ctx := context.Background()
	run, err := engine.Investigate(ctx, excID)
	if err != nil {
		t.Fatalf("investigate failed: %v", err)
	}

	if run.Status != "completed" {
		t.Errorf("expected run status 'completed', got '%s'", run.Status)
	}

	var remStatus, actionType string
	err = pool.QueryRow(ctx, "SELECT status, action_type FROM remediations WHERE exception_id = $1", excID).Scan(&remStatus, &actionType)
	if err != nil {
		t.Fatalf("query remediation: %v", err)
	}
	if remStatus != "proposed" || actionType != "journal_entry_draft" {
		t.Errorf("expected proposed journal_entry_draft remediation, got status=%s, action=%s", remStatus, actionType)
	}
}

func TestEngine_MissingDeposit_Escalated(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	excID := getExceptionID(t, pool, "STL-20261015-M")

	ctx := context.Background()
	run, err := engine.Investigate(ctx, excID)
	if err != nil {
		t.Fatalf("investigate failed: %v", err)
	}

	if run.Status != "escalated" {
		t.Errorf("expected run status 'escalated', got '%s'", run.Status)
	}

	var excStatus string
	pool.QueryRow(ctx, "SELECT status FROM exceptions WHERE id = $1", excID).Scan(&excStatus)
	if excStatus != "escalated" {
		t.Errorf("expected exception status 'escalated', got '%s'", excStatus)
	}
}

func TestEngine_StaleRunsCleanup(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedAndRunRecon(t, pool)

	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)
	remService := remediation.NewService(pool, gwMock, acctMock)
	fakeLLM := agent.NewFakeLLM()

	engine := agent.NewEngine(pool, fakeLLM, gwMock, acctMock, remService)
	excID := getExceptionID(t, pool, "STL-20261012-F")
	ctx := context.Background()

	// Simulate a crashed run: insert a run with 'running' and set exception to 'investigating'
	var runID int64
	err := pool.QueryRow(ctx, `
		INSERT INTO workflow_runs (exception_id, attempt, status, started_at)
		VALUES ($1, 1, 'running', now())
		RETURNING id
	`, excID).Scan(&runID)
	if err != nil {
		t.Fatalf("insert crashed run: %v", err)
	}

	_, err = pool.Exec(ctx, "UPDATE exceptions SET status = 'investigating' WHERE id = $1", excID)
	if err != nil {
		t.Fatalf("update exception investigating: %v", err)
	}

	// Simulate a remediation stuck in 'executing'
	var remID int64
	err = pool.QueryRow(ctx, `
		INSERT INTO remediations (exception_id, run_id, action_type, payload, rationale, status, idempotency_key, created_at)
		VALUES ($1, $2, 'fee_inquiry', '{}', 'test crash', 'executing', 'test-crash-idemp', now())
		RETURNING id
	`, excID, runID).Scan(&remID)
	if err != nil {
		t.Fatalf("insert crashed remediation: %v", err)
	}

	// Run cleanup
	if err := engine.CleanupStaleRuns(ctx); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	// Verify run is marked 'failed'
	var runStatus string
	pool.QueryRow(ctx, "SELECT status FROM workflow_runs WHERE id = $1", runID).Scan(&runStatus)
	if runStatus != "failed" {
		t.Errorf("expected run status 'failed', got '%s'", runStatus)
	}

	// Verify exception is reset to 'open'
	var excStatus string
	pool.QueryRow(ctx, "SELECT status FROM exceptions WHERE id = $1", excID).Scan(&excStatus)
	if excStatus != "open" {
		t.Errorf("expected exception status 'open', got '%s'", excStatus)
	}

	// Verify remediation is marked 'execution_failed'
	var remStatus string
	pool.QueryRow(ctx, "SELECT status FROM remediations WHERE id = $1", remID).Scan(&remStatus)
	if remStatus != "execution_failed" {
		t.Errorf("expected remediation status 'execution_failed', got '%s'", remStatus)
	}
}

