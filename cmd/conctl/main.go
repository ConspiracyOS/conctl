package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ConspiracyOS/conctl/internal/bootstrap"
	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/contracts"
	"github.com/ConspiracyOS/conctl/internal/runner"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: conctl <command> [args]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  bootstrap      Provision the conspiracy")
		fmt.Fprintln(os.Stderr, "  run <agent>     Run an agent task cycle")
		fmt.Fprintln(os.Stderr, "  route-inbox     Move outer inbox to concierge")
		fmt.Fprintln(os.Stderr, "  healthcheck     Evaluate contracts")
		fmt.Fprintln(os.Stderr, "  task <message>  Drop a task into the outer inbox")
		fmt.Fprintln(os.Stderr, "  status          Show agent status")
		fmt.Fprintln(os.Stderr, "  logs            Show recent audit log entries")
		fmt.Fprintln(os.Stderr, "  responses       Show recent agent responses")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "bootstrap":
		runBootstrap()
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: conctl run <agent-name>")
			os.Exit(1)
		}
		runAgent(os.Args[2])
	case "route-inbox":
		routeInbox()
	case "healthcheck":
		runHealthcheck()
	case "task":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: conctl task <message>")
			os.Exit(1)
		}
		dropTask(os.Args[2])
	case "status":
		showStatus()
	case "logs":
		showLogs()
	case "responses":
		showResponses()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func loadConfig() *config.Config {
	path := "/etc/conos/conos.toml"
	if env := os.Getenv("CONOS_CONFIG"); env != "" {
		path = env
	}
	cfg, err := config.Parse(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func runBootstrap() {
	cfg := loadConfig()
	cmds := bootstrap.PlanProvision(cfg)
	for _, c := range cmds {
		fmt.Printf("+ %s\n", c)
		cmd := exec.Command("sh", "-c", c)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "command failed: %s: %v\n", c, err)
			// Continue — bootstrap is idempotent
		}
	}

	// Write systemd units
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		units := bootstrap.GenerateUnits(resolved)
		for name, content := range units {
			path := "/etc/systemd/system/" + name
			fmt.Printf("+ write %s\n", path)
			os.WriteFile(path, []byte(content), 0644)
		}
	}

	// Write healthcheck timer units
	hcUnits := bootstrap.GenerateHealthcheckUnits(cfg.Contracts.System.HealthcheckInterval)
	for name, content := range hcUnits {
		path := "/etc/systemd/system/" + name
		fmt.Printf("+ write %s\n", path)
		os.WriteFile(path, []byte(content), 0644)
	}

	// Reload systemd and enable units
	exec.Command("systemctl", "daemon-reload").Run()
	exec.Command("systemctl", "enable", "--now", "conos-healthcheck.timer").Run()
	for _, a := range cfg.Agents {
		switch a.Mode {
		case "on-demand", "":
			exec.Command("systemctl", "enable", "--now", "conos-"+a.Name+".path").Run()
		case "continuous":
			exec.Command("systemctl", "enable", "--now", "conos-"+a.Name+".service").Run()
		case "cron":
			exec.Command("systemctl", "enable", "--now", "conos-"+a.Name+".timer").Run()
		}
	}

	// Assemble AGENTS.md for each agent — root-owned, read-only (Linux enforces integrity)
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		if err := runner.AssembleAgentsMD(resolved, runner.DefaultDirs()); err != nil {
			fmt.Fprintf(os.Stderr, "warning: AGENTS.md assembly for %s: %v\n", a.Name, err)
			continue
		}
		homeDir := fmt.Sprintf("/home/a-%s", a.Name)
		exec.Command("chown", "root:root", homeDir+"/AGENTS.md").Run()
		exec.Command("chmod", "0444", homeDir+"/AGENTS.md").Run()
	}

	// Deploy skills to each agent's workspace/skills/
	for _, a := range cfg.Agents {
		user := "a-" + a.Name
		skillsDir := fmt.Sprintf("/srv/conos/agents/%s/workspace/skills", a.Name)
		os.MkdirAll(skillsDir, 0755)

		// Collect skills from roles and agent-specific dirs
		// Outer config: /etc/conos/roles/<role>/skills/, /etc/conos/agents/<name>/skills/
		var sources []string
		for _, r := range a.Roles {
			sources = append(sources, fmt.Sprintf("/etc/conos/roles/%s/skills", r))
		}
		sources = append(sources, fmt.Sprintf("/etc/conos/agents/%s/skills", a.Name))

		for _, src := range sources {
			entries, err := os.ReadDir(src)
			if err != nil {
				continue // dir doesn't exist, skip
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				data, err := os.ReadFile(filepath.Join(src, e.Name()))
				if err != nil {
					continue
				}
				dst := filepath.Join(skillsDir, e.Name())
				os.WriteFile(dst, data, 0644)
				fmt.Printf("+ skill %s -> %s\n", e.Name(), dst)
			}
		}

		// Fix ownership
		exec.Command("chown", "-R", user+":agents", skillsDir).Run()
	}

	fmt.Println("bootstrap complete")
}

