package money_test

import (
	"testing"

	"github.com/hackathon/settlement-agent/internal/money"
)

func TestParseMinor_Valid(t *testing.T) {
	tests := []struct {
		input    string
		expected money.Minor
	}{
		{"1,234.50", 123450},
		{"₹1,234.50", 123450},
		{"₹500", 50000},
		{"500", 50000},
		{"100.5", 10050},
		{"0.05", 5},
		{"0.50", 50},
		{"0", 0},
		{"₹ 2,50,000.00", 25000000},
	}

	for _, tc := range tests {
		got, err := money.ParseMinor(tc.input)
		if err != nil {
			t.Errorf("ParseMinor(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.expected {
			t.Errorf("ParseMinor(%q) = %d, expected %d", tc.input, got, tc.expected)
		}
	}
}

func TestParseMinor_Invalid(t *testing.T) {
	tests := []string{
		"10.505",   // >2 decimal places
		"-5",       // negative
		"-₹100",    // negative
		"abc",      // non-numeric
		"12.34.56", // multiple dots
		"",         // empty
		"   ",      // whitespace
	}

	for _, input := range tests {
		got, err := money.ParseMinor(input)
		if err == nil {
			t.Errorf("ParseMinor(%q) expected error, but got %d", input, got)
		}
	}
}

func TestFeeFor_HalfUpRounding(t *testing.T) {
	// (amount * rateBps + 5000) / 10000
	// Boundary test: exactly .5 paise rounds UP
	// e.g. amount 5000 paise (₹50), rate 10 bps -> 5000 * 10 = 50000 -> (50000 + 5000) / 10000 = 55000 / 10000 = 5
	// e.g. amount 4999 paise, rate 10 bps -> 49990 + 5000 = 54990 / 10000 = 5
	// e.g. amount 4990 paise, rate 10 bps -> 49900 + 5000 = 54900 / 10000 = 5
	// e.g. amount 1000 paise (₹10), rate 200 bps (2%) -> 1000 * 200 = 200000 -> 205000 / 10000 = 20 paise
	fee := money.FeeFor(1000, 200, 0)
	if fee != 20 {
		t.Errorf("FeeFor(1000, 200, 0) = %d, expected 20", fee)
	}

	// 2,50,000 INR (25000000 paise) at 200 bps -> 5000 INR (500000 paise)
	fee2 := money.FeeFor(25000000, 200, 0)
	if fee2 != 500000 {
		t.Errorf("FeeFor(25000000, 200, 0) = %d, expected 500000", fee2)
	}

	// With fixed fee of ₹5 (500 paise)
	fee3 := money.FeeFor(25000000, 200, 500)
	if fee3 != 500500 {
		t.Errorf("FeeFor with fixed fee = %d, expected 500500", fee3)
	}

	// Half-up boundary check:
	// amount 50 paise, rate 100 bps (1%) -> 50 * 100 = 5000 -> 5000 + 5000 = 10000 / 10000 = 1 paise
	if feeRoundUp := money.FeeFor(50, 100, 0); feeRoundUp != 1 {
		t.Errorf("FeeFor(50, 100, 0) = %d, expected 1 (half-up)", feeRoundUp)
	}
	// amount 49 paise, rate 100 bps -> 49 * 100 = 4900 -> 4900 + 5000 = 9900 / 10000 = 0 paise
	if feeRoundDown := money.FeeFor(49, 100, 0); feeRoundDown != 0 {
		t.Errorf("FeeFor(49, 100, 0) = %d, expected 0", feeRoundDown)
	}
}

func TestFormatINR(t *testing.T) {
	tests := []struct {
		input    money.Minor
		expected string
	}{
		{123450, "₹1,234.50"},
		{50000, "₹500.00"},
		{10000000, "₹1,00,000.00"},
		{24400000, "₹2,44,000.00"},
		{-100000, "-₹1,000.00"},
		{0, "₹0.00"},
		{5, "₹0.05"},
	}

	for _, tc := range tests {
		got := money.FormatINR(tc.input)
		if got != tc.expected {
			t.Errorf("FormatINR(%d) = %q, expected %q", tc.input, got, tc.expected)
		}
	}
}
