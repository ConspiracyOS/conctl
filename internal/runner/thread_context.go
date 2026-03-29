package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ThreadTurn struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

type ThreadFacts struct {
	OpenItems []string `json:"open_items,omitempty"`
}

type ThreadRunnerState struct {
	SessionKey      string    `json:"session_key"`
	SessionStrategy string    `json:"session_strategy"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ThreadContext struct {
	Key    string
	Dir    string
	Brief  string
	Recent []ThreadTurn
	Facts  ThreadFacts
	Meta   TaskMetadata
}

func loadThreadContext(agentDir string, task Task) (*ThreadContext, error) {
	if !task.Metadata.HasThread() {
		return nil, nil
	}
	key := task.Metadata.ThreadKey()
	dir := filepath.Join(agentDir, "threads", key)
	ctx := &ThreadContext{
		Key:  key,
		Dir:  dir,
		Meta: task.Metadata,
	}
	if data, err := os.ReadFile(filepath.Join(dir, "brief.md")); err == nil {
		ctx.Brief = strings.TrimSpace(string(data))
	} else if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	turns, err := loadRecentTurns(filepath.Join(dir, "recent.jsonl"))
	if err != nil {
		return nil, err
	}
	ctx.Recent = turns
	facts, err := loadThreadFacts(filepath.Join(dir, "facts.json"))
	if err != nil {
		return nil, err
	}
	ctx.Facts = facts
	return ctx, nil
}

func renderThreadContext(ctx *ThreadContext) string {
	if ctx == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n\n# Conversation Context\n")
	b.WriteString(fmt.Sprintf("- Transport: %s\n", firstNonEmpty(ctx.Meta.Transport, "unknown")))
	if ctx.Meta.Channel != "" {
		b.WriteString(fmt.Sprintf("- Channel: %s\n", ctx.Meta.Channel))
	}
	if ctx.Meta.From != "" {
		b.WriteString(fmt.Sprintf("- From: %s\n", ctx.Meta.From))
	}
	b.WriteString(fmt.Sprintf("- Thread ID: %s\n", ctx.Meta.ThreadID))

	if ctx.Brief != "" {
		b.WriteString("\n## Thread Brief\n\n")
		b.WriteString(strings.TrimSpace(ctx.Brief))
		b.WriteString("\n")
	}

	if len(ctx.Facts.OpenItems) > 0 {
		b.WriteString("\n## Open Items\n\n")
		for _, item := range ctx.Facts.OpenItems {
			if strings.TrimSpace(item) == "" {
				continue
			}
			b.WriteString("- ")
			b.WriteString(strings.TrimSpace(item))
			b.WriteString("\n")
		}
	}

	if len(ctx.Recent) > 0 {
		b.WriteString("\n## Recent Turns\n\n")
		for _, turn := range ctx.Recent {
			b.WriteString(fmt.Sprintf("%s: %s\n\n", strings.Title(turn.Role), clampString(turn.Content, 1200)))
		}
	}

	return b.String()
}

func persistThreadContext(agentDir string, task Task, output, sessionKey, sessionStrategy string, recentLimit, briefMaxBytes int, now time.Time) error {
	if !task.Metadata.HasThread() {
		return nil
	}
	ctx, err := loadThreadContext(agentDir, task)
	if err != nil {
		return err
	}
	if ctx == nil {
		return nil
	}
	if err := os.MkdirAll(ctx.Dir, 0755); err != nil {
		return err
	}

	turns := append([]ThreadTurn{}, ctx.Recent...)
	turns = append(turns,
		ThreadTurn{Role: "user", Content: clampString(task.Content, 4000), At: now.UTC()},
		ThreadTurn{Role: "assistant", Content: clampString(output, 4000), At: now.UTC()},
	)

	summaryLines := loadBriefSummaryLines(filepath.Join(ctx.Dir, "brief.md"))
	if recentLimit <= 0 {
		recentLimit = 8
	}
	if len(turns) > recentLimit {
		overflow := turns[:len(turns)-recentLimit]
		for _, turn := range overflow {
			summaryLines = append(summaryLines, summarizeTurn(turn))
		}
		turns = append([]ThreadTurn{}, turns[len(turns)-recentLimit:]...)
	}

	// Always keep a rolling summary of the latest exchange so brief.md is
	// useful even for short threads that never overflow the recent window.
	latestSummary := summarizeTurn(ThreadTurn{
		Role:    "assistant",
		Content: output,
		At:      now.UTC(),
	})
	// Replace the previous "latest" marker if present, otherwise append.
	replaced := false
	for i, line := range summaryLines {
		if strings.HasPrefix(line, "- [latest] ") {
			summaryLines[i] = "- [latest] " + strings.TrimPrefix(latestSummary, "- ")
			replaced = true
			break
		}
	}
	if !replaced {
		summaryLines = append(summaryLines, "- [latest] "+strings.TrimPrefix(latestSummary, "- "))
	}
	summaryLines = trimSummaryLines(summaryLines, briefMaxBytes)

	if err := writeRecentTurns(filepath.Join(ctx.Dir, "recent.jsonl"), turns); err != nil {
		return err
	}
	if err := writeThreadBrief(filepath.Join(ctx.Dir, "brief.md"), ctx.Meta, summaryLines, now); err != nil {
		return err
	}
	// Extract open items from assistant output — look for bullet lists mentioning
	// actionable keywords (TODO, next, blocker, waiting, need).
	if items := extractOpenItems(output); len(items) > 0 {
		ctx.Facts.OpenItems = items
	}
	if err := ensureThreadFacts(filepath.Join(ctx.Dir, "facts.json"), ctx.Facts); err != nil {
		return err
	}
	return writeThreadRunnerState(filepath.Join(ctx.Dir, "runner_state.json"), ThreadRunnerState{
		SessionKey:      sessionKey,
		SessionStrategy: sessionStrategy,
		UpdatedAt:       now.UTC(),
	})
}

func loadRecentTurns(path string) ([]ThreadTurn, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var turns []ThreadTurn
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var turn ThreadTurn
		if err := json.Unmarshal([]byte(line), &turn); err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func writeRecentTurns(path string, turns []ThreadTurn) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var b strings.Builder
	for _, turn := range turns {
		data, err := json.Marshal(turn)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func loadThreadFacts(path string) (ThreadFacts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ThreadFacts{}, nil
		}
		return ThreadFacts{}, err
	}
	var facts ThreadFacts
	if err := json.Unmarshal(data, &facts); err != nil {
		return ThreadFacts{}, err
	}
	return facts, nil
}

func ensureThreadFacts(path string, facts ThreadFacts) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func writeThreadRunnerState(path string, state ThreadRunnerState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func writeThreadBrief(path string, meta TaskMetadata, summaryLines []string, now time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# Thread Brief\n\n")
	b.WriteString(fmt.Sprintf("_Updated: %s_\n\n", now.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Transport: %s\n", firstNonEmpty(meta.Transport, "unknown")))
	if meta.Channel != "" {
		b.WriteString(fmt.Sprintf("- Channel: %s\n", meta.Channel))
	}
	if meta.From != "" {
		b.WriteString(fmt.Sprintf("- From: %s\n", meta.From))
	}
	b.WriteString(fmt.Sprintf("- Thread ID: %s\n", meta.ThreadID))
	b.WriteString("\n## Summary\n")
	if len(summaryLines) == 0 {
		b.WriteString("\n_No prior summarized turns._\n")
	} else {
		b.WriteString("\n")
		for _, line := range summaryLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func loadBriefSummaryLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var lines []string
	summary := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "## Summary"):
			summary = true
		case summary && strings.HasPrefix(strings.TrimSpace(line), "- "):
			lines = append(lines, strings.TrimSpace(line))
		}
	}
	return lines
}

func trimSummaryLines(lines []string, maxBytes int) []string {
	if maxBytes <= 0 {
		return lines
	}
	var trimmed []string
	size := 0
	for i := len(lines) - 1; i >= 0; i-- {
		lineBytes := len(lines[i]) + 1
		if size+lineBytes > maxBytes {
			break
		}
		size += lineBytes
		trimmed = append(trimmed, lines[i])
	}
	for i, j := 0, len(trimmed)-1; i < j; i, j = i+1, j-1 {
		trimmed[i], trimmed[j] = trimmed[j], trimmed[i]
	}
	return trimmed
}

func summarizeTurn(turn ThreadTurn) string {
	return fmt.Sprintf("- %s: %s", strings.Title(turn.Role), clampString(flattenWhitespace(turn.Content), 280))
}

func flattenWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func clampString(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// extractOpenItems scans assistant output for bullet-point lines containing
// actionable keywords and returns them as open items. Replaces previous items
// each turn — the latest output is the source of truth.
func extractOpenItems(output string) []string {
	keywords := []string{"todo", "next", "blocker", "waiting", "need", "action", "follow-up", "followup"}
	var items []string
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") {
			continue
		}
		lower := strings.ToLower(trimmed)
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				items = append(items, strings.TrimSpace(trimmed[2:]))
				break
			}
		}
	}
	if len(items) > 10 {
		items = items[:10]
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
