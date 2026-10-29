package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hackathon/settlement-agent/internal/api"
	"github.com/hackathon/settlement-agent/internal/config"
)

func TestHealthz_DBDisconnected(t *testing.T) {
	router := api.NewRouter(api.RouterOptions{
		Config:   config.Load(),
		Database: nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503 Service Unavailable, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body["status"] != "error" {
		t.Errorf("expected status 'error', got %v", body["status"])
	}
	if body["db"] != "disconnected" {
		t.Errorf("expected db 'disconnected', got %v", body["db"])
	}
}
