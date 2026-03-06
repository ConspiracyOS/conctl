package contracts

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type snapshotDoc struct {
	Timestamp    string   `json:"timestamp"`
	EvaluationID string   `json:"evaluation_id"`
	RunID        string   `json:"run_id,omitempty"`
	Actor        string   `json:"actor,omitempty"`
	Failed       []string `json:"failed"`
	Warned       []string `json:"warned"`
	Unknown      []string `json:"unknown"`
}

type snapshotDiffEvent struct {
	Timestamp    string   `json:"timestamp"`
	EvaluationID string   `json:"evaluation_id"`
	PrevHash     string   `json:"prev_hash,omitempty"`
	Hash         string   `json:"hash"`
	NewFailures  []string `json:"new_failures,omitempty"`
	Resolved     []string `json:"resolved,omitempty"`
}

// PersistSnapshotAndDiff writes the latest state snapshot and appends an immutable diff event.
func PersistSnapshotAndDiff(auditDir string, result RunResult) error {
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		return err
	}

	current := buildSnapshotDoc(result)
	currentBytes, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}

	latestPath := filepath.Join(auditDir, "contracts-state.latest.json")
	previous, _ := readSnapshot(latestPath)

	event := snapshotDiffEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		EvaluationID: result.EvaluationID,
		NewFailures:  listDiff(current.Failed, previous.Failed),
		Resolved:     listDiff(previous.Failed, current.Failed),
	}
	prevHash, _ := readLastHash(filepath.Join(auditDir, "contracts-state.diff.jsonl"))
	event.PrevHash = prevHash

	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", event.Timestamp, event.EvaluationID, strings.Join(event.NewFailures, ","), event.PrevHash)))
	event.Hash = hex.EncodeToString(h[:])

	eventBytes, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if err := appendLine(filepath.Join(auditDir, "contracts-state.diff.jsonl"), string(eventBytes)); err != nil {
		return err
	}
	return os.WriteFile(latestPath, currentBytes, 0644)
}

func buildSnapshotDoc(result RunResult) snapshotDoc {
	doc := snapshotDoc{
		Timestamp:    result.Timestamp.UTC().Format(time.RFC3339),
		EvaluationID: result.EvaluationID,
		RunID:        result.RunID,
		Actor:        result.Actor,
	}
	for _, r := range result.Results {
		key := r.ContractID + "/" + r.CheckName
		switch r.Status {
		case "fail":
			doc.Failed = append(doc.Failed, key)
		case "warn":
			doc.Warned = append(doc.Warned, key)
		case "unknown":
			doc.Unknown = append(doc.Unknown, key)
		}
	}
	sort.Strings(doc.Failed)
	sort.Strings(doc.Warned)
	sort.Strings(doc.Unknown)
	return doc
}

func readSnapshot(path string) (snapshotDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return snapshotDoc{}, err
	}
	var doc snapshotDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return snapshotDoc{}, err
	}
	return doc, nil
}

func listDiff(a, b []string) []string {
	bSet := make(map[string]struct{}, len(b))
	for _, x := range b {
		bSet[x] = struct{}{}
	}
	var out []string
	for _, x := range a {
		if _, ok := bSet[x]; !ok {
			out = append(out, x)
		}
	}
	return out
}

func readLastHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var last string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		last = scanner.Text()
	}
	if last == "" {
		return "", scanner.Err()
	}

	var evt snapshotDiffEvent
	if err := json.Unmarshal([]byte(last), &evt); err != nil {
		return "", err
	}
	return evt.Hash, nil
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintln(f, line)
	return err
}
