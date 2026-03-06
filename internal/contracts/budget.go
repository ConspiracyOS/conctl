package contracts

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type SpendEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Agent     string    `json:"agent"`
	RunID     string    `json:"run_id,omitempty"`
	Model     string    `json:"model,omitempty"`
	CostUSD   float64   `json:"cost_usd"`
}

// RecordSpendAndCheckBudget appends a spend event and returns whether the daily budget is exceeded.
func RecordSpendAndCheckBudget(path string, event SpendEvent, dailyBudgetUSD float64) (bool, float64, error) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.CostUSD < 0 {
		return false, 0, fmt.Errorf("cost_usd cannot be negative")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return false, 0, err
	}
	if err := appendLine(path, string(data)); err != nil {
		return false, 0, err
	}
	total, err := sumDailySpend(path, event.Timestamp)
	if err != nil {
		return false, 0, err
	}
	if dailyBudgetUSD <= 0 {
		return false, total, nil
	}
	return total > dailyBudgetUSD, total, nil
}

func sumDailySpend(path string, day time.Time) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	targetDay := day.UTC().Format("2006-01-02")
	var total float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e SpendEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Timestamp.UTC().Format("2006-01-02") == targetDay {
			total += e.CostUSD
		}
	}
	return total, scanner.Err()
}
