package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ConspiracyOS/conctl/internal/bootstrap"
	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/runner"
	conruntime "github.com/ConspiracyOS/conctl/internal/runtime"
)

type logOpts struct {
	n      int
	follow bool
	agent  string
}

func runBootstrap() {
	fs := flag.NewFlagSet("bootstrap", flag.ExitOnError)
	sidecar := fs.Bool("sidecar", false, "sidecar mode: coexist with existing OS")
	prune := fs.Bool("prune", false, "remove agents/units not in current config")
	fs.Parse(os.Args[2:])

	cfg := loadConfig()
	opts := bootstrap.BootstrapOptions{}
	if *sidecar {
		opts.Mode = bootstrap.ModeSidecar
	}
	m := bootstrap.FromConfig(cfg, opts)
	cmds := bootstrap.ProvisionFromManifest(m)
	for _, c := range cmds {
		fmt.Printf("+ %s\n", c)
		cmd := exec.Command("sh", "-c", c)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "command failed: %s: %v\n", c, err)
		}
	}

	// AGENTS.md assembly — needs runner package, can't be a shell command
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		if err := runner.AssembleAgentsMD(resolved, runner.DefaultDirs()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: AGENTS.md assembly for %s: %v\n", a.Name, err)
		}
	}

	if *prune {
		pruneCmds := bootstrap.PruneCommands(cfg)
		for _, c := range pruneCmds {
			fmt.Printf("+ [prune] %s\n", c)
			cmd := exec.Command("sh", "-c", c)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "prune command failed: %s: %v\n", c, err)
			}
		}
	}

	fmt.Println("bootstrap complete")
}

func routeInbox() {
	if err := runner.MoveOuterInboxTasks(runner.DefaultDirs()); err != nil {
		fmt.Fprintf(os.Stderr, "route-inbox failed: %v\n", err)
		os.Exit(1)
	}
}

func runAgent(name string) {
	cfg := loadConfig()
	if err := runAgentLoop(name, cfg, runner.DefaultDirs()); err != nil {
		fmt.Fprintf(os.Stderr, "run failed: %v\n", err)
		os.Exit(1)
	}
}

// runAgentLoop drains the agent's inbox until it is empty, then returns.
// Returns nil when the inbox is drained; returns an error on runtime failure.
// The runtime is created once per loop so that state cached on the first Invoke
// call (e.g. resolved API key) is preserved across all tasks in the run.
func runAgentLoop(name string, cfg *config.Config, dirs runner.Dirs) error {
	agent := cfg.ResolvedAgent(name)
	workspaceDir := filepath.Join(dirs.StateBase, "agents", name, "workspace")
	rt := conruntime.New(agent, workspaceDir)
	for {
		if err := runner.RunWithRuntime(name, cfg, dirs, rt); err != nil {
			if strings.Contains(err.Error(), "no tasks in inbox") {
				return nil // Inbox drained
			}
			return err
		}
	}
}

// dropTask writes a task file to the outer inbox. File ownership determines
// trust level: run as a member of the "trusted" group (or root) for verified
// framing. See internal/runner/runner.go isTrustedUID.
func dropTask(message string, meta runner.TaskMetadata) {
	if err := dropTaskTo("/srv/conos/inbox", message, meta); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write task: %v\n", err)
		os.Exit(1)
	}
}

func dropTaskTo(inbox, message string, meta runner.TaskMetadata) error {
	taskID, err := runner.WriteTaskWithMetadata(inbox, message, meta)
	if err != nil {
		return err
	}
	fmt.Printf("Task %s.task dropped into inbox\n", taskID)
	return nil
}

func dropTaskToAgent(agentsDir, agentName, message string, meta runner.TaskMetadata) error {
	inbox := filepath.Join(agentsDir, agentName, "inbox")
	if _, err := os.Stat(inbox); err != nil {
		return fmt.Errorf("agent %q not found (no inbox at %s)", agentName, inbox)
	}
	// Try direct write to agent inbox first. If permission denied (sidecar mode,
	// host user not in agents group), fall back to the outer inbox which is
	// group-writable and routes through concierge.
	_, err := runner.WriteTaskWithMetadata(inbox, message, meta)
	if err != nil && os.IsPermission(err) {
		outerInbox := "/srv/conos/inbox"
		if _, serr := os.Stat(outerInbox); serr == nil {
			// Prefix message with routing hint so concierge knows the target
			routed := fmt.Sprintf("---ROUTE_TO: %s---\n\n%s", agentName, message)
			_, err = runner.WriteTaskWithMetadata(outerInbox, routed, meta)
			if err == nil {
				fmt.Fprintf(os.Stderr, "note: delivered via outer inbox (no direct access to %s inbox)\n", agentName)
			}
		}
	}
	return err
}

