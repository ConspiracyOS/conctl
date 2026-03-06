package contracts

import (
	"testing"
	"time"
)

func TestAutoCompleteAllPassing(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	_, err := OpenTaskContract(root, TaskContractInput{
		ID:               "TASK-001",
		Title:            "Make CON-SYS-001 pass",
		Actor:            "sysadmin",
		CompletionChecks: []string{"CON-SYS-001", "CON-SYS-002"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-SYS-001", Status: "pass"},
			{ContractID: "CON-SYS-002", Status: "pass"},
		},
	}

	completed, err := EvaluateTaskCompletions(root, result, "healthcheck", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0] != "TASK-001" {
		t.Fatalf("expected [TASK-001], got %v", completed)
	}

	task, _ := ShowTaskContract(root, "TASK-001")
	if task.Status != "completed" {
		t.Fatalf("expected completed, got %s", task.Status)
	}
}

func TestAutoCompletePartiallyPassing(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	_, err := OpenTaskContract(root, TaskContractInput{
		ID:               "TASK-002",
		Title:            "Needs two checks",
		Actor:            "sysadmin",
		CompletionChecks: []string{"CON-A", "CON-B"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-A", Status: "pass"},
			{ContractID: "CON-B", Status: "fail"},
		},
	}

	completed, err := EvaluateTaskCompletions(root, result, "healthcheck", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 0 {
		t.Fatalf("expected no completions, got %v", completed)
	}

	task, _ := ShowTaskContract(root, "TASK-002")
	if task.Status != "open" {
		t.Fatalf("expected open, got %s", task.Status)
	}
}

func TestAutoCompleteEmptyChecks(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	_, err := OpenTaskContract(root, TaskContractInput{
		ID:    "TASK-003",
		Title: "Manual only",
		Actor: "sysadmin",
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-X", Status: "pass"},
		},
	}

	completed, err := EvaluateTaskCompletions(root, result, "healthcheck", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 0 {
		t.Fatalf("expected no completions for empty checks, got %v", completed)
	}
}

func TestAutoCompleteAlreadyCompleted(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	_, err := OpenTaskContract(root, TaskContractInput{
		ID:               "TASK-004",
		Title:            "Already done",
		Actor:            "sysadmin",
		CompletionChecks: []string{"CON-Y"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	// Mark it completed manually first
	_, err = UpdateTaskContract(root, "TASK-004", "completed", "", "sysadmin", "", "done manually", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}

	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-Y", Status: "pass"},
		},
	}

	completed, err := EvaluateTaskCompletions(root, result, "healthcheck", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 0 {
		t.Fatalf("expected no completions for already-completed task, got %v", completed)
	}

	task, _ := ShowTaskContract(root, "TASK-004")
	if len(task.History) != 2 {
		t.Fatalf("history should not have grown, got %d entries", len(task.History))
	}
}

func TestAutoCompleteMultipleTasks(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	_, err := OpenTaskContract(root, TaskContractInput{
		ID:               "TASK-A",
		Title:            "Should complete",
		Actor:            "sysadmin",
		CompletionChecks: []string{"CON-1"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	_, err = OpenTaskContract(root, TaskContractInput{
		ID:               "TASK-B",
		Title:            "Should not complete",
		Actor:            "sysadmin",
		CompletionChecks: []string{"CON-1", "CON-2"},
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	_, err = OpenTaskContract(root, TaskContractInput{
		ID:    "TASK-C",
		Title: "Manual task",
		Actor: "sysadmin",
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	result := RunResult{
		Results: []CheckResult{
			{ContractID: "CON-1", Status: "pass"},
			{ContractID: "CON-2", Status: "fail"},
		},
	}

	completed, err := EvaluateTaskCompletions(root, result, "healthcheck", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(completed) != 1 || completed[0] != "TASK-A" {
		t.Fatalf("expected [TASK-A], got %v", completed)
	}

	taskA, _ := ShowTaskContract(root, "TASK-A")
	taskB, _ := ShowTaskContract(root, "TASK-B")
	taskC, _ := ShowTaskContract(root, "TASK-C")

	if taskA.Status != "completed" {
		t.Fatalf("TASK-A: expected completed, got %s", taskA.Status)
	}
	if taskB.Status != "open" {
		t.Fatalf("TASK-B: expected open, got %s", taskB.Status)
	}
	if taskC.Status != "open" {
		t.Fatalf("TASK-C: expected open, got %s", taskC.Status)
	}
}
