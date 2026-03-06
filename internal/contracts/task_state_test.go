package contracts

import (
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAndUpdateTaskContract(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	task, err := OpenTaskContract(root, TaskContractInput{
		ID:    "CON-TASK-100",
		Title: "Fix audit drift",
		Actor: "sysadmin",
		RunID: "run-1",
	}, now)
	if err != nil {
		t.Fatalf("open task: %v", err)
	}
	if task.Status != "open" {
		t.Fatalf("unexpected status: %s", task.Status)
	}
	if _, err := ShowTaskContract(root, task.ID); err != nil {
		t.Fatalf("show task: %v", err)
	}
	updated, err := UpdateTaskContract(root, task.ID, "in_progress", "sysadmin", "sysadmin", "run-2", "started work", now.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updated.Status != "in_progress" || updated.Owner != "sysadmin" {
		t.Fatalf("unexpected updated task: %+v", updated)
	}
	if len(updated.History) != 2 {
		t.Fatalf("history len = %d", len(updated.History))
	}
	if _, err := filepath.Abs(root); err != nil {
		t.Fatal(err)
	}
}
