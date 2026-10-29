package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/db"
)

type RouterOptions struct {
	Config   *config.Config
	Database *db.DB
}

func NewRouter(opts RouterOptions) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// Public Health Check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if opts.Database == nil || opts.Database.Pool == nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "error",
				"db":     "disconnected",
				"error":  "database not initialized",
			})
			return
		}

		ctxPing, cancelPing := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancelPing()

		if err := opts.Database.Ping(ctxPing); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "error",
				"db":     "disconnected",
				"error":  err.Error(),
			})
			return
		}

		ver, _ := db.Version(opts.Database.Pool)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"status":        "ok",
			"db":            "connected",
			"goose_version": ver,
			"timestamp":     time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Authenticated API routes
	r.Route("/api", func(apiRoute chi.Router) {
		apiRoute.Use(AuthMiddleware(opts.Config))

		if opts.Database != nil && opts.Database.Pool != nil {
			dataHandler := NewDataHandler(opts.Database.Pool, opts.Config)

			// Viewer routes (GET)
			apiRoute.Get("/imports", dataHandler.HandleListImports)
			apiRoute.Get("/summary", dataHandler.HandleSummary)
			apiRoute.Get("/settlements", dataHandler.HandleListSettlements)
			apiRoute.Get("/settlements/{id}", dataHandler.HandleGetSettlement)
			apiRoute.Get("/exceptions", dataHandler.HandleListExceptions)
			apiRoute.Get("/exceptions/{id}", dataHandler.HandleGetException)

			// Operator routes (POST)
			apiRoute.Group(func(opRoute chi.Router) {
				opRoute.Use(RequireOperator)
				opRoute.Post("/imports", dataHandler.HandleImport)
				opRoute.Post("/imports/seed", dataHandler.HandleSeedImport)
				opRoute.Post("/settlement-runs", dataHandler.HandleSettlementRun)
			})
		}
	})

	return r
}
