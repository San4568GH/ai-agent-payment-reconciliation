package recon_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/hackathon/settlement-agent/internal/recon"
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

func loadSeedData(t *testing.T, pool *pgxpool.Pool) {
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
}

func TestGoldenScenario_Reconciliation(t *testing.T) {
	pool := setupTestDB(t)
	loadSeedData(t, pool)

	ctx := context.Background()
	reconStore := recon.NewStore(pool)
	engine := recon.NewEngine()

	asOf, err := time.Parse("2006-01-02", "2026-10-20")
	if err != nil {
		t.Fatalf("parse as_of date: %v", err)
	}

	// First Reconciliation Run
	input, err := reconStore.LoadInput(ctx, asOf, 2)
	if err != nil {
		t.Fatalf("LoadInput failed: %v", err)
	}
	if len(input.Batches) != 6 {
		t.Fatalf("expected 6 settlement batches, got %d", len(input.Batches))
	}

	output := engine.Run(input)
	if err := reconStore.SaveOutput(ctx, output); err != nil {
		t.Fatalf("SaveOutput failed: %v", err)
	}

	matchedCount := 0
	exceptionsCount := 0
	for _, br := range output.BatchResults {
		if br.Status == "matched" {
			matchedCount++
		}
		if br.Status == "exception" {
			exceptionsCount++
		}
	}

	// Assertions for Run 1
	if matchedCount != 2 {
		t.Errorf("expected 2 matched batches, got %d", matchedCount)
	}
	if exceptionsCount != 4 {
		t.Errorf("expected 4 batches with exceptions, got %d", exceptionsCount)
	}

	// Verify the 2 matched batches in DB
	for _, matchedRef := range []string{"STL-20261005-A", "STL-20261006-B"} {
		var status string
		err := pool.QueryRow(ctx, `
			SELECT rr.status
			FROM recon_results rr
			JOIN settlement_batches sb ON rr.batch_id = sb.id
			WHERE sb.batch_ref = $1
		`, matchedRef).Scan(&status)
		if err != nil {
			t.Errorf("query recon_result for %s: %v", matchedRef, err)
		} else if status != "matched" {
			t.Errorf("batch %s expected status 'matched', got '%s'", matchedRef, status)
		}
	}

	// Verify the 4 exceptions and exact variances
	expectedExceptions := map[string]struct {
		excType  string
		variance int64
	}{
		"STL-20261012-F": {excType: "FEE_VARIANCE", variance: -100000},
		"STL-20261013-S": {excType: "DELAYED_OR_SPLIT_DEPOSIT", variance: -3800000},
		"STL-20261014-R": {excType: "REFUND_NOT_IN_LEDGER", variance: -500000},
		"STL-20261015-M": {excType: "MISSING_DEPOSIT", variance: -5880000},
	}

	for batchRef, exp := range expectedExceptions {
		var excType string
		var varianceMinor int64
		var status string
		err := pool.QueryRow(ctx, `
			SELECT e.type, e.variance_minor, e.status
			FROM exceptions e
			JOIN settlement_batches sb ON e.batch_id = sb.id
			WHERE sb.batch_ref = $1
		`, batchRef).Scan(&excType, &varianceMinor, &status)
		if err != nil {
			t.Errorf("query exception for %s: %v", batchRef, err)
			continue
		}

		if excType != exp.excType {
			t.Errorf("[%s] expected exception type %s, got %s", batchRef, exp.excType, excType)
		}
		if varianceMinor != exp.variance {
			t.Errorf("[%s] expected variance %d paise, got %d paise", batchRef, exp.variance, varianceMinor)
		}
		if status != "open" {
			t.Errorf("[%s] expected status 'open', got %s", batchRef, status)
		}
	}

	// Verify Decoy deposit DEP-9999 is NOT matched
	var decoyMatches int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM matches m
		JOIN bank_deposits bd ON m.deposit_id = bd.id
		WHERE bd.bank_txn_id = 'DEP-9999'
	`).Scan(&decoyMatches)
	if err != nil {
		t.Fatalf("query decoy matches: %v", err)
	}
	if decoyMatches != 0 {
		t.Errorf("decoy deposit DEP-9999 should not be matched, but found %d matches", decoyMatches)
	}

	// Count total exceptions in DB before re-run
	var totalExceptionsBefore int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM exceptions").Scan(&totalExceptionsBefore); err != nil {
		t.Fatalf("count exceptions before re-run: %v", err)
	}
	if totalExceptionsBefore != 4 {
		t.Fatalf("expected 4 total exceptions before re-run, got %d", totalExceptionsBefore)
	}

	// Second Reconciliation Run (Idempotency check)
	input2, err := reconStore.LoadInput(ctx, asOf, 2)
	if err != nil {
		t.Fatalf("re-run LoadInput failed: %v", err)
	}
	output2 := engine.Run(input2)
	if err := reconStore.SaveOutput(ctx, output2); err != nil {
		t.Fatalf("re-run SaveOutput failed: %v", err)
	}

	// Assert 0 new exceptions created
	var totalExceptionsAfter int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM exceptions").Scan(&totalExceptionsAfter); err != nil {
		t.Fatalf("count exceptions after re-run: %v", err)
	}
	if totalExceptionsAfter != 4 {
		t.Errorf("expected 4 total exceptions after re-run (idempotent), got %d", totalExceptionsAfter)
	}
}
