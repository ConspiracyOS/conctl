package contracts

import (
	"path/filepath"
	"testing"
	"time"
)

func TestClaimTask_NewClaim(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "claims.json")
	now := time.Unix(100, 0).UTC()
	claim, err := ClaimTask(reg, "CON-TASK-001", "sysadmin", "run-1", 2*time.Minute, now)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if claim.Owner != "sysadmin" {
		t.Fatalf("owner = %q", claim.Owner)
	}
	if !claim.LeaseUntil.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("unexpected lease end: %s", claim.LeaseUntil)
	}
}

func TestClaimTask_ConflictOnActiveLease(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "claims.json")
	now := time.Unix(100, 0).UTC()
	if _, err := ClaimTask(reg, "CON-TASK-001", "sysadmin", "run-1", 5*time.Minute, now); err != nil {
		t.Fatalf("seed claim failed: %v", err)
	}
	if _, err := ClaimTask(reg, "CON-TASK-001", "operator", "run-2", 5*time.Minute, now.Add(1*time.Minute)); err == nil {
		t.Fatal("expected active lease conflict")
	}
}

func TestClaimTask_StaleLeaseTakeover(t *testing.T) {
	reg := filepath.Join(t.TempDir(), "claims.json")
	now := time.Unix(100, 0).UTC()
	if _, err := ClaimTask(reg, "CON-TASK-001", "sysadmin", "run-1", 1*time.Minute, now); err != nil {
		t.Fatalf("seed claim failed: %v", err)
	}
	claim, err := ClaimTask(reg, "CON-TASK-001", "operator", "run-2", 2*time.Minute, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("takeover should succeed: %v", err)
	}
	if claim.Owner != "operator" {
		t.Fatalf("owner = %q, want operator", claim.Owner)
	}
}
