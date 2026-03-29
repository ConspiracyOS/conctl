package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/runner"
)

func TestShowLogs_NoLogs(t *testing.T) {
	showLogsFrom(t.TempDir(), &logOpts{n: 20})
}

func TestShowLogsFrom_TodayLog(t *testing.T) {
	auditDir := t.TempDir()
	today := time.Now().Format("2006-01-02")

	// Write a log with more than 20 lines — covers the tail branch
	var lines []string
	for i := 0; i < 25; i++ {
		lines = append(lines, strings.Repeat("x", 10))
	}
	os.WriteFile(filepath.Join(auditDir, today+".log"), []byte(strings.Join(lines, "\n")), 0644)

	showLogsFrom(auditDir, &logOpts{n: 20})
}

func TestShowLogsFrom_ContractsLogFallback(t *testing.T) {
	auditDir := t.TempDir()
	// No today.log, but contracts.log exists — should print it
	os.WriteFile(filepath.Join(auditDir, "contracts.log"), []byte("contract log line\n"), 0644)
	// No panic or error expected
	showLogsFrom(auditDir, &logOpts{n: 20})
}

func TestShowLogsFrom_LastN(t *testing.T) {
	dir := t.TempDir()
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(dir, today+".log")
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%02d", i)
	}
	if err := os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Redirect stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	showLogsFrom(dir, &logOpts{n: 5, follow: false, agent: ""})

	w.Close()
	os.Stdout = old
	var buf strings.Builder
	io.Copy(&buf, r)
	out := buf.String()

	if !strings.Contains(out, "line-29") {
		t.Errorf("expected last line (line-29), got: %s", out)
	}
	if strings.Contains(out, "line-00") {
		t.Errorf("should not contain early lines (line-00), got: %s", out)
	}
}

func TestDropTaskToAgent(t *testing.T) {
	dir := t.TempDir()
	agentInbox := filepath.Join(dir, "agents", "researcher", "inbox")
	if err := os.MkdirAll(agentInbox, 0755); err != nil {
		t.Fatal(err)
	}

	meta := runner.TaskMetadata{ThreadID: "thread-123", From: "alice", Channel: "ops", Transport: "openclaw"}
	if err := dropTaskToAgent(filepath.Join(dir, "agents"), "researcher", "do research", meta); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(agentInbox)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected task file and metadata sidecar, got %d entries", len(files))
	}
	var taskFile string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".task") {
			taskFile = file.Name()
			break
		}
	}
	data, _ := os.ReadFile(filepath.Join(agentInbox, taskFile))
	if string(data) != "do research" {
		t.Fatalf("unexpected task content: %q", string(data))
	}
	metaBytes, err := os.ReadFile(filepath.Join(agentInbox, taskFile+".meta.json"))
	if err != nil {
		t.Fatalf("expected metadata sidecar: %v", err)
	}
	var saved runner.TaskMetadata
	if err := json.Unmarshal(metaBytes, &saved); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if saved.ThreadID != "thread-123" || saved.From != "alice" || saved.Transport != "openclaw" {
		t.Fatalf("unexpected metadata: %+v", saved)
	}
}

func TestKillAgent_NoUnits(t *testing.T) {
	// killAgentUnits calls systemctl stop — expected to fail in test env (no systemd)
	// Just verify it doesn't panic.
	err := killAgentUnits("nonexistent-agent")
	_ = err // error expected in test env
}

func TestDropTaskTo_Success(t *testing.T) {
	inbox := t.TempDir()
	if err := dropTaskTo(inbox, "hello world", runner.TaskMetadata{}); err != nil {
		t.Fatalf("dropTaskTo failed: %v", err)
	}
	entries, _ := os.ReadDir(inbox)
	if len(entries) != 1 {
		t.Fatalf("expected 1 task file in inbox, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".task") {
		t.Errorf("expected .task file, got %q", entries[0].Name())
	}
	data, _ := os.ReadFile(filepath.Join(inbox, entries[0].Name()))
	if string(data) != "hello world" {
		t.Errorf("unexpected task content: %q", string(data))
	}
}

func TestDropTaskTo_Error(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}
	inbox := t.TempDir()
	os.Chmod(inbox, 0555)
	defer os.Chmod(inbox, 0755)

	if err := dropTaskTo(inbox, "test", runner.TaskMetadata{}); err == nil {
		t.Error("expected error writing to read-only inbox")
	}
}

