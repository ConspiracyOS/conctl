package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestExecRuntime_Echo(t *testing.T) {
	// Use "cat" as the simplest exec runtime — echoes stdin to stdout
	rt := &Exec{
		Cmd:       "cat",
		Workspace: t.TempDir(),
	}

	output, err := rt.Invoke(context.Background(), "hello from prompt", "test-session")
	if err != nil {
		t.Fatalf("Exec.Invoke failed: %v", err)
	}
	if output != "hello from prompt" {
		t.Errorf("expected prompt echoed back, got %q", output)
	}
}

func TestExecRuntime_WithArgs(t *testing.T) {
	// Use "tr" to transform input — proves args are passed
	rt := &Exec{
		Cmd:       "tr",
		Args:      []string{"a-z", "A-Z"},
		Workspace: t.TempDir(),
	}

	output, err := rt.Invoke(context.Background(), "hello", "test-session")
	if err != nil {
		t.Fatalf("Exec.Invoke failed: %v", err)
	}
	if output != "HELLO" {
		t.Errorf("expected %q, got %q", "HELLO", output)
	}
}

func TestExecRuntime_BadCommand(t *testing.T) {
	rt := &Exec{
		Cmd:       "nonexistent-binary-xyz",
		Workspace: t.TempDir(),
	}

	_, err := rt.Invoke(context.Background(), "test", "test-session")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestExecRuntime_OutputTruncation(t *testing.T) {
	// Generate 2MB of output — should be truncated to 1MB
	rt := &Exec{
		Cmd:       "sh",
		Args:      []string{"-c", "dd if=/dev/zero bs=1024 count=2048 2>/dev/null"},
		Workspace: t.TempDir(),
	}

	output, err := rt.Invoke(context.Background(), "", "test-session")
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	if len(output) > maxOutputSize {
		t.Errorf("output should be truncated to %d bytes, got %d", maxOutputSize, len(output))
	}
	if len(output) < maxOutputSize {
		t.Errorf("output should be at least %d bytes (was fully read), got %d", maxOutputSize, len(output))
	}
}

func TestExecRuntime_StderrOnError(t *testing.T) {
	// Command exits non-zero and writes to stderr — error message should include stderr.
	rt := &Exec{
		Cmd:       "sh",
		Args:      []string{"-c", "echo stderr-output >&2; exit 1"},
		Workspace: t.TempDir(),
	}
	_, err := rt.Invoke(context.Background(), "", "test-session")
	if err == nil {
		t.Fatal("expected error for exit 1")
	}
	if !strings.Contains(err.Error(), "stderr-output") {
		t.Errorf("error should include stderr output, got: %v", err)
	}
}

func TestExecRuntime_ContextCancellation(t *testing.T) {
	// Context expires before sleep finishes — covers the kill-process-group path.
	rt := &Exec{
		Cmd:       "sh",
		Args:      []string{"-c", "sleep 10"},
		Workspace: t.TempDir(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := rt.Invoke(ctx, "", "test-session")
	if err == nil {
		t.Error("expected error from context timeout")
	}
}

func TestNew_PicoClaw(t *testing.T) {
	agent := config.AgentConfig{Name: "test", Runner: "picoclaw"}
	rt := New(agent, "/tmp/test-workspace")
	p, ok := rt.(*PicoClaw)
	if !ok {
		t.Fatal("expected PicoClaw runtime for runner=picoclaw")
	}
	if p.Workspace != "/tmp/test-workspace" {
		t.Errorf("expected workspace forwarded, got %q", p.Workspace)
	}
}

func TestNew_Default(t *testing.T) {
	agent := config.AgentConfig{Name: "test"}
	rt := New(agent, "")
	if _, ok := rt.(*PicoClaw); !ok {
		t.Error("expected PicoClaw runtime for empty runner")
	}
}

func TestNew_Exec(t *testing.T) {
	ws := "/srv/conos/agents/test/workspace"
	agent := config.AgentConfig{Name: "test", Runner: "claude", RunnerArgs: []string{"--model", "claude-opus-4-6", "--print"}}
	rt := New(agent, ws)
	e, ok := rt.(*Exec)
	if !ok {
		t.Fatal("expected Exec runtime for runner=claude")
	}
	if e.Cmd != "claude" {
		t.Errorf("expected cmd=claude, got %q", e.Cmd)
	}
	if len(e.Args) != 3 || e.Args[0] != "--model" || e.Args[2] != "--print" {
		t.Errorf("expected args=[--model claude-opus-4-6 --print], got %v", e.Args)
	}
	if e.Workspace != ws {
		t.Errorf("expected workspace %q, got %q", ws, e.Workspace)
	}
}

func TestExecRuntime_ClaudeStatelessArgs(t *testing.T) {
	rt := &Exec{
		Cmd:             "claude",
		Args:            []string{"--resume", "abc123", "-c", "--model", "sonnet", "--print"},
		SessionStrategy: "stateless",
	}

	got := rt.commandArgs()
	if containsArg(got, "--resume") || containsArg(got, "-c") {
		t.Fatalf("expected session flags stripped, got %v", got)
	}
	if !containsArg(got, "--no-session-persistence") {
		t.Fatalf("expected --no-session-persistence appended, got %v", got)
	}
	if !containsArg(got, "--print") {
		t.Fatalf("expected other args preserved, got %v", got)
	}
}

func TestExecRuntime_ClaudeStatelessArgs_StripsInlineSessionFlags(t *testing.T) {
	rt := &Exec{
		Cmd:             "/usr/local/bin/claude-code",
		Args:            []string{"--resume=abc123", "--fork-session", "--session-id", "11111111-1111-1111-1111-111111111111", "--no-session-persistence", "--print"},
		SessionStrategy: "stateless",
	}

	got := rt.commandArgs()
	for _, forbidden := range []string{
		"--resume=abc123",
		"--fork-session",
		"--session-id",
		"11111111-1111-1111-1111-111111111111",
	} {
		if containsArg(got, forbidden) {
			t.Fatalf("expected %q stripped, got %v", forbidden, got)
		}
	}
	if !containsArg(got, "--no-session-persistence") {
		t.Fatalf("expected --no-session-persistence preserved, got %v", got)
	}
	if countArg(got, "--no-session-persistence") != 1 {
		t.Fatalf("expected exactly one --no-session-persistence, got %v", got)
	}
	if !containsArg(got, "--print") {
		t.Fatalf("expected non-session args preserved, got %v", got)
	}
}

func TestExecRuntime_CommandEnv_PreservesClaudeAuthEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-ant-oat-test-token")
	rt := &Exec{
		Cmd:       "claude",
		APIKeyEnv: "ANTHROPIC_AUTH_TOKEN",
	}

	env := rt.commandEnv()
	if !containsEnv(env, "ANTHROPIC_AUTH_TOKEN=sk-ant-oat-test-token") {
		t.Fatalf("expected Claude auth token preserved for trusted runner, got %v", env)
	}
}

func TestExecRuntime_CommandEnv_PreservesSelectedClaudeOAuthEnvOnly(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "wrong-token")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-token")
	rt := &Exec{
		Cmd:       "claude",
		APIKeyEnv: "CLAUDE_CODE_OAUTH_TOKEN",
	}

	env := rt.commandEnv()
	if !containsEnv(env, "CLAUDE_CODE_OAUTH_TOKEN=oauth-token") {
		t.Fatalf("expected selected Claude OAuth token preserved, got %v", env)
	}
	if containsEnv(env, "ANTHROPIC_AUTH_TOKEN=wrong-token") {
		t.Fatalf("expected conflicting Claude auth token stripped, got %v", env)
	}
}

func TestExecRuntime_CommandEnv_StripsGenericExecAuthEnv(t *testing.T) {
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "sk-ant-oat-test-token")
	rt := &Exec{
		Cmd:       "cat",
		APIKeyEnv: "ANTHROPIC_AUTH_TOKEN",
	}

	env := rt.commandEnv()
	if containsEnv(env, "ANTHROPIC_AUTH_TOKEN=sk-ant-oat-test-token") {
		t.Fatalf("expected generic exec runner to strip auth token, got %v", env)
	}
}

func countArg(args []string, want string) int {
	count := 0
	for _, arg := range args {
		if arg == want {
			count++
		}
	}
	return count
}

func containsEnv(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}
