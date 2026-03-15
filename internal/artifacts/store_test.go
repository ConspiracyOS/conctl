package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndShowArtifact(t *testing.T) {
	root := t.TempDir()
	status := t.TempDir()

	artifact, err := Create(CreateInput{
		ArtifactsRoot: root,
		StatusRoot:    status,
		Title:         "Quarterly Report",
		Kind:          "report",
		ContentType:   "text/markdown; charset=utf-8",
		Filename:      "report.md",
		CreatedBy:     "agent:strategist",
		RunID:         "run-1",
		Audience:      "user",
		Exposure:      ExposureDashboardLocal,
		Content:       []byte("# Report\n"),
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if artifact.ID == "" || artifact.LinkPath == "" {
		t.Fatalf("artifact missing id/link: %+v", artifact)
	}
	if !strings.HasPrefix(artifact.LinkPath, "/artifacts/") {
		t.Fatalf("unexpected link path: %s", artifact.LinkPath)
	}

	loaded, err := Show(root, artifact.ID)
	if err != nil {
		t.Fatalf("show artifact: %v", err)
	}
	if loaded.Title != "Quarterly Report" {
		t.Fatalf("unexpected title: %s", loaded.Title)
	}

	if _, err := os.Stat(filepath.Join(status, "artifacts", artifact.ID, "index.html")); err != nil {
		t.Fatalf("missing published index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(status, "artifacts", artifact.ID, "report.md")); err != nil {
		t.Fatalf("missing published file: %v", err)
	}
}

func TestCreatePrivateArtifactHasNoLink(t *testing.T) {
	root := t.TempDir()
	status := t.TempDir()

	artifact, err := Create(CreateInput{
		ArtifactsRoot: root,
		StatusRoot:    status,
		Content:       []byte("secret"),
		Exposure:      ExposurePrivate,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if artifact.LinkPath != "" {
		t.Fatalf("private artifact should not have link path: %+v", artifact)
	}
}

func TestSave(t *testing.T) {
	root := t.TempDir()
	status := t.TempDir()

	artifact, err := Create(CreateInput{
		ArtifactsRoot: root,
		StatusRoot:    status,
		Title:         "Save Test",
		Content:       []byte("hello"),
		Exposure:      ExposureDashboardLocal,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Modify and save
	artifact.Title = "Updated Title"
	if err := Save(artifact); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Reload and verify
	loaded, err := Show(root, artifact.ID)
	if err != nil {
		t.Fatalf("show after save: %v", err)
	}
	if loaded.Title != "Updated Title" {
		t.Fatalf("expected updated title, got %q", loaded.Title)
	}
}

func TestSaveNilArtifact(t *testing.T) {
	err := Save(nil)
	if err == nil {
		t.Fatal("expected error for nil artifact")
	}
	if !strings.Contains(err.Error(), "artifact is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultFilenameUnknownContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		kind        string
		want        string
	}{
		{"unknown type falls back to .txt", "application/x-totally-fake-12345", "", "artifact.txt"},
		{"empty string falls back to .txt", "", "", "artifact.txt"},
		{"empty type with report kind", "", "report", "artifact.md"},
		{"unknown type with report kind", "application/x-totally-fake-12345", "report", "artifact.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultFilename(tt.contentType, tt.kind)
			if got != tt.want {
				t.Fatalf("defaultFilename(%q, %q) = %q, want %q", tt.contentType, tt.kind, got, tt.want)
			}
		})
	}
}

func TestDefaultContentType(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"data.json", "application/json"},
		{"page.html", "text/html; charset=utf-8"},
		{"data.csv", "text/csv; charset=utf-8"},
		{"readme.txt", "text/plain; charset=utf-8"},
		{"file.xyz-unknown-ext", "text/plain; charset=utf-8"},
		{"noextension", "text/plain; charset=utf-8"},
	}
	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := defaultContentType(tt.filename)
			if got != tt.want {
				t.Fatalf("defaultContentType(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

func TestParseUnixTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantTS  int64
	}{
		{"valid timestamp", "1709740800", false, 1709740800},
		{"zero", "0", false, 0},
		{"invalid string", "not-a-number", true, 0},
		{"empty string", "", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUnixTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Unix() != tt.wantTS {
				t.Fatalf("got %d, want %d", got.Unix(), tt.wantTS)
			}
		})
	}
}

func TestNewArtifactID(t *testing.T) {
	id, err := newArtifactID()
	if err != nil {
		t.Fatalf("newArtifactID: %v", err)
	}
	if !strings.HasPrefix(id, "art_") {
		t.Fatalf("expected art_ prefix, got %q", id)
	}
	// "art_" (4) + 16 hex chars = 20
	if len(id) != 20 {
		t.Fatalf("expected length 20, got %d for %q", len(id), id)
	}

	// Verify uniqueness (two calls should differ)
	id2, err := newArtifactID()
	if err != nil {
		t.Fatalf("newArtifactID second call: %v", err)
	}
	if id == id2 {
		t.Fatalf("expected unique IDs, both are %q", id)
	}
}

func TestPublishStatusView(t *testing.T) {
	statusRoot := t.TempDir()
	artifact := &Artifact{
		ID:          "art_testpublish1234",
		Title:       "Test Publish",
		ContentType: "text/plain; charset=utf-8",
		Filename:    "output.txt",
		CreatedAt:   time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
	}
	content := []byte("published content")

	if err := publishStatusView(statusRoot, artifact, content); err != nil {
		t.Fatalf("publishStatusView: %v", err)
	}

	// Check the artifact file was written
	artPath := filepath.Join(statusRoot, "artifacts", artifact.ID, "output.txt")
	data, err := os.ReadFile(artPath)
	if err != nil {
		t.Fatalf("read published file: %v", err)
	}
	if string(data) != "published content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// Check the index.html was written and contains expected fields
	indexPath := filepath.Join(statusRoot, "artifacts", artifact.ID, "index.html")
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	indexStr := string(indexData)
	for _, want := range []string{"Test Publish", artifact.ID, "text/plain", "output.txt"} {
		if !strings.Contains(indexStr, want) {
			t.Fatalf("index.html missing %q", want)
		}
	}
}

func TestShowNotFound(t *testing.T) {
	root := t.TempDir()
	_, err := Show(root, "art_nonexistent000000")
	if err == nil {
		t.Fatal("expected error for missing artifact")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMintAndVerifySignedLink(t *testing.T) {
	artifact := &Artifact{
		ID:       "art_test",
		Filename: "report.md",
		LinkPath: "/artifacts/art_test/report.md",
	}
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	link, err := MintSignedLink("https://example.test", artifact, []byte("secret"), 15*time.Minute, now)
	if err != nil {
		t.Fatalf("mint link: %v", err)
	}
	if !strings.Contains(link.URL, "sig=") || !strings.Contains(link.URL, "exp=") {
		t.Fatalf("unexpected signed url: %s", link.URL)
	}
	exp := strings.Split(strings.Split(link.URL, "exp=")[1], "&")[0]
	sig := strings.Split(link.URL, "sig=")[1]
	if err := VerifySignedLink(artifact, []byte("secret"), exp, sig, now.Add(10*time.Minute)); err != nil {
		t.Fatalf("verify signed link: %v", err)
	}
	if err := VerifySignedLink(artifact, []byte("secret"), exp, sig, now.Add(16*time.Minute)); err == nil {
		t.Fatal("expected expired link verification failure")
	}
}
