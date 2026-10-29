package mocks

import (
	"encoding/json"
	"net/http"
	"sync"
)

type FailureMode string

const (
	ModeNone          FailureMode = "none"
	ModeError500      FailureMode = "error_500"
	ModeCommitThen500 FailureMode = "commit_then_500"
)

// FailureInjector controls simulated external service errors.
type FailureInjector struct {
	mu   sync.RWMutex
	mode FailureMode
}

func NewFailureInjector() *FailureInjector {
	return &FailureInjector{mode: ModeNone}
}

func (fi *FailureInjector) SetMode(mode FailureMode) {
	fi.mu.Lock()
	defer fi.mu.Unlock()
	fi.mode = mode
}

func (fi *FailureInjector) GetMode() FailureMode {
	fi.mu.RLock()
	defer fi.mu.RUnlock()
	return fi.mode
}

// HandleAdminFail serves POST /mock/_admin/fail
func (fi *FailureInjector) HandleAdminFail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	switch FailureMode(req.Mode) {
	case ModeNone, ModeError500, ModeCommitThen500:
		fi.SetMode(FailureMode(req.Mode))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"mode":   req.Mode,
		})
	default:
		http.Error(w, "invalid mode: must be none, error_500, or commit_then_500", http.StatusBadRequest)
	}
}

