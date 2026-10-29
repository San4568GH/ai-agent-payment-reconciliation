package ingest

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

var kolkataLoc *time.Location

func init() {
	var err error
	kolkataLoc, err = time.LoadLocation("Asia/Kolkata")
	if err != nil {
		// Fallback to FixedZone (+05:30) if tzdata is missing
		kolkataLoc = time.FixedZone("IST", 5*3600+30*60)
	}
}

// NormalizeRef uppercases and drops every character not in A–Z or 0–9.
// e.g. "ord-1042", "ORD 1042" and "#ord1042" all become "ORD1042".
func NormalizeRef(ref string) string {
	ref = strings.ToUpper(strings.TrimSpace(ref))
	var b strings.Builder
	for _, r := range ref {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseTimestamp parses timestamps in RFC3339 or "2006-01-02 15:04:05" (interpreted as Asia/Kolkata),
// and returns UTC time.
func ParseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty timestamp string")
	}

	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}

	// Try "2006-01-02 15:04:05" in Asia/Kolkata
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, kolkataLoc); err == nil {
		return t.UTC(), nil
	}

	// Try "2006-01-02T15:04:05"
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, kolkataLoc); err == nil {
		return t.UTC(), nil
	}

	return time.Time{}, fmt.Errorf("invalid timestamp %q: expected RFC3339 or 'YYYY-MM-DD HH:MM:SS'", s)
}

// ParseBankDate parses bank transaction dates in "2006-01-02" or "02/01/2006" formats.
// Returns UTC date at 00:00:00.
func ParseBankDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty bank date string")
	}

	// Try ISO "2006-01-02"
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}

	// Try DD/MM/YYYY "02/01/2006"
	if t, err := time.Parse("02/01/2006", s); err == nil {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
	}

	return time.Time{}, fmt.Errorf("invalid bank date %q: expected YYYY-MM-DD or DD/MM/YYYY", s)
}

// StripUnicodeJunk removes non-printable or suspicious unicode characters from raw text.
func StripUnicodeJunk(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsGraphic(r) || unicode.IsSpace(r) {
			return r
		}
		return -1
	}, s)
}