func TestShowResponsesIn_Empty(t *testing.T) {
	// No agents dir entries — just returns without output
	if err := showResponsesIn(t.TempDir()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_BadDir(t *testing.T) {
	err := showResponsesIn("/nonexistent-xyz/agents")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
	if !strings.Contains(err.Error(), "cannot read agents dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_WithResponse(t *testing.T) {
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "concierge", "outbox")
	os.MkdirAll(outbox, 0755)
	os.WriteFile(filepath.Join(outbox, "20260101-120000-task.response"), []byte("response content"), 0644)
	// Plain file in agentsDir covers the !e.IsDir() continue branch
	os.WriteFile(filepath.Join(agentsDir, "notadir.txt"), []byte("ignored"), 0644)

	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_Truncation(t *testing.T) {
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "sysadmin", "outbox")
	os.MkdirAll(outbox, 0755)
	// Content > 500 bytes should be truncated
	big := strings.Repeat("a", 600)
	os.WriteFile(filepath.Join(outbox, "20260101-120000-task.response"), []byte(big), 0644)
	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowResponsesIn_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root")
	}
	agentsDir := t.TempDir()
	outbox := filepath.Join(agentsDir, "test-agent", "outbox")
	os.MkdirAll(outbox, 0755)
	f := filepath.Join(outbox, "20260101-120000.response")
	os.WriteFile(f, []byte("content"), 0000) // mode 0: unreadable
	defer os.Chmod(f, 0644)

	if err := showResponsesIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowStatusIn(t *testing.T) {
	agentsDir := t.TempDir()
	// Create a fake agent with two tasks pending
	inboxDir := filepath.Join(agentsDir, "concierge", "inbox")
	os.MkdirAll(inboxDir, 0755)
	os.WriteFile(filepath.Join(inboxDir, "001.task"), []byte("task"), 0644)
	os.WriteFile(filepath.Join(inboxDir, "002.task"), []byte("task"), 0644)
	os.WriteFile(filepath.Join(inboxDir, "readme.txt"), []byte("ignored"), 0644)
	// Plain file in agentsDir covers the !e.IsDir() continue branch
	os.WriteFile(filepath.Join(agentsDir, "notadir.txt"), []byte("ignored"), 0644)

	// systemctl will return non-zero (not installed in test env) → state = "inactive"
	// No panic or error expected; output goes to stdout
	if err := showStatusIn(agentsDir); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowStatusIn_BadDir(t *testing.T) {
	err := showStatusIn("/nonexistent-xyz/agents")
	if err == nil {
		t.Error("expected error for nonexistent dir")
	}
	if !strings.Contains(err.Error(), "cannot read agents dir") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCollectAuthWarnings_OAuthToken(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			Runner:    "picoclaw",
			Provider:  "anthropic",
			APIKeyEnv: "CONOS_API_KEY",
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	env := map[string]string{
		"CONOS_API_KEY": "sk-ant-oat01-example",
	}
	warnings := collectAuthWarnings(cfg, env)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "OAuth token") {
		t.Fatalf("expected OAuth warning, got: %v", warnings[0])
	}
}

func TestCollectAuthWarnings_NoWarningForApiKey(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			Runner:    "picoclaw",
			Provider:  "anthropic",
			APIKeyEnv: "CONOS_API_KEY",
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	env := map[string]string{
		"CONOS_API_KEY": "sk-ant-api03-example",
	}
	warnings := collectAuthWarnings(cfg, env)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

func TestCollectAuthWarnings_MissingKey(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			Runner:    "picoclaw",
			Provider:  "anthropic",
			APIKeyEnv: "CONOS_API_KEY",
		},
		Agents: []config.AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	warnings := collectAuthWarnings(cfg, map[string]string{})
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d (%v)", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "missing Anthropic credential") {
		t.Fatalf("expected missing key warning, got: %v", warnings[0])
	}
}

func TestParseSimpleEnvFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "env")
	content := "# comment\nCONOS_API_KEY=abc\nBROKENLINE\nX = y\n"
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	env := parseSimpleEnvFile(f)
	if env["CONOS_API_KEY"] != "abc" {
		t.Fatalf("expected CONOS_API_KEY=abc, got %q", env["CONOS_API_KEY"])
	}
	if env["X"] != "y" {
		t.Fatalf("expected X=y, got %q", env["X"])
	}
}

