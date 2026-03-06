package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type TaskContract struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Owner       string            `json:"owner,omitempty"`
	RunID       string            `json:"run_id,omitempty"`
	Status      string            `json:"status"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	DueAt       time.Time         `json:"due_at,omitempty"`
	Labels      []string          `json:"labels,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	History          []TaskEvent       `json:"history"`
	CompletionChecks []string          `json:"completion_checks,omitempty"`
}

type TaskEvent struct {
	At      time.Time `json:"at"`
	Actor   string    `json:"actor,omitempty"`
	RunID   string    `json:"run_id,omitempty"`
	Type    string    `json:"type"`
	Message string    `json:"message,omitempty"`
	Status  string    `json:"status,omitempty"`
}

type TaskContractInput struct {
	ID          string
	Title       string
	Description string
	Actor       string
	RunID       string
	DueAt       time.Time
	Labels           []string
	Metadata         map[string]string
	CompletionChecks []string
}

func OpenTaskContract(root string, in TaskContractInput, now time.Time) (*TaskContract, error) {
	if in.ID == "" {
		return nil, fmt.Errorf("id is required")
	}
	if in.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	task := &TaskContract{
		ID:          in.ID,
		Title:       in.Title,
		Description: in.Description,
		Status:      "open",
		CreatedAt:   now.UTC(),
		UpdatedAt:   now.UTC(),
		DueAt:       in.DueAt,
		Labels:           in.Labels,
		Metadata:         in.Metadata,
		CompletionChecks: in.CompletionChecks,
		History: []TaskEvent{{
			At:      now.UTC(),
			Actor:   in.Actor,
			RunID:   in.RunID,
			Type:    "opened",
			Status:  "open",
			Message: in.Title,
		}},
	}
	if err := saveTaskContract(root, task); err != nil {
		return nil, err
	}
	return task, nil
}

func UpdateTaskContract(root, id, status, owner, actor, runID, message string, now time.Time) (*TaskContract, error) {
	task, err := ShowTaskContract(root, id)
	if err != nil {
		return nil, err
	}
	if status != "" {
		task.Status = status
	}
	if owner != "" {
		task.Owner = owner
	}
	task.RunID = runID
	task.UpdatedAt = now.UTC()
	eventType := "updated"
	if status != "" {
		eventType = "status_changed"
	}
	task.History = append(task.History, TaskEvent{
		At:      now.UTC(),
		Actor:   actor,
		RunID:   runID,
		Type:    eventType,
		Status:  task.Status,
		Message: message,
	})
	if err := saveTaskContract(root, task); err != nil {
		return nil, err
	}
	return task, nil
}

func ShowTaskContract(root, id string) (*TaskContract, error) {
	path := filepath.Join(root, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var task TaskContract
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func saveTaskContract(root string, task *TaskContract) error {
	if err := os.MkdirAll(root, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, task.ID+".json"), data, 0644)
}
