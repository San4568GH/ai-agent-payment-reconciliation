package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hackathon/settlement-agent/internal/api"
	"github.com/hackathon/settlement-agent/internal/config"
)

func TestAuthMiddleware_Unauthorized(t *testing.T) {
	cfg := &config.Config{
		OperatorToken: "op-secret",
		ViewerToken:   "view-secret",
	}

	mw := api.AuthMiddleware(cfg)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Case 1: Missing header
	req1 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rec1.Code)
	}

	// Case 2: Wrong token
	req2 := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req2.Header.Set("Authorization", "Bearer invalid-token")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rec2.Code)
	}
}

func TestRequireOperator_RoleEnforcement(t *testing.T) {
	cfg := &config.Config{
		OperatorToken: "op-secret",
		ViewerToken:   "view-secret",
		OperatorName:  "Alice",
	}

	mw := api.AuthMiddleware(cfg)
	protectedHandler := mw(api.RequireOperator(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("operator-ok"))
	})))

	// Viewer token -> 403 Forbidden
	reqViewer := httptest.NewRequest(http.MethodPost, "/api/imports", nil)
	reqViewer.Header.Set("Authorization", "Bearer view-secret")
	recViewer := httptest.NewRecorder()
	protectedHandler.ServeHTTP(recViewer, reqViewer)

	if recViewer.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for viewer role on operator route, got %d", recViewer.Code)
	}

	var errBody api.ErrorEnvelope
	if err := json.Unmarshal(recViewer.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("decode 403 response: %v", err)
	}
	if errBody.Error.Code != "forbidden" {
		t.Errorf("expected error code 'forbidden', got %q", errBody.Error.Code)
	}

	// Operator token -> 200 OK
	reqOp := httptest.NewRequest(http.MethodPost, "/api/imports", nil)
	reqOp.Header.Set("Authorization", "Bearer op-secret")
	recOp := httptest.NewRecorder()
	protectedHandler.ServeHTTP(recOp, reqOp)

	if recOp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for operator role, got %d", recOp.Code)
	}
}
