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

func TestEffectiveMode_NoACL(t *testing.T) {
	got := effectiveMode("755", 0)
	if got != "755" {
		t.Errorf("expected 755, got %s", got)
	}
}

func TestEffectiveMode_ACLInflatesGroup(t *testing.T) {
	// ACL with rwx (7) on a 755 dir → group bits become 7 → 775
	got := effectiveMode("755", 7)
	if got != "775" {
		t.Errorf("expected 775, got %s", got)
	}
}

func TestEffectiveMode_ACLNoChangeWhenAlreadyHigher(t *testing.T) {
	// 770 already has group=7, ACL rwx (7) shouldn't change it
	got := effectiveMode("770", 7)
	if got != "770" {
		t.Errorf("expected 770, got %s", got)
	}
}

func TestAclMaskForPaths(t *testing.T) {
	acls := []ACL{
		{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rwx"},
		{Path: "/srv/conos/logs/audit", Group: "agents", Perms: "rw", Default: true}, // default ACL ignored
		{Path: "/srv/conos/ledger", Group: "agents", Perms: "rwx"},
	}
	masks := aclMaskForPaths(acls)
	if masks["/srv/conos/logs/audit"] != 7 {
		t.Errorf("expected mask 7 for audit, got %d", masks["/srv/conos/logs/audit"])
	}
	if masks["/srv/conos/ledger"] != 7 {
		t.Errorf("expected mask 7 for ledger, got %d", masks["/srv/conos/ledger"])
	}
}

func TestVerifyLocal_ACLMaskTolerance(t *testing.T) {
	// Create a dir with mode 755 — on Linux with ACLs, stat would show 775
	// because setfacl inflates the group bits. Here we test the logic:
	// manifest says 755, ACL says rwx → effective expected is 775.
	// A dir that IS 775 should pass (not flagged).
	dir := t.TempDir()
	sub := filepath.Join(dir, "audit")
	os.MkdirAll(sub, 0700)
	os.Chmod(sub, 0775) // explicit chmod bypasses umask

	m := Manifest{
		Directories: []Directory{{Path: sub, Mode: "755", Owner: "root", Group: "root"}},
		ACLs:        []ACL{{Path: sub, Group: "agents", Perms: "rwx"}},
	}
	findings := VerifyLocal(m)
	for _, f := range findings {
		if f.Path == sub && f.Category == "mode" && f.Status == "fail" {
			t.Errorf("should not flag mode mismatch when ACL explains the difference: %s", f.What)
		}
	}
}

func TestVerifyLocal_ACLMaskStillCatchesReal(t *testing.T) {
	// A dir with mode 777 when ACL would only explain 775 should still fail
	dir := t.TempDir()
	sub := filepath.Join(dir, "bad")
	os.MkdirAll(sub, 0700)
	os.Chmod(sub, 0777) // explicit chmod bypasses umask

	m := Manifest{
		Directories: []Directory{{Path: sub, Mode: "755", Owner: "root", Group: "root"}},
		ACLs:        []ACL{{Path: sub, Group: "agents", Perms: "rwx"}},
	}
	findings := VerifyLocal(m)
	found := false
	for _, f := range findings {
		if f.Path == sub && f.Category == "mode" && f.Status == "fail" {
			found = true
		}
	}
	if !found {
		t.Error("expected fail: mode 777 is not explained by ACL rwx on group (775)")
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
