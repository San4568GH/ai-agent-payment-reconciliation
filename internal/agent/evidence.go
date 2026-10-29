package agent

import (
	"fmt"
	"strings"
	"sync"
)

// EvidenceRegistry tracks valid entity references and IDs during an investigation run.
// Any evidence ID cited in tool calls or submit_conclusion must exist in this registry.
type EvidenceRegistry struct {
	mu  sync.RWMutex
	ids map[string]struct{}
}

func NewEvidenceRegistry() *EvidenceRegistry {
	return &EvidenceRegistry{
		ids: make(map[string]struct{}),
	}
}

// Register adds one or more identifiers to the registry.
func (r *EvidenceRegistry) Register(identifiers ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range identifiers {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			r.ids[trimmed] = struct{}{}
		}
	}
}

// Contains checks if an identifier is registered.
func (r *EvidenceRegistry) Contains(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, found := r.ids[strings.TrimSpace(id)]
	return found
}

// ValidateAll ensures all cited IDs exist in the registry.
func (r *EvidenceRegistry) ValidateAll(citedIDs []string) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var invalid []string
	for _, id := range citedIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := r.ids[trimmed]; !ok {
			invalid = append(invalid, trimmed)
		}
	}

	if len(invalid) > 0 {
		return fmt.Errorf("cited evidence IDs not found in run evidence registry: %s", strings.Join(invalid, ", "))
	}
	return nil
}

// All returns a slice of all registered IDs.
func (r *EvidenceRegistry) All() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]string, 0, len(r.ids))
	for id := range r.ids {
		res = append(res, id)
	}
	return res
}

