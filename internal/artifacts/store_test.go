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
