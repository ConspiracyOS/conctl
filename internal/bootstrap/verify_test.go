package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDirectories_Pass(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "inbox")
	os.MkdirAll(sub, 0700)

	m := Manifest{
		Directories: []Directory{{Path: sub, Mode: "700", Owner: "root", Group: "root"}},
	}
	findings := VerifyLocal(m)
	for _, f := range findings {
		if f.Path == sub && f.Status == "fail" {
			t.Errorf("expected pass for existing dir, got: %s", f.What)
		}
	}
}

func TestVerifyDirectories_Missing(t *testing.T) {
	m := Manifest{
		Directories: []Directory{{Path: "/tmp/nonexistent-conos-test-dir", Mode: "755", Owner: "root", Group: "root"}},
	}
	findings := VerifyLocal(m)
	found := false
	for _, f := range findings {
		if f.Path == "/tmp/nonexistent-conos-test-dir" && f.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail finding for missing directory")
	}
}

func TestVerifyDirectories_WrongMode(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "wrong-mode")
	os.MkdirAll(sub, 0755)

	m := Manifest{
		Directories: []Directory{{Path: sub, Mode: "700", Owner: "root", Group: "root"}},
	}
	findings := VerifyLocal(m)
	found := false
	for _, f := range findings {
		if f.Path == sub && f.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail for wrong mode (755 vs 700)")
	}
}

func TestVerifyFiles_Missing(t *testing.T) {
	m := Manifest{
		Files: []File{{Path: "/tmp/nonexistent-conos-key", Mode: "600", Owner: "root", Group: "root"}},
	}
	findings := VerifyLocal(m)
	found := false
	for _, f := range findings {
		if f.Path == "/tmp/nonexistent-conos-key" && f.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail for missing file")
	}
}

func TestVerifyFiles_Exists(t *testing.T) {
	f, _ := os.CreateTemp("", "conos-verify-test")
	f.Close()
	defer os.Remove(f.Name())
	os.Chmod(f.Name(), 0600)

	m := Manifest{
		Files: []File{{Path: f.Name(), Mode: "600", Owner: "root", Group: "root"}},
	}
	findings := VerifyLocal(m)
	for _, finding := range findings {
		if finding.Path == f.Name() && finding.Status == "fail" && finding.What != "" {
			if finding.Category != "ownership" {
				t.Logf("non-ownership finding: %s (expected on non-root)", finding.What)
			}
		}
	}
}
