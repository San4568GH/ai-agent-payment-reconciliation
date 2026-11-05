package ingest_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/internal/ingest"
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

func TestImporter_RepeatImportIdempotency(t *testing.T) {
	pool := setupTestDB(t)
	importer := ingest.NewImporter(pool)
	ctx := context.Background()

	csvData := `order_ref,amount,currency,status,ordered_at
ORD-T1,1000.00,INR,captured,2026-10-01 10:00:00
ORD-T2,2000.00,INR,captured,2026-10-01 10:05:00
`

	// 1st import -> completed, 2 inserted
	res1, err := importer.ImportFile(ctx, ingest.KindOrders, "test_orders.csv", strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	if res1.Status != "completed" || res1.Inserted != 2 || res1.SkippedDuplicates != 0 {
		t.Errorf("unexpected 1st import result: %+v", res1)
	}

	// 2nd import of identical file -> duplicate, 0 inserted
	res2, err := importer.ImportFile(ctx, ingest.KindOrders, "test_orders.csv", strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if res2.Status != "duplicate" || res2.Inserted != 0 {
		t.Errorf("unexpected 2nd import result (expected duplicate): %+v", res2)
	}
}

func TestImporter_InvalidRowRollback(t *testing.T) {
	pool := setupTestDB(t)
	importer := ingest.NewImporter(pool)
	ctx := context.Background()

	// Row 1 is valid, Row 2 has >2 decimal places (10.505)
	csvData := `order_ref,amount,currency,status,ordered_at
ORD-GOOD,1000.00,INR,captured,2026-10-01 10:00:00
ORD-BAD,10.505,INR,captured,2026-10-01 10:05:00
`

	res, err := importer.ImportFile(ctx, ingest.KindOrders, "bad_orders.csv", strings.NewReader(csvData))
	if err == nil {
		t.Fatalf("expected validation error, got success: %+v", res)
	}

	var valErr *ingest.ValidationError
	if !strings.Contains(err.Error(), "invalid rows") {
		t.Errorf("expected validation error message, got: %v", err)
	}
	_ = valErr

	// Verify that ORD-GOOD was NOT inserted due to rollback
	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM orders WHERE order_ref_norm = 'ORDGOOD'").Scan(&count)
	if err != nil {
		t.Fatalf("query orders count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 orders inserted after transaction rollback, got %d", count)
	}
}

func TestImporter_OverlappingFileSkip(t *testing.T) {
	pool := setupTestDB(t)
	importer := ingest.NewImporter(pool)
	ctx := context.Background()

	file1 := `order_ref,amount,currency,status,ordered_at
ORD-O1,1500.00,INR,captured,2026-10-01 10:00:00
`
	// file2 has different hash because it includes ORD-O2
	file2 := `order_ref,amount,currency,status,ordered_at
ORD-O1,1500.00,INR,captured,2026-10-01 10:00:00
ORD-O2,2500.00,INR,captured,2026-10-01 10:10:00
`

	res1, err := importer.ImportFile(ctx, ingest.KindOrders, "file1.csv", strings.NewReader(file1))
	if err != nil {
		t.Fatalf("import file1 failed: %v", err)
	}
	if res1.Inserted != 1 {
		t.Errorf("expected 1 inserted from file1, got %d", res1.Inserted)
	}

	res2, err := importer.ImportFile(ctx, ingest.KindOrders, "file2.csv", strings.NewReader(file2))
	if err != nil {
		t.Fatalf("import file2 failed: %v", err)
	}
	// ORD-O1 skipped via ON CONFLICT DO NOTHING, ORD-O2 inserted
	if res2.Inserted != 1 || res2.SkippedDuplicates != 1 {
		t.Errorf("expected 1 inserted, 1 skipped for file2, got: %+v", res2)
	}
}

