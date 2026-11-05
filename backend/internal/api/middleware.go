package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/hackathon/settlement-agent/internal/config"
)

type contextKey string

const (
	roleContextKey  contextKey = "user_role"
	actorContextKey contextKey = "user_actor"
)

func RoleFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(roleContextKey).(string); ok {
		return val
	}
	return ""
}

func ActorFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(actorContextKey).(string); ok {
		return val
	}
	return "unknown"
}

func AuthMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed authorization header", nil)
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			var role string
			var actor string

			if token == cfg.OperatorToken {
				role = "operator"
				actor = "operator:" + cfg.OperatorName
			} else if token == cfg.ViewerToken {
				role = "viewer"
				actor = "viewer"
			} else {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid bearer token", nil)
				return
			}

			ctx := context.WithValue(r.Context(), roleContextKey, role)
			ctx = context.WithValue(ctx, actorContextKey, actor)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func RequireOperator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := RoleFromContext(r.Context())
		if role != "operator" {
			WriteError(w, http.StatusForbidden, "forbidden", "operator role required for this action", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ErrorEnvelope struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, code, message string, details any) {
	if details == nil {
		details = map[string]any{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorEnvelope{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
