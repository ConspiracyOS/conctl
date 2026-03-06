package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

type TaskClaim struct {
	ContractID string    `json:"contract_id"`
	Owner      string    `json:"owner"`
	RunID      string    `json:"run_id"`
	LeaseUntil time.Time `json:"lease_until"`
	ClaimedAt  time.Time `json:"claimed_at"`
}

// ClaimTask atomically claims ownership of a task contract with lease semantics.
// If an active lease exists for a different owner, ClaimTask returns an error.
// Expired leases can be taken over by another owner.
func ClaimTask(registryPath, contractID, owner, runID string, lease time.Duration, now time.Time) (TaskClaim, error) {
	if owner == "" {
		return TaskClaim{}, fmt.Errorf("owner is required")
	}
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0755); err != nil {
		return TaskClaim{}, err
	}

	f, err := os.OpenFile(registryPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return TaskClaim{}, err
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return TaskClaim{}, err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	claims, err := readClaimsLocked(registryPath)
	if err != nil {
		return TaskClaim{}, err
	}
	if existing, ok := claims[contractID]; ok {
		if existing.Owner != owner && existing.LeaseUntil.After(now) {
			return TaskClaim{}, fmt.Errorf("task %s already claimed by %s until %s", contractID, existing.Owner, existing.LeaseUntil.UTC().Format(time.RFC3339))
		}
	}

	claim := TaskClaim{
		ContractID: contractID,
		Owner:      owner,
		RunID:      runID,
		LeaseUntil: now.Add(lease),
		ClaimedAt:  now.UTC(),
	}
	claims[contractID] = claim

	if err := writeClaimsLocked(registryPath, claims); err != nil {
		return TaskClaim{}, err
	}
	return claim, nil
}

func readClaimsLocked(path string) (map[string]TaskClaim, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return map[string]TaskClaim{}, nil
	}
	var claims map[string]TaskClaim
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	if claims == nil {
		claims = map[string]TaskClaim{}
	}
	return claims, nil
}

func writeClaimsLocked(path string, claims map[string]TaskClaim) error {
	// deterministic ordering for stable diffs
	keys := make([]string, 0, len(claims))
	for k := range claims {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make(map[string]TaskClaim, len(claims))
	for _, k := range keys {
		ordered[k] = claims[k]
	}
	data, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
