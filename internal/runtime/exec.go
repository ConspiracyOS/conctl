package runtime

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// maxOutputSize limits stdout capture to prevent OOM from runaway CLI output.
const maxOutputSize = 1 << 20 // 1MB

var claudeAuthEnvVars = []string{
	"ANTHROPIC_API_KEY",
	"ANTHROPIC_AUTH_TOKEN",
	"CLAUDE_CODE_OAUTH_TOKEN",
}

// Exec runs an agent using an external CLI binary.
// The prompt is passed via stdin. The response is read from stdout.
type Exec struct {
	Cmd             string
	Args            []string
	Workspace       string
	APIKeyEnv       string // per-agent API key env var to filter from child environment
	SessionStrategy string
	RunAsUser       string // drop privileges to this user before exec (e.g. "a-concierge")
	SettingsFile    string // path to runner settings file (e.g. for Claude --settings)
}

// Invoke runs the configured CLI, passing prompt via stdin and capturing stdout.
// sessionKey is accepted for interface compatibility but not forwarded — external
// CLIs manage their own session state.
func (e *Exec) Invoke(ctx context.Context, prompt, sessionKey string) (string, error) {
	cmd := exec.CommandContext(ctx, e.Cmd, e.commandArgs()...)
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Dir = e.Workspace
	cmd.Env = e.commandEnv()
	sysProcAttr := &syscall.SysProcAttr{Setpgid: true}
	if e.RunAsUser != "" {
		if cred, err := lookupCredential(e.RunAsUser); err == nil {
			sysProcAttr.Credential = cred
		}
	}
	cmd.SysProcAttr = sysProcAttr

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Kill entire process group on context cancellation (child processes survive
	// a regular SIGKILL to the parent).
	if ctx.Err() != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	if err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("exec runtime %s: %w\nstderr: %s", e.Cmd, err, stderr.String())
		}
		return "", fmt.Errorf("exec runtime %s: %w", e.Cmd, err)
	}

	output := stdout.Bytes()
	if len(output) > maxOutputSize {
		output = output[:maxOutputSize]
	}
	if err := ValidateAdapterExecution(AdapterContract{
		Name:           e.Cmd,
		RequireRunID:   false,
		MaxOutputBytes: maxOutputSize,
	}, AdapterExecution{
		RunID:    sessionKey,
		ExitCode: 0,
		Stdout:   output,
	}); err != nil {
		return "", err
	}
	return string(output), nil
}

func (e *Exec) commandEnv() []string {
	switch strings.ToLower(filepath.Base(e.Cmd)) {
	case "claude", "claude-code":
		return claudeCommandEnv(e.APIKeyEnv)
	default:
		return SanitizedEnv(e.APIKeyEnv)
	}
}

func (e *Exec) commandArgs() []string {
	args := append([]string(nil), e.Args...)

	// Inject --settings if configured and not already present
	if e.SettingsFile != "" && !containsArg(args, "--settings") {
		args = append(args, "--settings", e.SettingsFile)
	}

	if e.SessionStrategy != "stateless" {
		return args
	}

	switch strings.ToLower(filepath.Base(e.Cmd)) {
	case "claude", "claude-code":
		args = stripClaudeSessionArgs(args)
		if !containsArg(args, "--no-session-persistence") {
			args = append(args, "--no-session-persistence")
		}
	}

	return args
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func stripClaudeSessionArgs(args []string) []string {
	var filtered []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--continue", args[i] == "-c", args[i] == "--fork-session":
			continue
		case args[i] == "--session-id":
			if i+1 < len(args) {
				i++
			}
			continue
		case args[i] == "--resume", args[i] == "-r":
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		case strings.HasPrefix(args[i], "--resume="),
			strings.HasPrefix(args[i], "--session-id="),
			strings.HasPrefix(args[i], "--continue="),
			strings.HasPrefix(args[i], "--fork-session="):
			continue
		}
		filtered = append(filtered, args[i])
	}
	return filtered
}

func claudeCommandEnv(selected string) []string {
	filtered := make([]string, 0, len(claudeAuthEnvVars)+1)
	filtered = append(filtered, claudeAuthEnvVars...)
	if selected != "" && !containsString(filtered, selected) {
		filtered = append(filtered, selected)
	}

	env := SanitizedEnv(filtered...)
	if selected == "" {
		return env
	}
	value := os.Getenv(selected)
	if value == "" {
		return env
	}
	return append(env, selected+"="+value)
}

// lookupCredential resolves a username to a syscall.Credential for privilege dropping.
func lookupCredential(username string) (*syscall.Credential, error) {
	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("lookup user %q: %w", username, err)
	}
	uid, _ := strconv.ParseUint(u.Uid, 10, 32)
	gid, _ := strconv.ParseUint(u.Gid, 10, 32)
	return &syscall.Credential{
		Uid: uint32(uid),
		Gid: uint32(gid),
	}, nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
