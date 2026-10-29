package mocks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type FeeInquiryRequest struct {
	BatchRef         string `json:"batch_ref"`
	ExpectedFeeMinor int64  `json:"expected_fee_minor"`
	ReportedFeeMinor int64  `json:"reported_fee_minor"`
	DifferenceMinor  int64  `json:"difference_minor"`
	Reason           string `json:"reason"`
}

type FeeInquiryResponse struct {
	TicketID   string    `json:"ticket_id"`
	BatchRef   string    `json:"batch_ref"`
	Status     string    `json:"status"`
	ReceivedAt time.Time `json:"received_at"`
	Message    string    `json:"message"`
}

type GatewayMock struct {
	mu           sync.RWMutex
	injector     *FailureInjector
	inquiries    map[string]FeeInquiryResponse // keyed by Idempotency-Key
	ticketSeq    int
	disputesList []map[string]any
}

func NewGatewayMock(fi *FailureInjector) *GatewayMock {
	return &GatewayMock{
		injector:  fi,
		inquiries: make(map[string]FeeInquiryResponse),
		ticketSeq: 1000,
		disputesList: []map[string]any{
			{
				"dispute_id": "DSP-901",
				"txn_ref":    "PAY-003",
				"amount":     1000.00,
				"currency":   "INR",
				"status":     "under_review",
				"opened_at":  "2026-10-11T14:30:00Z",
			},
			{
				"dispute_id": "DSP-902",
				"txn_ref":    "PAY-007",
				"amount":     2500.00,
				"currency":   "INR",
				"status":     "resolved_merchant_won",
				"opened_at":  "2026-10-09T09:15:00Z",
			},
		},
	}
}

func (gm *GatewayMock) RegisterRoutes(r chi.Router) {
	r.Post("/gateway/fee-inquiry", gm.HandleFeeInquiry)
	r.Get("/gateway/disputes", gm.HandleListDisputes)
	r.Get("/gateway/config", gm.HandleGetConfig)
}

func (gm *GatewayMock) HandleFeeInquiry(w http.ResponseWriter, r *http.Request) {
	idempKey := r.Header.Get("Idempotency-Key")
	if idempKey == "" {
		http.Error(w, "missing required Idempotency-Key header", http.StatusBadRequest)
		return
	}

	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Check if already processed
	if existing, found := gm.inquiries[idempKey]; found {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(existing)
		return
	}

	mode := gm.injector.GetMode()
	if mode == ModeError500 {
		http.Error(w, "gateway simulated internal server error (error_500)", http.StatusInternalServerError)
		return
	}

	var req FeeInquiryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid inquiry request: %v", err), http.StatusBadRequest)
		return
	}

	gm.ticketSeq++
	resp := FeeInquiryResponse{
		TicketID:   fmt.Sprintf("TKT-GW-%d", gm.ticketSeq),
		BatchRef:   req.BatchRef,
		Status:     "received",
		ReceivedAt: time.Now().UTC(),
		Message:    "Gateway operations inquiry ticket opened for fee variance review",
	}

	// If commit_then_500: commit to store first, then return 500!
	if mode == ModeCommitThen500 {
		gm.inquiries[idempKey] = resp
		http.Error(w, "gateway simulated dropped connection after commit (commit_then_500)", http.StatusInternalServerError)
		return
	}

	gm.inquiries[idempKey] = resp
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

func (gm *GatewayMock) HandleListDisputes(w http.ResponseWriter, r *http.Request) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"disputes": gm.disputesList,
		"count":    len(gm.disputesList),
	})
}

func (gm *GatewayMock) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"gateway":               "Razorpay",
		"merchant_id":          "MID-HYPER-2026",
		"settlement_cycle":      "T+2",
		"fee_schedule_effective": "2026-10-15",
		"notes":                 "Card fee revision effective 2026-10-15: 200 bps -> 240 bps. UPI fee: 50 bps.",
	})
}

// InspectGatewayConfig is a programmatic helper used by the agent tool inspect_gateway_config
func (gm *GatewayMock) InspectGatewayConfig(batchRef string) map[string]any {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	isBatchF := strings.Contains(batchRef, "20261012-F") || strings.Contains(batchRef, "1012-F")
	if isBatchF {
		return map[string]any{
			"batch_ref":         batchRef,
			"gateway_contract":  "Standard Retail v2",
			"effective_card_rate_bps": 200,
			"effective_upi_rate_bps":  50,
			"notes":             "Batch STL-20261012-F captured on 2026-10-10 under card 200 bps schedule. Gateway invoice incorrectly billed at 240 bps prematurely.",
			"fee_variance_verified": true,
		}
	}

	return map[string]any{
		"batch_ref":        batchRef,
		"gateway_contract": "Standard Retail v2",
		"active_rates": []map[string]any{
			{"method": "card", "rate_bps": 200, "effective_to": "2026-10-14"},
			{"method": "card", "rate_bps": 240, "effective_from": "2026-10-15"},
			{"method": "upi", "rate_bps": 50},
		},
	}
}

