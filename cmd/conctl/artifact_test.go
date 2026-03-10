package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactCreateAndShow(t *testing.T) {
	stateBase := t.TempDir()
	oldStdout := os.Stdout
	oldStdin := os.Stdin
	defer func() {
		os.Stdout = oldStdout
		os.Stdin = oldStdin
	}()

	if err := os.MkdirAll(filepath.Join(stateBase, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateBase, "status"), 0755); err != nil {
		t.Fatal(err)
	}

	origCreate := artifactCreateRoot
	origShow := artifactShowRoot
	origStatus := artifactStatusRoot
	artifactCreateRoot = filepath.Join(stateBase, "artifacts")
	artifactShowRoot = filepath.Join(stateBase, "artifacts")
	artifactStatusRoot = filepath.Join(stateBase, "status")
	defer func() {
		artifactCreateRoot = origCreate
		artifactShowRoot = origShow
		artifactStatusRoot = origStatus
	}()

	stdinR, stdinW, _ := os.Pipe()
	if _, err := stdinW.Write([]byte("artifact body")); err != nil {
		t.Fatal(err)
	}
	stdinW.Close()
	os.Stdin = stdinR

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW
	artifactCreate([]string{"--title", "Weekly Report", "--name", "report.txt"})
	stdoutW.Close()
	out, _ := io.ReadAll(stdoutR)

	var created artifactJSON
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unmarshal create output: %v; output=%s", err, string(out))
	}
	if created.ID == "" || created.LinkPath == "" {
		t.Fatalf("unexpected create output: %+v", created)
	}

	stdoutR2, stdoutW2, _ := os.Pipe()
	os.Stdout = stdoutW2
	artifactShow([]string{created.ID})
	stdoutW2.Close()
	out2, _ := io.ReadAll(stdoutR2)

	var shown artifactJSON
	if err := json.Unmarshal(out2, &shown); err != nil {
		t.Fatalf("unmarshal show output: %v; output=%s", err, string(out2))
	}
	if shown.Title != "Weekly Report" {
		t.Fatalf("unexpected artifact title: %+v", shown)
	}
}

func TestArtifactLink(t *testing.T) {
	stateBase := t.TempDir()
	oldStdout := os.Stdout
	oldStdin := os.Stdin
	defer func() {
		os.Stdout = oldStdout
		os.Stdin = oldStdin
	}()

	if err := os.MkdirAll(filepath.Join(stateBase, "artifacts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(stateBase, "status"), 0755); err != nil {
		t.Fatal(err)
	}

	origCreate := artifactCreateRoot
	origShow := artifactShowRoot
	origStatus := artifactStatusRoot
	artifactCreateRoot = filepath.Join(stateBase, "artifacts")
	artifactShowRoot = filepath.Join(stateBase, "artifacts")
	artifactStatusRoot = filepath.Join(stateBase, "status")
	defer func() {
		artifactCreateRoot = origCreate
		artifactShowRoot = origShow
		artifactStatusRoot = origStatus
	}()

	secretFile := filepath.Join(stateBase, "artifact-signing.key")
	if err := os.WriteFile(secretFile, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	stdinR, stdinW, _ := os.Pipe()
	if _, err := stdinW.Write([]byte("artifact body")); err != nil {
		t.Fatal(err)
	}
	stdinW.Close()
	os.Stdin = stdinR

	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW
	artifactCreate([]string{"--title", "Linkable", "--name", "report.txt"})
	stdoutW.Close()
	out, _ := io.ReadAll(stdoutR)
	var created artifactJSON
	if err := json.Unmarshal(out, &created); err != nil {
		t.Fatalf("unmarshal create output: %v; output=%s", err, string(out))
	}

	t.Setenv("CONOS_ARTIFACT_SIGNING_KEY_FILE", secretFile)
	stdoutR2, stdoutW2, _ := os.Pipe()
	os.Stdout = stdoutW2
	artifactLink([]string{"--base-url", "https://example.test", created.ID})
	stdoutW2.Close()
	out2, _ := io.ReadAll(stdoutR2)

	var link signedLinkJSON
	if err := json.Unmarshal(out2, &link); err != nil {
		t.Fatalf("unmarshal link output: %v; output=%s", err, string(out2))
	}
	if !strings.Contains(link.URL, "https://example.test/artifacts/") {
		t.Fatalf("unexpected signed link: %s", link.URL)
	}
}