func TestParseSimpleEnvFile_CommentsAndBlanks(t *testing.T) {
	f := filepath.Join(t.TempDir(), "env")
	content := "# full line comment\n\n  # indented comment\n  \nKEY=val\n"
	os.WriteFile(f, []byte(content), 0644)
	env := parseSimpleEnvFile(f)
	if len(env) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(env), env)
	}
	if env["KEY"] != "val" {
		t.Fatalf("expected KEY=val, got %q", env["KEY"])
	}
}

func TestParseSimpleEnvFile_NoEquals(t *testing.T) {
	f := filepath.Join(t.TempDir(), "env")
	content := "NOEQUALSSIGN\nGOOD=value\nALSOBAD\n"
	os.WriteFile(f, []byte(content), 0644)
	env := parseSimpleEnvFile(f)
	if len(env) != 1 {
		t.Fatalf("expected 1 entry, got %d: %v", len(env), env)
	}
	if env["GOOD"] != "value" {
		t.Fatalf("expected GOOD=value, got %q", env["GOOD"])
	}
}

func TestParseSimpleEnvFile_EqualsInValue(t *testing.T) {
	f := filepath.Join(t.TempDir(), "env")
	content := "TOKEN=abc=def=ghi\n"
	os.WriteFile(f, []byte(content), 0644)
	env := parseSimpleEnvFile(f)
	if env["TOKEN"] != "abc=def=ghi" {
		t.Fatalf("expected value with equals signs preserved, got %q", env["TOKEN"])
	}
}

func TestParseSimpleEnvFile_NonexistentFile(t *testing.T) {
	env := parseSimpleEnvFile("/nonexistent/path/env")
	if len(env) != 0 {
		t.Fatalf("expected empty map for nonexistent file, got %v", env)
	}
}

func TestParseSimpleEnvFile_EmptyPath(t *testing.T) {
	env := parseSimpleEnvFile("")
	if len(env) != 0 {
		t.Fatalf("expected empty map for empty path, got %v", env)
	}
}