func showStatus() {
	if err := showStatusInWithPreflight("/srv/conos/agents", "/etc/conos/conos.toml", "/etc/conos/env"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func showStatusIn(agentsDir string) error {
	return showStatusInWithPreflight(agentsDir, "", "")
}

func showStatusInWithPreflight(agentsDir, configPath, envPath string) error {
	for _, w := range authPreflightWarnings(configPath, envPath) {
		fmt.Printf("WARN: %s\n", w)
	}

	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return fmt.Errorf("cannot read agents dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		inboxPath := filepath.Join(agentsDir, name, "inbox")
		tasks, _ := os.ReadDir(inboxPath)
		pending := 0
		for _, t := range tasks {
			if strings.HasSuffix(t.Name(), ".task") {
				pending++
			}
		}

		state := "inactive"
		for _, suffix := range []string{".path", ".service", ".timer"} {
			out, err := exec.Command("systemctl", "is-active", "conos-"+name+suffix).Output()
			if err == nil && strings.TrimSpace(string(out)) == "active" {
				state = "active"
				break
			}
		}

		fmt.Printf("%-20s %s  (%d pending)\n", name, state, pending)
	}
	return nil
}

func authPreflightWarnings(configPath, envPath string) []string {
	if configPath == "" {
		return nil
	}
	cfg, err := config.Parse(configPath)
	if err != nil {
		return nil
	}

	env := map[string]string{}
	for _, kv := range os.Environ() {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	for k, v := range parseSimpleEnvFile(envPath) {
		env[k] = v
	}
	return collectAuthWarnings(cfg, env)
}

func collectAuthWarnings(cfg *config.Config, env map[string]string) []string {
	if cfg == nil {
		return nil
	}
	var out []string
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		// Preflight is scoped to picoclaw+anthropic where we observed runtime 401s.
		if resolved.Provider != "anthropic" {
			continue
		}
		if resolved.Runner != "" && resolved.Runner != "picoclaw" {
			continue
		}

		keyEnv := resolved.APIKeyEnv
		if keyEnv == "" {
			keyEnv = "CONOS_API_KEY"
		}
		key := env[keyEnv]
		if key == "" {
			key = env["CONOS_AUTH_ANTHROPIC"]
		}
		if key == "" {
			out = append(out, fmt.Sprintf("%s: missing Anthropic credential (checked %s, CONOS_AUTH_ANTHROPIC)", a.Name, keyEnv))
			continue
		}
		if strings.HasPrefix(key, "sk-ant-oat") {
			out = append(out, fmt.Sprintf("%s: Anthropic OAuth token detected in %s; Messages API may reject OAuth (401). Use a standard API key instead.", a.Name, keyEnv))
		}
	}
	return out
}

func parseSimpleEnvFile(path string) map[string]string {
	out := map[string]string{}
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		out[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return out
}

func parseLogOpts(args []string) *logOpts {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "follow the log (stream)")
	n := fs.Int("n", 20, "number of lines to show")
	fs.Parse(args)
	agent := ""
	if fs.NArg() > 0 {
		agent = fs.Arg(0)
	}
	return &logOpts{n: *n, follow: *follow, agent: agent}
}

func showLogsWithOpts(opts *logOpts) {
	showLogsFrom("/srv/conos/logs/audit", opts)
}

func showLogsFrom(auditDir string, opts *logOpts) {
	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(auditDir, today+".log")

	if opts.follow {
		// Use tail -f — simplest correct approach
		cmd := exec.Command("tail", "-f", "-n", fmt.Sprintf("%d", opts.n), logPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
		return
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		// Fallback to contracts.log (preserve existing behavior)
		data, err = os.ReadFile(filepath.Join(auditDir, "contracts.log"))
		if err != nil {
			fmt.Println("No audit logs found for today")
			return
		}
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	if opts.agent != "" {
		var matched []string
		for _, line := range lines {
			if strings.Contains(line, "["+opts.agent+"]") || strings.Contains(line, " "+opts.agent+" ") {
				matched = append(matched, line)
			}
		}
		lines = matched
	}

	start := 0
	if len(lines) > opts.n {
		start = len(lines) - opts.n
	}
	for _, l := range lines[start:] {
		fmt.Println(l)
	}
}

func showResponses() {
	if err := showResponsesIn("/srv/conos/agents"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func showResponsesIn(agentsDir string) error {
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return fmt.Errorf("cannot read agents dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		outboxPath := filepath.Join(agentsDir, name, "outbox")
		files, err := os.ReadDir(outboxPath)
		if err != nil {
			continue
		}

		var responses []string
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".response") {
				responses = append(responses, f.Name())
			}
		}
		if len(responses) == 0 {
			continue
		}

		sort.Strings(responses)
		latest := responses[len(responses)-1]
		data, err := os.ReadFile(filepath.Join(outboxPath, latest))
		if err != nil {
			continue
		}

		fmt.Printf("=== %s: %s ===\n", name, latest)
		content := string(data)
		if len(content) > 500 {
			content = content[:500] + "\n... (truncated)"
		}
		fmt.Println(content)
		fmt.Println()
	}
	return nil
}

func killAgentUnits(name string) error {
	var lastErr error
	for _, suffix := range []string{".path", ".service", ".timer"} {
		unit := "conos-" + name + suffix
		if err := exec.Command("systemctl", "stop", unit).Run(); err != nil {
			lastErr = err // unit may not exist; continue
		}
	}
	fmt.Printf("killed %s\n", name)
	return lastErr
}

// clearSessions removes PicoClaw session files for the named agent,
// or for all agents if name is empty.
func clearSessions(name string) error {
	agentsDir := "/srv/conos/agents"
	if name != "" {
		return clearAgentSessions(filepath.Join(agentsDir, name))
	}
	entries, err := os.ReadDir(agentsDir)
	if err != nil {
		return fmt.Errorf("cannot read agents dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := clearAgentSessions(filepath.Join(agentsDir, e.Name())); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", e.Name(), err)
		}
	}
	return nil
}

func clearAgentSessions(agentDir string) error {
	sessionsDir := filepath.Join(agentDir, "workspace", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cleared := 0
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if err := os.Remove(filepath.Join(sessionsDir, e.Name())); err != nil {
			return err
		}
		cleared++
	}
	agent := filepath.Base(agentDir)
	fmt.Printf("%s: cleared %d session(s)\n", agent, cleared)
	return nil
}
