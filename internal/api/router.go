package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/internal/mocks"
	"github.com/hackathon/settlement-agent/internal/remediation"
)

type RouterOptions struct {
	Config   *config.Config
	Database *db.DB
	LLM      agent.LLM
}

func NewRouter(opts RouterOptions) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS headers for local React workstation development
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token, Idempotency-Key")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

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

	// Mock Services (Dev / Test / Simulation)
	fi := mocks.NewFailureInjector()
	gwMock := mocks.NewGatewayMock(fi)
	acctMock := mocks.NewAccountingMock(fi)

	r.Route("/mock", func(mockRoute chi.Router) {
		mockRoute.Post("/_admin/fail", fi.HandleAdminFail)
		gwMock.RegisterRoutes(mockRoute)
		acctMock.RegisterRoutes(mockRoute)
	})

	// Authenticated API routes
	r.Route("/api", func(apiRoute chi.Router) {
		apiRoute.Use(AuthMiddleware(opts.Config))

		if opts.Database != nil && opts.Database.Pool != nil {
			dataHandler := NewDataHandler(opts.Database.Pool, opts.Config)

			// LLM selection: explicit > Anthropic Client if key configured > FakeLLM fallback
			llmClient := opts.LLM
			if llmClient == nil && opts.Config != nil && opts.Config.AnthropicAPIKey != "" {
				c, err := agent.NewAnthropicClient(opts.Config.AnthropicAPIKey, opts.Config.LLMModel)
				if err == nil {
					llmClient = c
				}
			}
			if llmClient == nil {
				llmClient = agent.NewFakeLLM()
			}

			remService := remediation.NewService(opts.Database.Pool, gwMock, acctMock)
			agentEngine := agent.NewEngine(opts.Database.Pool, llmClient, gwMock, acctMock, remService)
			agentHandler := NewAgentHandler(opts.Database.Pool, agentEngine, remService)

			// Viewer routes (GET)
			apiRoute.Get("/imports", dataHandler.HandleListImports)
			apiRoute.Get("/summary", dataHandler.HandleSummary)
			apiRoute.Get("/settlements", dataHandler.HandleListSettlements)
			apiRoute.Get("/settlements/{id}", dataHandler.HandleGetSettlement)
			apiRoute.Get("/exceptions", dataHandler.HandleListExceptions)
			apiRoute.Get("/exceptions/{id}", dataHandler.HandleGetException)
			apiRoute.Get("/workflows/{id}", agentHandler.HandleGetWorkflow)
			apiRoute.Get("/audit-events", agentHandler.HandleListAuditEvents)

			// Operator routes (POST)
			apiRoute.Group(func(opRoute chi.Router) {
				opRoute.Use(RequireOperator)
				opRoute.Post("/imports", dataHandler.HandleImport)
				opRoute.Post("/imports/seed", dataHandler.HandleSeedImport)
				opRoute.Post("/settlement-runs", dataHandler.HandleSettlementRun)
				opRoute.Post("/exceptions/{id}/investigate", agentHandler.HandleInvestigate)
				opRoute.Post("/exceptions/{id}/escalate", agentHandler.HandleEscalate)
				opRoute.Post("/remediations/{id}/approve", agentHandler.HandleApproveRemediation)
				opRoute.Post("/remediations/{id}/reject", agentHandler.HandleRejectRemediation)
				opRoute.Post("/remediations/{id}/execute", agentHandler.HandleExecuteRemediation)
			})
		}
	})

	return r
}
