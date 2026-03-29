package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TaskMetadata carries transport/thread attribution alongside a .task payload.
type TaskMetadata struct {
	ThreadID    string    `json:"thread_id,omitempty"`
	From        string    `json:"from,omitempty"`
	Channel     string    `json:"channel,omitempty"`
	Transport   string    `json:"transport,omitempty"`
	Source      string    `json:"source,omitempty"`
	ParentRunID string    `json:"parent_run_id,omitempty"`
	SubmittedAt time.Time `json:"submitted_at,omitempty"`
}

func (m TaskMetadata) Normalize() TaskMetadata {
	m.ThreadID = strings.TrimSpace(m.ThreadID)
	m.From = strings.TrimSpace(m.From)
	m.Channel = strings.TrimSpace(m.Channel)
	m.Transport = strings.TrimSpace(m.Transport)
	m.Source = strings.TrimSpace(m.Source)
	m.ParentRunID = strings.TrimSpace(m.ParentRunID)
	return m
}

func (m TaskMetadata) IsZero() bool {
	m = m.Normalize()
	return m.ThreadID == "" &&
		m.From == "" &&
		m.Channel == "" &&
		m.Transport == "" &&
		m.Source == "" &&
		m.ParentRunID == "" &&
		m.SubmittedAt.IsZero()
}

func (m TaskMetadata) HasThread() bool {
	return strings.TrimSpace(m.ThreadID) != ""
}

func (m TaskMetadata) ThreadKey() string {
	if !m.HasThread() {
		return ""
	}
	base := strings.Join([]string{
		strings.TrimSpace(m.Transport),
		strings.TrimSpace(m.Channel),
		strings.TrimSpace(m.From),
		strings.TrimSpace(m.ThreadID),
	}, "|")
	sum := sha256.Sum256([]byte(base))
	return hex.EncodeToString(sum[:8])
}

// WriteTaskWithMetadata writes a .task file and optional .task.meta.json sidecar.
func WriteTaskWithMetadata(inbox, message string, meta TaskMetadata) (string, error) {
	taskID := fmt.Sprintf("%d", time.Now().UnixMicro())
	if err := WriteNamedTaskWithMetadata(inbox, taskID, message, meta); err != nil {
		return "", err
	}
	return taskID, nil
}

// WriteNamedTaskWithMetadata writes a task using a caller-provided task ID.
func WriteNamedTaskWithMetadata(inbox, taskID, message string, meta TaskMetadata) error {
	taskPath := filepath.Join(inbox, taskID+".task")
	if err := os.WriteFile(taskPath, []byte(message), 0644); err != nil {
		return err
	}
	meta = meta.Normalize()
	if meta.IsZero() {
		return nil
	}
	if meta.SubmittedAt.IsZero() {
		meta.SubmittedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(taskMetadataPath(taskPath), data, 0644)
}

func taskMetadataPath(taskPath string) string {
	return taskPath + ".meta.json"
}

func loadTaskMetadata(taskPath string) (TaskMetadata, error) {
	data, err := os.ReadFile(taskMetadataPath(taskPath))
	if err != nil {
		if os.IsNotExist(err) {
			return TaskMetadata{}, nil
		}
		return TaskMetadata{}, err
	}
	var meta TaskMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return TaskMetadata{}, fmt.Errorf("parsing task metadata: %w", err)
	}
	return meta.Normalize(), nil
}

func moveTaskMetadata(srcTask, dstTask string) error {
	srcMeta := taskMetadataPath(srcTask)
	dstMeta := taskMetadataPath(dstTask)

	if _, err := os.Stat(srcMeta); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Rename(srcMeta, dstMeta); err == nil {
		return nil
	}
	data, err := os.ReadFile(srcMeta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dstMeta, data, 0644); err != nil {
		return err
	}
	return os.Remove(srcMeta)
}
