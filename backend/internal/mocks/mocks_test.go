package mocks_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/hackathon/settlement-agent/internal/mocks"
)

func setupTestRouter() (*chi.Mux, *mocks.FailureInjector, *mocks.GatewayMock, *mocks.AccountingMock) {
	r := chi.NewRouter()
	fi := mocks.NewFailureInjector()
	gw := mocks.NewGatewayMock(fi)
	ac := mocks.NewAccountingMock(fi)

	r.Post("/mock/_admin/fail", fi.HandleAdminFail)
	gw.RegisterRoutes(r)
	ac.RegisterRoutes(r)

	return r, fi, gw, ac
}

func TestFailureInjector_AdminRoute(t *testing.T) {
	r, fi, _, _ := setupTestRouter()

	body := bytes.NewBufferString(`{"mode":"error_500"}`)
	req := httptest.NewRequest(http.MethodPost, "/mock/_admin/fail", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}
	if fi.GetMode() != mocks.ModeError500 {
		t.Fatalf("expected injector mode error_500, got %s", fi.GetMode())
	}

	// Reset
	bodyReset := bytes.NewBufferString(`{"mode":"none"}`)
	reqReset := httptest.NewRequest(http.MethodPost, "/mock/_admin/fail", bodyReset)
	wReset := httptest.NewRecorder()
	r.ServeHTTP(wReset, reqReset)

	if fi.GetMode() != mocks.ModeNone {
		t.Fatalf("expected injector mode none, got %s", fi.GetMode())
	}
}

func TestGatewayMock_FeeInquiry_IdempotencyAndFailures(t *testing.T) {
	r, fi, _, _ := setupTestRouter()

	inqReq := mocks.FeeInquiryRequest{
		BatchRef:         "STL-20261012-F",
		ExpectedFeeMinor: 500000,
		ReportedFeeMinor: 600000,
		DifferenceMinor:  100000,
		Reason:           "Premature application of 240 bps rate",
	}
	payload, _ := json.Marshal(inqReq)

	// 1. Success 201
	req1 := httptest.NewRequest(http.MethodPost, "/gateway/fee-inquiry", bytes.NewReader(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "idemp-gw-001")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w1.Code, w1.Body.String())
	}
	var res1 mocks.FeeInquiryResponse
	json.NewDecoder(w1.Body).Decode(&res1)
	if res1.TicketID == "" || res1.Status != "received" {
		t.Fatalf("unexpected inquiry response: %+v", res1)
	}

	// 2. Replay with same Idempotency-Key -> 200 OK with same TicketID
	req2 := httptest.NewRequest(http.MethodPost, "/gateway/fee-inquiry", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "idemp-gw-001")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for replay, got %d", w2.Code)
	}
	var res2 mocks.FeeInquiryResponse
	json.NewDecoder(w2.Body).Decode(&res2)
	if res2.TicketID != res1.TicketID {
		t.Fatalf("expected replay ticket ID %s, got %s", res1.TicketID, res2.TicketID)
	}

	// 3. Test ModeError500
	fi.SetMode(mocks.ModeError500)
	req3 := httptest.NewRequest(http.MethodPost, "/gateway/fee-inquiry", bytes.NewReader(payload))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", "idemp-gw-error")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 InternalServerError under error_500, got %d", w3.Code)
	}

	// 4. Test ModeCommitThen500
	fi.SetMode(mocks.ModeCommitThen500)
	req4a := httptest.NewRequest(http.MethodPost, "/gateway/fee-inquiry", bytes.NewReader(payload))
	req4a.Header.Set("Content-Type", "application/json")
	req4a.Header.Set("Idempotency-Key", "idemp-gw-commit500")
	w4a := httptest.NewRecorder()
	r.ServeHTTP(w4a, req4a)

	if w4a.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 under commit_then_500, got %d", w4a.Code)
	}

	// Retry on commit_then_500 (even while mode is commit_then_500 or none) should return 200 with stored result
	req4b := httptest.NewRequest(http.MethodPost, "/gateway/fee-inquiry", bytes.NewReader(payload))
	req4b.Header.Set("Content-Type", "application/json")
	req4b.Header.Set("Idempotency-Key", "idemp-gw-commit500")
	w4b := httptest.NewRecorder()
	r.ServeHTTP(w4b, req4b)

	if w4b.Code != http.StatusOK {
		t.Fatalf("expected 200 OK replay after commit_then_500, got %d", w4b.Code)
	}
	var res4b mocks.FeeInquiryResponse
	json.NewDecoder(w4b.Body).Decode(&res4b)
	if res4b.TicketID == "" {
		t.Fatalf("expected committed ticket ID, got empty")
	}
}

