package strutil

import "strings"

// FirstNonEmpty returns the first non-empty string from values.
// Each value is trimmed before checking.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
