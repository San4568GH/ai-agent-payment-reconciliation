package ingest_test

import (
	"testing"

	"github.com/hackathon/settlement-agent/internal/ingest"
)

func TestNormalizeRef(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ORD-1042", "ORD1042"},
		{"ord 1042", "ORD1042"},
		{"#ord1042", "ORD1042"},
		{"STL-20261012-F", "STL20261012F"},
		{"  stl 20261012 f ", "STL20261012F"},
		{"REF_999/part-A", "REF999PARTA"},
		{"", ""},
	}

	for _, tc := range tests {
		got := ingest.NormalizeRef(tc.input)
		if got != tc.expected {
			t.Errorf("NormalizeRef(%q) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseTimestamp(t *testing.T) {
	// RFC3339 format
	t1, err := ingest.ParseTimestamp("2026-10-10T11:00:00Z")
	if err != nil {
		t.Fatalf("ParseTimestamp RFC3339 error: %v", err)
	}
	if t1.Year() != 2026 || t1.Month() != 10 || t1.Day() != 10 || t1.Hour() != 11 {
		t.Errorf("unexpected parsed time: %v", t1)
	}

	// 2006-01-02 15:04:05 (IST, which is UTC+5:30)
	t2, err := ingest.ParseTimestamp("2026-10-10 11:00:00")
	if err != nil {
		t.Fatalf("ParseTimestamp IST error: %v", err)
	}
	// 11:00:00 IST is 05:30:00 UTC
	if t2.Hour() != 5 || t2.Minute() != 30 {
		t.Errorf("expected 05:30 UTC for 11:00 IST, got %02d:%02d", t2.Hour(), t2.Minute())
	}
}

func TestParseBankDate(t *testing.T) {
	// ISO format: 2026-10-13
	d1, err := ingest.ParseBankDate("2026-10-13")
	if err != nil {
		t.Fatalf("ParseBankDate ISO error: %v", err)
	}
	if d1.Format("2006-01-02") != "2026-10-13" {
		t.Errorf("expected 2026-10-13, got %s", d1.Format("2006-01-02"))
	}

	// DD/MM/YYYY: 13/10/2026
	d2, err := ingest.ParseBankDate("13/10/2026")
	if err != nil {
		t.Fatalf("ParseBankDate DD/MM/YYYY error: %v", err)
	}
	if d2.Format("2006-01-02") != "2026-10-13" {
		t.Errorf("expected 2026-10-13, got %s", d2.Format("2006-01-02"))
	}
}
