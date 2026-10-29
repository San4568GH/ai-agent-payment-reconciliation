package db_test

import (
	"strings"
	"testing"

	"github.com/hackathon/settlement-agent/migrations"
)

func TestMigrationFilesExist(t *testing.T) {
	entries, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatalf("failed to read embedded migrations dir: %v", err)
	}

	found := false
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sql") {
			found = true
			content, err := migrations.FS.ReadFile(entry.Name())
			if err != nil {
				t.Fatalf("failed to read migration file %s: %v", entry.Name(), err)
			}
			sqlText := string(content)
			if !strings.Contains(sqlText, "-- +goose Up") {
				t.Errorf("migration %s missing '-- +goose Up'", entry.Name())
			}
			if !strings.Contains(sqlText, "-- +goose Down") {
				t.Errorf("migration %s missing '-- +goose Down'", entry.Name())
			}

			// Verify all 15 tables are mentioned in Up section
			tables := []string{
				"import_batches", "orders", "order_refunds", "gateway_payments",
				"gateway_refunds", "settlement_batches", "bank_deposits",
				"gateway_fee_schedules", "recon_results", "matches",
				"exceptions", "workflow_runs", "workflow_steps",
				"remediations", "audit_events",
			}
			for _, tbl := range tables {
				if !strings.Contains(sqlText, "CREATE TABLE IF NOT EXISTS "+tbl) {
					t.Errorf("migration %s missing table %s", entry.Name(), tbl)
				}
			}
		}
	}

	if !found {
		t.Fatal("no .sql migration files found in embedded filesystem")
	}
}
