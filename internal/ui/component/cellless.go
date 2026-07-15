package component

import (
	"strconv"
	"strings"
)

// cellLess compares two rendered cells for sorting: empty cells sort last,
// numbers numerically, relative ages ("just now", "5m", "3h", "13d") by
// duration, everything else case-insensitively.
func cellLess(a, b string) bool {
	if a == "" || b == "" {
		return b == "" && a != "" // non-empty before empty
	}
	if na, aok := strconv.ParseFloat(a, 64); aok == nil {
		if nb, bok := strconv.ParseFloat(b, 64); bok == nil {
			return na < nb
		}
	}
	if da, ok := relMinutes(a); ok {
		if db, ok := relMinutes(b); ok {
			return da < db
		}
	}
	return strings.ToLower(a) < strings.ToLower(b)
}

// relMinutes parses the relTime format used by resource cells into minutes.
func relMinutes(s string) (int, bool) {
	if s == "just now" {
		return 0, true
	}
	if len(s) < 2 {
		return 0, false
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, false
	}
	switch unit {
	case 'm':
		return n, true
	case 'h':
		return n * 60, true
	case 'd':
		return n * 60 * 24, true
	}
	return 0, false
}
