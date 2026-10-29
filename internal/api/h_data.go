package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/ingest"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DataHandler struct {
	pool     *pgxpool.Pool
	cfg      *config.Config
	importer *ingest.Importer
}

func NewDataHandler(pool *pgxpool.Pool, cfg *config.Config) *DataHandler {
	return &DataHandler{
		pool:     pool,
		cfg:      cfg,
		importer: ingest.NewImporter(pool),
	}
}

// POST /api/imports: multipart form with kind and file
func (h *DataHandler) HandleImport(w http.ResponseWriter, r *http.Request) {
	// Parse multipart up to 32MB
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

type ImportBatchItem struct {
	ID        int64     `json:"id"`
	Kind      string    `json:"kind"`
	Filename  string    `json:"filename"`
	SHA256    string    `json:"sha256"`
	RowsTotal int       `json:"rows_total"`
	Inserted  int       `json:"inserted"`
	Skipped   int       `json:"skipped"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
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

	var batches []ImportBatchItem
	for rows.Next() {
		var b ImportBatchItem
		if err := rows.Scan(&b.ID, &b.Kind, &b.Filename, &b.SHA256, &b.RowsTotal, &b.Inserted, &b.Skipped, &b.Status, &b.CreatedAt); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", fmt.Sprintf("scan import batch: %v", err), nil)
			return
		}
		batches = append(batches, b)
	}

	if batches == nil {
		batches = []ImportBatchItem{}
	}

	WriteJSON(w, http.StatusOK, batches)
}
