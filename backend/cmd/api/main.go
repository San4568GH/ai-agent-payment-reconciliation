package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/hackathon/settlement-agent/internal/agent"
	"github.com/hackathon/settlement-agent/internal/api"
	"github.com/hackathon/settlement-agent/internal/config"
	"github.com/hackathon/settlement-agent/internal/db"
	"github.com/hackathon/settlement-agent/migrations"
)

func main() {
	cfg := config.Load()

	log.Printf("Starting settlement-agent api on %s", cfg.HTTPAddr)
	log.Printf("LLM Mode: %s, Model: %s", cfg.LLMMode, cfg.LLMModel)

	// Connect to PostgreSQL with retry loop for resilience during container startup/restart
	var database *db.DB
	var err error
	maxRetries := 20
	for attempt := 1; attempt <= maxRetries; attempt++ {
		ctxAttempt, cancelAttempt := context.WithTimeout(context.Background(), 3*time.Second)
		database, err = db.Connect(ctxAttempt, cfg.DatabaseURL)
		cancelAttempt()
		if err == nil && database != nil && database.Pool != nil {
			log.Printf("Connected to PostgreSQL successfully on attempt %d/%d", attempt, maxRetries)
			break
		}
		log.Printf("Waiting for PostgreSQL to be ready (attempt %d/%d): %v", attempt, maxRetries, err)
		time.Sleep(1 * time.Second)
	}

	if err != nil || database == nil || database.Pool == nil {
		log.Fatalf("Fatal: Database connection failed after %d attempts: %v", maxRetries, err)
	}

	log.Println("Applying database migrations...")
	if err := db.Migrate(database.Pool, migrations.FS, "."); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	ver, err := db.Version(database.Pool)
	if err != nil {
		log.Printf("Warning: failed to fetch migration version: %v", err)
	} else {
		log.Printf("Goose migrations up to date (current version: %d)", ver)
	}

	// Startup cleanup context
	ctxStartup, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	// Cleanup any stale 'running' runs, 'investigating' exceptions, or 'executing' remediations from previous abnormal shutdown
	_, _ = database.Pool.Exec(ctxStartup, "UPDATE exceptions SET status = 'open', updated_at = now() WHERE status = 'investigating'")
	_, _ = database.Pool.Exec(ctxStartup, "UPDATE workflow_runs SET status = 'failed', error = 'interrupted by server restart', finished_at = now() WHERE status = 'running'")
	_, _ = database.Pool.Exec(ctxStartup, "UPDATE remediations SET status = 'execution_failed', last_error = 'interrupted by server restart' WHERE status = 'executing'")

	// Run LLM smoke test
	ctxSmoke, cancelSmoke := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelSmoke()
	smokeResult, err := agent.RunSmokeTest(ctxSmoke, cfg.AnthropicAPIKey, cfg.LLMModel, cfg.LLMMode)
	if err != nil {
		log.Printf("LLM Smoke Test Notice: %v", err)
	} else {
		log.Printf("LLM Smoke Test Passed: provider=%s, model=%s, tool=%s", smokeResult.Provider, smokeResult.Model, smokeResult.ToolUsed)
	}

	// Router setup
	r := api.NewRouter(api.RouterOptions{
		Config:   cfg,
		Database: database,
	})

	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown channel
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Server listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-shutdown
	log.Println("Shutting down server...")

	ctxShut, cancelShut := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShut()

	if err := srv.Shutdown(ctxShut); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	if database != nil {
		database.Close()
	}

	log.Println("Server gracefully stopped")
}