func writeMinimalConfig(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "conos.toml")
	content := `[base]
provider = "anthropic"
runner = "picoclaw"
api_key_env = "CONOS_API_KEY"

[[agents]]
name = "concierge"
tier = "operator"
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAuthPreflightWarnings_EmptyConfigPath(t *testing.T) {
	warnings := authPreflightWarnings("", "/some/env")
	if warnings != nil {
		t.Fatalf("expected nil for empty config path, got %v", warnings)
	}
}

func TestAuthPreflightWarnings_NonexistentConfig(t *testing.T) {
	warnings := authPreflightWarnings("/nonexistent/conos.toml", "")
	if warnings != nil {
		t.Fatalf("expected nil for nonexistent config, got %v", warnings)
	}
}

func TestAuthPreflightWarnings_ValidConfigWithAPIKey(t *testing.T) {
	dir := t.TempDir()
	configPath := writeMinimalConfig(t, dir)

	envFile := filepath.Join(dir, "env")
	os.WriteFile(envFile, []byte("CONOS_API_KEY=sk-ant-api03-realkey\n"), 0644)

	warnings := authPreflightWarnings(configPath, envFile)
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings with valid API key, got %v", warnings)
	}
}

func TestAuthPreflightWarnings_OAuthToken(t *testing.T) {
	dir := t.TempDir()
	configPath := writeMinimalConfig(t, dir)

	envFile := filepath.Join(dir, "env")
	os.WriteFile(envFile, []byte("CONOS_API_KEY=sk-ant-oat01-oauthtoken\n"), 0644)

	warnings := authPreflightWarnings(configPath, envFile)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 OAuth warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "OAuth token") {
		t.Fatalf("expected OAuth warning, got: %s", warnings[0])
	}
}

func TestAuthPreflightWarnings_MissingKeyNoEnvFile(t *testing.T) {
	dir := t.TempDir()
	configPath := writeMinimalConfig(t, dir)

	// Unset the env vars that collectAuthWarnings checks, in case they're
	// set in the test runner's environment.
	t.Setenv("CONOS_API_KEY", "")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "")

	warnings := authPreflightWarnings(configPath, "")
	if len(warnings) != 1 {
		t.Fatalf("expected 1 missing-key warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "missing Anthropic credential") {
		t.Fatalf("expected missing key warning, got: %s", warnings[0])
	}
}

func TestAuthPreflightWarnings_NonexistentEnvPath(t *testing.T) {
	dir := t.TempDir()
	configPath := writeMinimalConfig(t, dir)

	t.Setenv("CONOS_API_KEY", "")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "")

	// Non-existent env path should not cause an error; parseSimpleEnvFile returns empty map.
	warnings := authPreflightWarnings(configPath, "/nonexistent/env")
	if len(warnings) != 1 {
		t.Fatalf("expected 1 missing-key warning (env file missing), got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "missing Anthropic credential") {
		t.Fatalf("expected missing key warning, got: %s", warnings[0])
	}
}

func TestRunAgentLoop_DrainInbox(t *testing.T) {
	agentName := "test-agent"
	stateBase := t.TempDir()
	homeBase := t.TempDir()
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.MkdirAll(filepath.Join(agentDir, "inbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "outbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "processed"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Agent\n"), 0644)

	// Two tasks — both processed by cat, then inbox drained → return nil
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("first"), 0644)
	os.WriteFile(filepath.Join(agentDir, "inbox", "002.task"), []byte("second"), 0644)

	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "cat"}}}
	dirs := runner.Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}

	if err := runAgentLoop(agentName, cfg, dirs); err != nil {
		t.Fatalf("runAgentLoop failed: %v", err)
	}

	entries, _ := os.ReadDir(filepath.Join(agentDir, "outbox"))
	if len(entries) != 2 {
		t.Errorf("expected 2 responses in outbox, got %d", len(entries))
	}
}

func TestRunAgentLoop_RuntimeError(t *testing.T) {
	agentName := "test-agent"
	stateBase := t.TempDir()
	homeBase := t.TempDir()
	agentDir := filepath.Join(stateBase, "agents", agentName)
	os.MkdirAll(filepath.Join(agentDir, "inbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "outbox"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "processed"), 0755)
	os.MkdirAll(filepath.Join(agentDir, "workspace"), 0755)
	agentHome := filepath.Join(homeBase, "a-"+agentName)
	os.MkdirAll(agentHome, 0755)
	os.WriteFile(filepath.Join(agentHome, "AGENTS.md"), []byte("# Agent\n"), 0644)
	os.WriteFile(filepath.Join(agentDir, "inbox", "001.task"), []byte("fail"), 0644)

	// "false" exits 1 with no output → runner.Run returns "runtime returned no output"
	cfg := &config.Config{Agents: []config.AgentConfig{{Name: agentName, Runner: "false"}}}
	dirs := runner.Dirs{HomeBase: homeBase, StateBase: stateBase, ConfigBase: t.TempDir()}

	err := runAgentLoop(agentName, cfg, dirs)
	if err == nil {
		t.Fatal("expected error from failing runtime")
	}
}
