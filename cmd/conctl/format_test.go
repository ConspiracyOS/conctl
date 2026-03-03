package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TestCountPending
// ---------------------------------------------------------------------------

func TestCountPending(t *testing.T) {
	tests := []struct {
		name     string
		files    []string // files to create inside the temp dir
		want     int
	}{
		{
			name:  "empty dir",
			files: nil,
			want:  0,
		},
		{
			name:  "only task files",
			files: []string{"a.task", "b.task", "c.task"},
			want:  3,
		},
		{
			name:  "mixed files",
			files: []string{"a.task", "b.task", "note.txt", "response.response"},
			want:  2,
		},
		{
			name:  "no task files",
			files: []string{"note.txt", "image.png"},
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
					t.Fatalf("setup: create file %q: %v", f, err)
				}
			}
			got := CountPending(dir)
			if got != tc.want {
				t.Errorf("CountPending(%q) = %d, want %d", dir, got, tc.want)
			}
		})
	}
}

func TestCountPendingNonExistentDir(t *testing.T) {
	got := CountPending("/does/not/exist/at/all")
	if got != 0 {
		t.Errorf("CountPending on missing dir = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// TestFormatStatusLine
// ---------------------------------------------------------------------------

func TestFormatStatusLine(t *testing.T) {
	tests := []struct {
		name    string
		agent   string
		state   string
		pending int
		want    string
	}{
		{
			name:    "short name active no pending",
			agent:   "concierge",
			state:   "active",
			pending: 0,
			want:    "concierge            active  (0 pending)\n",
		},
		{
			name:    "short name inactive with pending",
			agent:   "sysadmin",
			state:   "inactive",
			pending: 3,
			want:    "sysadmin             inactive  (3 pending)\n",
		},
		{
			name:    "exactly 20 chars",
			agent:   "12345678901234567890",
			state:   "active",
			pending: 1,
			want:    "12345678901234567890 active  (1 pending)\n",
		},
		{
			name:    "name longer than 20 chars",
			agent:   "averylongagentnamexyz",
			state:   "active",
			pending: 0,
			want:    "averylongagentnamexyz active  (0 pending)\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatStatusLine(tc.agent, tc.state, tc.pending)
			if got != tc.want {
				t.Errorf("FormatStatusLine(%q, %q, %d)\n got:  %q\n want: %q",
					tc.agent, tc.state, tc.pending, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestTailLines
// ---------------------------------------------------------------------------

func TestTailLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			n:     20,
			want:  []string{},
		},
		{
			name:  "whitespace only",
			input: "   \n\t  \n",
			n:     20,
			want:  []string{},
		},
		{
			name:  "fewer lines than n",
			input: "line1\nline2\nline3",
			n:     20,
			want:  []string{"line1", "line2", "line3"},
		},
		{
			name:  "exactly n lines",
			input: "a\nb\nc",
			n:     3,
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "more lines than n",
			input: "a\nb\nc\nd\ne",
			n:     3,
			want:  []string{"c", "d", "e"},
		},
		{
			name:  "single line",
			input: "only one",
			n:     5,
			want:  []string{"only one"},
		},
		{
			name:  "n=1 returns last line",
			input: "first\nsecond\nthird",
			n:     1,
			want:  []string{"third"},
		},
		{
			name:  "trailing newline trimmed",
			input: "x\ny\nz\n",
			n:     10,
			want:  []string{"x", "y", "z"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := TailLines(tc.input, tc.n)
			if len(got) != len(tc.want) {
				t.Fatalf("TailLines len=%d, want len=%d\n got:  %v\n want: %v",
					len(got), len(tc.want), got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("TailLines[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestFormatResponse
// ---------------------------------------------------------------------------

func TestFormatResponse(t *testing.T) {
	tests := []struct {
		name     string
		agent    string
		filename string
		content  string
		maxLen   int
		want     string
	}{
		{
			name:     "short content not truncated",
			agent:    "concierge",
			filename: "1234.response",
			content:  "Hello world",
			maxLen:   500,
			want:     "=== concierge: 1234.response ===\nHello world",
		},
		{
			name:     "content exactly at limit",
			agent:    "sysadmin",
			filename: "5678.response",
			content:  "abcde",
			maxLen:   5,
			want:     "=== sysadmin: 5678.response ===\nabcde",
		},
		{
			name:     "content exceeds limit",
			agent:    "sysadmin",
			filename: "5678.response",
			content:  "abcdefghij",
			maxLen:   5,
			want:     "=== sysadmin: 5678.response ===\nabcde\n... (truncated)",
		},
		{
			name:     "empty content",
			agent:    "researcher",
			filename: "0001.response",
			content:  "",
			maxLen:   500,
			want:     "=== researcher: 0001.response ===\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatResponse(tc.agent, tc.filename, tc.content, tc.maxLen)
			if got != tc.want {
				t.Errorf("FormatResponse(%q, %q, %q, %d)\n got:  %q\n want: %q",
					tc.agent, tc.filename, tc.content, tc.maxLen, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestLatestResponse
// ---------------------------------------------------------------------------

func TestLatestResponse(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{
			name:  "empty list",
			files: []string{},
			want:  "",
		},
		{
			name:  "single response file",
			files: []string{"1234567890.response"},
			want:  "1234567890.response",
		},
		{
			name:  "multiple response files sorted",
			files: []string{"1000.response", "2000.response", "3000.response"},
			want:  "3000.response",
		},
		{
			name:  "no response files",
			files: []string{"notes.txt", "data.json"},
			want:  "",
		},
		{
			name:  "mixed files response is latest lexicographically",
			files: []string{"1000.response", "2000.response", "notes.txt"},
			want:  "2000.response",
		},
		{
			name:  "response file not last in list",
			files: []string{"1000.response", "zzz-notes.txt"},
			want:  "1000.response",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := LatestResponse(tc.files)
			if got != tc.want {
				t.Errorf("LatestResponse(%v) = %q, want %q", tc.files, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestFilterResponses
// ---------------------------------------------------------------------------

func TestFilterResponses(t *testing.T) {
	tests := []struct {
		name  string
		files []string // filenames to create; dirs handled separately
		dirs  []string // subdir names to create
		want  []string
	}{
		{
			name:  "empty directory",
			files: nil,
			want:  []string{},
		},
		{
			name:  "only response files",
			files: []string{"a.response", "b.response"},
			want:  []string{"a.response", "b.response"},
		},
		{
			name:  "mixed files",
			files: []string{"a.response", "b.task", "c.txt", "d.response"},
			want:  []string{"a.response", "d.response"},
		},
		{
			name:  "no response files",
			files: []string{"task.task", "note.md"},
			want:  []string{},
		},
		{
			name:  "subdirectory with response name is excluded",
			files: []string{"a.response"},
			dirs:  []string{"subdir.response"},
			want:  []string{"a.response", "subdir.response"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0644); err != nil {
					t.Fatalf("setup: create file %q: %v", f, err)
				}
			}
			for _, d := range tc.dirs {
				if err := os.Mkdir(filepath.Join(dir, d), 0755); err != nil {
					t.Fatalf("setup: create dir %q: %v", d, err)
				}
			}

			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("ReadDir: %v", err)
			}

			got := FilterResponses(entries)

			// Normalise nil vs empty slice for comparison
			if got == nil {
				got = []string{}
			}

			if len(got) != len(tc.want) {
				t.Fatalf("FilterResponses len=%d, want len=%d\n got:  %v\n want: %v",
					len(got), len(tc.want), got, tc.want)
			}
			// Both slices should be in sorted order (os.ReadDir returns sorted entries)
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("FilterResponses[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
