package main

import (
	"fmt"
	"os"
	"strings"
)

// CountPending counts the number of .task files in inboxPath.
// Returns 0 if the directory cannot be read.
func CountPending(inboxPath string) int {
	entries, err := os.ReadDir(inboxPath)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".task") {
			count++
		}
	}
	return count
}

// FormatStatusLine returns a single formatted status line for one agent.
// Format: "%-20s %s  (%d pending)\n"
func FormatStatusLine(name, state string, pending int) string {
	return fmt.Sprintf("%-20s %s  (%d pending)\n", name, state, pending)
}

// TailLines splits data into lines and returns the last n lines.
// The input is trimmed of surrounding whitespace before splitting.
// If data is empty or contains only whitespace, an empty slice is returned.
// If there are fewer than n lines, all lines are returned.
func TailLines(data string, n int) []string {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return []string{}
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// FormatResponse formats a response block for display.
// The header is "=== name: filename ===\n".
// content is truncated to maxLen bytes; if truncated, "\n... (truncated)" is appended.
func FormatResponse(name, filename, content string, maxLen int) string {
	header := fmt.Sprintf("=== %s: %s ===\n", name, filename)
	body := content
	if len(body) > maxLen {
		body = body[:maxLen] + "\n... (truncated)"
	}
	return header + body
}

// LatestResponse returns the last entry in files that ends with ".response".
// files is assumed to be already sorted (e.g. by sort.Strings).
// Returns an empty string if no matching entry is found.
func LatestResponse(files []string) string {
	latest := ""
	for _, f := range files {
		if strings.HasSuffix(f, ".response") {
			latest = f
		}
	}
	return latest
}

// FilterResponses returns the names of DirEntries whose names end with ".response".
func FilterResponses(entries []os.DirEntry) []string {
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".response") {
			out = append(out, e.Name())
		}
	}
	return out
}