func TestAccountingMock_JournalEntries_IdempotencyAndFailures(t *testing.T) {
	r, fi, _, _ := setupTestRouter()

	jeReq := mocks.JournalEntryRequest{
		DebitAccount:  "1200-Gateway-Receivables",
		CreditAccount: "5100-Payment-Processing-Fee",
		AmountMinor:   500000,
		Currency:      "INR",
		Description:   "Record missing refund from gateway statement",
		ReferenceID:   "REF-NOT-LEDGER-01",
	}
	payload, _ := json.Marshal(jeReq)

	// 1. Success 201
	req1 := httptest.NewRequest(http.MethodPost, "/accounting/journal-entries", bytes.NewReader(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "idemp-je-001")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w1.Code, w1.Body.String())
	}
	var res1 mocks.JournalEntryResponse
	json.NewDecoder(w1.Body).Decode(&res1)
	if res1.EntryID == "" || res1.Status != "posted" {
		t.Fatalf("unexpected journal entry response: %+v", res1)
	}

	// 2. Replay with same Idempotency-Key -> 200 OK with same EntryID
	req2 := httptest.NewRequest(http.MethodPost, "/accounting/journal-entries", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "idemp-je-001")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for replay, got %d", w2.Code)
	}
	var res2 mocks.JournalEntryResponse
	json.NewDecoder(w2.Body).Decode(&res2)
	if res2.EntryID != res1.EntryID {
		t.Fatalf("expected replay entry ID %s, got %s", res1.EntryID, res2.EntryID)
	}

	// 3. Test ModeCommitThen500
	fi.SetMode(mocks.ModeCommitThen500)
	req3a := httptest.NewRequest(http.MethodPost, "/accounting/journal-entries", bytes.NewReader(payload))
	req3a.Header.Set("Content-Type", "application/json")
	req3a.Header.Set("Idempotency-Key", "idemp-je-commit500")
	w3a := httptest.NewRecorder()
	r.ServeHTTP(w3a, req3a)

	if w3a.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 under commit_then_500, got %d", w3a.Code)
	}

	// Retry returns 200 with stored entry
	req3b := httptest.NewRequest(http.MethodPost, "/accounting/journal-entries", bytes.NewReader(payload))
	req3b.Header.Set("Content-Type", "application/json")
	req3b.Header.Set("Idempotency-Key", "idemp-je-commit500")
	w3b := httptest.NewRecorder()
	r.ServeHTTP(w3b, req3b)

	if w3b.Code != http.StatusOK {
		t.Fatalf("expected 200 OK replay after commit_then_500, got %d", w3b.Code)
	}

	// 4. Verify List Entries
	reqList := httptest.NewRequest(http.MethodGet, "/accounting/journal-entries", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list entries, got %d", wList.Code)
	}
	var listResp struct {
		Entries []mocks.JournalEntryResponse `json:"entries"`
		Count   int                          `json:"count"`
	}
	json.NewDecoder(wList.Body).Decode(&listResp)
	if listResp.Count != 2 {
		t.Fatalf("expected 2 recorded entries in list, got %d", listResp.Count)
	}
}

