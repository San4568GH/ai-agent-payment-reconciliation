package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type JournalEntryRequest struct {
	DebitAccount  string `json:"debit_account"`
	CreditAccount string `json:"credit_account"`
	AmountMinor   int64  `json:"amount_minor"`
	Currency      string `json:"currency"`
	Description   string `json:"description"`
	ReferenceID   string `json:"reference_id"`
}

type JournalEntryResponse struct {
	EntryID       string    `json:"entry_id"`
	DebitAccount  string    `json:"debit_account"`
	CreditAccount string    `json:"credit_account"`
	AmountMinor   int64     `json:"amount_minor"`
	Currency      string    `json:"currency"`
	Description   string    `json:"description"`
	ReferenceID   string    `json:"reference_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

type AccountingMock struct {
	mu        sync.RWMutex
	injector  *FailureInjector
	entries   map[string]JournalEntryResponse // keyed by Idempotency-Key
	entryList []JournalEntryResponse
	entrySeq  int
}

func NewAccountingMock(fi *FailureInjector) *AccountingMock {
	return &AccountingMock{
		injector:  fi,
		entries:   make(map[string]JournalEntryResponse),
		entryList: make([]JournalEntryResponse, 0),
		entrySeq:  5000,
	}
}

func (am *AccountingMock) RegisterRoutes(r chi.Router) {
	r.Post("/accounting/journal-entries", am.HandleCreateJournalEntry)
	r.Get("/accounting/journal-entries", am.HandleListJournalEntries)
	r.Get("/accounting/balance", am.HandleGetBalance)
}

func (am *AccountingMock) HandleCreateJournalEntry(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get("Idempotency-Key")
	if idempKey == "" {
		http.Error(w, "missing required Idempotency-Key header", http.StatusBadRequest)
		return
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if already processed
	if existing, found := am.entries[idempKey]; found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(existing)
		return
	}

	mode := am.injector.GetMode()
	if mode == ModeError500 {
		http.Error(w, "accounting service simulated failure (error_500)", http.StatusInternalServerError)
		return
	}

	var req JournalEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid journal entry request: %v", err), http.StatusBadRequest)
		return
	}

	am.entrySeq++
	entry := JournalEntryResponse{
		EntryID:       fmt.Sprintf("JE-%d", am.entrySeq),
		DebitAccount:  req.DebitAccount,
		CreditAccount: req.CreditAccount,
		AmountMinor:   req.AmountMinor,
		Currency:      req.Currency,
		Description:   req.Description,
		ReferenceID:   req.ReferenceID,
		Status:        "posted",
		CreatedAt:     time.Now().UTC(),
	}

	// If commit_then_500: commit first, then return 500!
	if mode == ModeCommitThen500 {
		am.entries[idempKey] = entry
		am.entryList = append(am.entryList, entry)
		http.Error(w, "accounting service simulated connection drop after commit (commit_then_500)", http.StatusInternalServerError)
		return
	}

	am.entries[idempKey] = entry
	am.entryList = append(am.entryList, entry)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(entry)
}

func (am *AccountingMock) HandleListJournalEntries(w http.ResponseWriter, r *http.Request) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"entries": am.entryList,
		"count":   len(am.entryList),
	})
}

func (am *AccountingMock) HandleGetBalance(w http.ResponseWriter, r *http.Request) {
	account := r.URL.Query().Get("account")
	if account == "" {
		account = "1100-Cash-In-Transit"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"account":      account,
		"balance_minor": 14500000,
		"currency":     "INR",
		"as_of":        time.Now().UTC().Format(time.RFC3339),
	})
}

