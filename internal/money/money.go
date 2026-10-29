package money

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Minor represents money amounts in integer paise (1 INR = 100 paise).
type Minor int64

// ParseMinor parses a decimal string (e.g. "1,234.50", "₹500", "100.5") into Minor paise.
// It uses string manipulation only and never converts to float.
// Rejects negative amounts, >2 decimal places, and invalid characters.
func ParseMinor(s string) (Minor, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount string")
	}

	// Reject explicit negative signs
	if strings.HasPrefix(s, "-") {
		return 0, fmt.Errorf("negative amounts not allowed: %s", s)
	}

	// Strip currency symbols and commas
	var b strings.Builder
	for _, r := range s {
		if r == '₹' || r == ',' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	clean := b.String()

	if clean == "" {
		return 0, fmt.Errorf("invalid amount: %s", s)
	}

	parts := strings.Split(clean, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("multiple decimal points in amount: %s", s)
	}

	rupeesStr := parts[0]
	if rupeesStr == "" {
		rupeesStr = "0"
	}
	for _, ch := range rupeesStr {
		if !unicode.IsDigit(ch) {
			return 0, fmt.Errorf("invalid character in rupees: %s", s)
		}
	}
	rupees, err := strconv.ParseInt(rupeesStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse rupees: %w", err)
	}

	var paise int64 = 0
	if len(parts) == 2 {
		paiseStr := parts[1]
		if len(paiseStr) > 2 {
			return 0, fmt.Errorf("more than 2 decimal places in amount: %s", s)
		}
		for _, ch := range paiseStr {
			if !unicode.IsDigit(ch) {
				return 0, fmt.Errorf("invalid character in paise: %s", s)
			}
		}
		if len(paiseStr) == 1 {
			paiseStr += "0"
		} else if len(paiseStr) == 0 {
			paiseStr = "00"
		}
		p, err := strconv.ParseInt(paiseStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse paise: %w", err)
		}
		paise = p
	}

	total := rupees*100 + paise
	return Minor(total), nil
}

// FeeFor calculates the gateway fee using integer half-up rounding:
// (amount * rate_bps + 5000) / 10000 + fixed_minor
func FeeFor(amount Minor, rateBps int, fixedMinor Minor) Minor {
	if amount <= 0 || rateBps < 0 {
		return fixedMinor
	}
	fee := (int64(amount)*int64(rateBps) + 5000) / 10000
	return Minor(fee) + fixedMinor
}

// FormatINR formats Minor paise into Indian Rupee format with ₹ prefix,
// Indian digit grouping (e.g. 1,00,000.00), and paise.
func FormatINR(m Minor) string {
	isNeg := m < 0
	absVal := m
	if isNeg {
		absVal = -m
	}

	rupees := int64(absVal / 100)
	paise := int64(absVal % 100)

	groupedRupees := formatIndianGrouping(rupees)
	res := fmt.Sprintf("₹%s.%02d", groupedRupees, paise)
	if isNeg {
		return "-" + res
	}
	return res
}

func formatIndianGrouping(val int64) string {
	s := strconv.FormatInt(val, 10)
	n := len(s)
	if n <= 3 {
		return s
	}

	last3 := s[n-3:]
	rest := s[:n-3]

	var parts []string
	for len(rest) > 2 {
		parts = append([]string{rest[len(rest)-2:]}, parts...)
		rest = rest[:len(rest)-2]
	}
	if len(rest) > 0 {
		parts = append([]string{rest}, parts...)
	}

	return strings.Join(parts, ",") + "," + last3
}
