package contracts

import (
	"os"
	"strings"
	"time"
)

// EvaluateTaskCompletions scans open/in_progress task-contracts.
// For each with CompletionChecks, it checks if ALL referenced contract IDs
// passed in the given RunResult. If so, auto-transitions to "completed".
// Returns list of task IDs that were auto-completed.
func EvaluateTaskCompletions(taskContractsRoot string, result RunResult, actor string, now time.Time) ([]string, error) {
	entries, err := os.ReadDir(taskContractsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Build a set of passing contract IDs from the run result.
	passing := make(map[string]bool)
	for _, cr := range result.Results {
		if cr.Status == "pass" {
			passing[cr.ContractID] = true
		}
	}

	var completed []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == ".claims.json" {
			continue
		}

		id := strings.TrimSuffix(name, ".json")
		task, err := ShowTaskContract(taskContractsRoot, id)
		if err != nil {
			continue
		}

		if task.Status != "open" && task.Status != "in_progress" {
			continue
		}
		if len(task.CompletionChecks) == 0 {
			continue
		}

		allPass := true
		for _, checkID := range task.CompletionChecks {
			if !passing[checkID] {
				allPass = false
				break
			}
		}
		if !allPass {
			continue
		}

		_, err = UpdateTaskContract(
			taskContractsRoot, task.ID,
			"completed", "", actor, "",
			"auto-completed: all completion checks passed",
			now,
		)
		if err != nil {
			continue
		}
		completed = append(completed, task.ID)
	}

	return completed, nil
}