func routeInbox() {
	if err := runner.MoveOuterInboxTasks(runner.DefaultDirs()); err != nil {
		fmt.Fprintf(os.Stderr, "route-inbox failed: %v\n", err)
		os.Exit(1)
	}
}

func runHealthcheck() {
	contractsDir := "/srv/conos/contracts"
	if env := os.Getenv("CONOS_CONTRACTS_DIR"); env != "" {
		contractsDir = env
	}
	logPath := "/srv/conos/logs/audit/contracts.log"
	if err := healthcheckIn(contractsDir, logPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func healthcheckIn(contractsDir, logPath string) error {
	allContracts, err := contracts.LoadDir(contractsDir)
	if err != nil {
		return fmt.Errorf("healthcheck: loading contracts: %w", err)
	}

	if len(allContracts) == 0 {
		fmt.Println("healthcheck: no contracts found")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := contracts.Evaluate(ctx, allContracts, contractsDir, &contracts.DefaultExecutor{})

	// Write to log file
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		contracts.WriteLog(result, f)
		f.Close()
	}

	// Also write to stdout (for journalctl)
	contracts.WriteLog(result, os.Stdout)

	// Dispatch failure actions
	for _, cr := range result.Results {
		if cr.Passed {
			continue
		}
		for _, c := range allContracts {
			for _, ch := range c.Checks {
				if c.ID == cr.ContractID && ch.Name == cr.CheckName {
					cmds, err := contracts.DispatchAction(ctx, ch.OnFail, c.Scope, &contracts.DefaultExecutor{})
					if err != nil {
						fmt.Fprintf(os.Stderr, "healthcheck: action dispatch for %s: %v\n", c.ID, err)
					}
					for _, cmd := range cmds {
						fmt.Printf("  ACTION: %s\n", cmd)
					}
				}
			}
		}
	}

	// Meta-escalation: if any contracts failed, send one summary task to sysadmin
	if result.Failed > 0 {
		var failures []string
		for _, cr := range result.Results {
			if !cr.Passed {
				failures = append(failures, fmt.Sprintf("%s/%s", cr.ContractID, cr.CheckName))
			}
		}
		msg := fmt.Sprintf("Healthcheck: %d contract(s) failed: %s. Review audit log and fix.", result.Failed, strings.Join(failures, ", "))
		if err := contracts.Escalate("sysadmin", msg); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: escalation failed: %v\n", err)
		}
		return fmt.Errorf("healthcheck: %d contract(s) failed", result.Failed)
	}
	return nil
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
func runAgentLoop(name string, cfg *config.Config, dirs runner.Dirs) error {
	for {
		if err := runner.Run(name, cfg, dirs); err != nil {
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
func dropTask(message string) {
	if err := dropTaskTo("/srv/conos/inbox", message); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write task: %v\n", err)
		os.Exit(1)
	}
}

func dropTaskTo(inbox, message string) error {
	taskID := fmt.Sprintf("%d", time.Now().Unix())
	taskPath := filepath.Join(inbox, taskID+".task")
	if err := os.WriteFile(taskPath, []byte(message), 0644); err != nil {
		return err
	}
	fmt.Printf("Task %s.task dropped into inbox\n", taskID)
	return nil
}

func showStatus() {
	if err := showStatusIn("/srv/conos/agents"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func showStatusIn(agentsDir string) error {
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

func showLogs() {
	showLogsFrom("/srv/conos/logs/audit")
}

func showLogsFrom(auditDir string) {
	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(auditDir, today+".log")

	data, err := os.ReadFile(logPath)
	if err != nil {
		data, err = os.ReadFile(filepath.Join(auditDir, "contracts.log"))
		if err != nil {
			fmt.Println("No audit logs found for today")
			return
		}
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	start := 0
	if len(lines) > 20 {
		start = len(lines) - 20
	}
	for _, line := range lines[start:] {
		fmt.Println(line)
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
