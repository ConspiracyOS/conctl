package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"

	"github.com/ConspiracyOS/conctl/internal/artifacts"
	"github.com/ConspiracyOS/conctl/internal/strutil"
	"github.com/ConspiracyOS/conctl/internal/bootstrap"
	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/contracts"
	"github.com/ConspiracyOS/conctl/internal/runner"
	conruntime "github.com/ConspiracyOS/conctl/internal/runtime"

	"gopkg.in/yaml.v3"
)

type logOpts struct {
	n      int
	follow bool
	agent  string
}

var (
	artifactCreateRoot = "/srv/conos/artifacts"
	artifactShowRoot   = "/srv/conos/artifacts"
	artifactStatusRoot = "/srv/conos/status"
	taskContractsRoot  = "/srv/conos/policy/task-contracts"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: conctl <command> [args]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  bootstrap                         Provision the conspiracy")
		fmt.Fprintln(os.Stderr, "  run <agent>                       Run an agent task cycle")
		fmt.Fprintln(os.Stderr, "  route-inbox                       Move outer inbox to concierge")
		fmt.Fprintln(os.Stderr, "  healthcheck                       Evaluate contracts")
		fmt.Fprintln(os.Stderr, "  doctor                            Render system doctor report from contracts")
		fmt.Fprintln(os.Stderr, "  artifact create|show|link|verify  Manage user-facing artifacts")
		fmt.Fprintln(os.Stderr, "  artifact-auth                     Start nginx auth_request backend")
		fmt.Fprintln(os.Stderr, "  task-contract open|claim|update|show  Manage contract-backed tasks")
		fmt.Fprintln(os.Stderr, "  task [--agent <name>] <message>   Drop task into inbox")
		fmt.Fprintln(os.Stderr, "  status                            Show agent status")
		fmt.Fprintln(os.Stderr, "  logs [-f] [-n N] [agent]          Show/stream audit log")
		fmt.Fprintln(os.Stderr, "  responses                         Show recent agent responses")
		fmt.Fprintln(os.Stderr, "  manifest show                     Dump expected system state as YAML")
		fmt.Fprintln(os.Stderr, "  kill <agent>                      Stop a running agent's systemd units")
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
	case "doctor":
		runDoctor()
	case "artifact":
		runArtifact(os.Args[2:])
	case "artifact-auth":
		runArtifactAuth(os.Args[2:])
	case "task-contract":
		runTaskContract(os.Args[2:])
	case "task":
		fs := flag.NewFlagSet("task", flag.ExitOnError)
		agentName := fs.String("agent", "", "send directly to this agent's inbox")
		fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "usage: conctl task [--agent <name>] <message>")
			os.Exit(1)
		}
		message := fs.Arg(0)
		if *agentName != "" {
			if err := dropTaskToAgent("/srv/conos/agents", *agentName, message); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else {
			dropTask(message)
		}
	case "status":
		showStatus()
	case "logs":
		opts := parseLogOpts(os.Args[2:])
		showLogsWithOpts(opts)
	case "responses":
		showResponses()
	case "manifest":
		runManifest(os.Args[2:])
	case "kill":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: conctl kill <agent-name>")
			os.Exit(1)
		}
		if err := killAgentUnits(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func runArtifact(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact <create|show> [args]")
		os.Exit(1)
	}
	switch args[0] {
	case "create":
		artifactCreate(args[1:])
	case "show":
		artifactShow(args[1:])
	case "link":
		artifactLink(args[1:])
	case "verify":
		artifactVerify(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown artifact command: %s\n", args[0])
		os.Exit(1)
	}
}

func runTaskContract(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conctl task-contract <open|claim|update|show> [args]")
		os.Exit(1)
	}
	switch args[0] {
	case "open":
		taskContractOpen(args[1:])
	case "claim":
		taskContractClaim(args[1:])
	case "update":
		taskContractUpdate(args[1:])
	case "show":
		taskContractShow(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown task-contract command: %s\n", args[0])
		os.Exit(1)
	}
}

func taskContractOpen(args []string) {
	fs := flag.NewFlagSet("task-contract open", flag.ExitOnError)
	id := fs.String("id", "", "task contract id")
	title := fs.String("title", "", "title")
	description := fs.String("description", "", "description")
	actor := fs.String("actor", os.Getenv("CONOS_ACTOR"), "actor")
	runID := fs.String("run-id", os.Getenv("CONOS_RUN_ID"), "run id")
	completionChecks := fs.String("completion-checks", "", "comma-separated contract IDs for auto-completion")
	fs.Parse(args)
	var checks []string
	if *completionChecks != "" {
		for _, s := range strings.Split(*completionChecks, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				checks = append(checks, s)
			}
		}
	}
	task, err := contracts.OpenTaskContract(taskContractsRoot, contracts.TaskContractInput{
		ID:               *id,
		Title:            *title,
		Description:      *description,
		Actor:            *actor,
		RunID:            *runID,
		CompletionChecks: checks,
	}, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(task, "", "  ")
	fmt.Println(string(data))
}

func taskContractClaim(args []string) {
	fs := flag.NewFlagSet("task-contract claim", flag.ExitOnError)
	id := fs.String("id", "", "task contract id")
	owner := fs.String("owner", "", "owner")
	runID := fs.String("run-id", os.Getenv("CONOS_RUN_ID"), "run id")
	lease := fs.Duration("lease", 5*time.Minute, "lease duration")
	fs.Parse(args)
	claim, err := contracts.ClaimTask(filepath.Join(taskContractsRoot, ".claims.json"), *id, *owner, *runID, *lease, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	task, err := contracts.UpdateTaskContract(taskContractsRoot, *id, "in_progress", *owner, *owner, *runID, "claimed", time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	out := map[string]any{"claim": claim, "task": task}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

func taskContractUpdate(args []string) {
	fs := flag.NewFlagSet("task-contract update", flag.ExitOnError)
	status := fs.String("status", "", "status")
	owner := fs.String("owner", "", "owner")
	actor := fs.String("actor", os.Getenv("CONOS_ACTOR"), "actor")
	runID := fs.String("run-id", os.Getenv("CONOS_RUN_ID"), "run id")
	message := fs.String("message", "", "history message")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl task-contract update [flags] <id>")
		os.Exit(1)
	}
	task, err := contracts.UpdateTaskContract(taskContractsRoot, fs.Arg(0), *status, *owner, *actor, *runID, *message, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(task, "", "  ")
	fmt.Println(string(data))
}

func taskContractShow(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl task-contract show <id>")
		os.Exit(1)
	}
	task, err := contracts.ShowTaskContract(taskContractsRoot, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(task, "", "  ")
	fmt.Println(string(data))
}

func artifactCreate(args []string) {
	fs := flag.NewFlagSet("artifact create", flag.ExitOnError)
	title := fs.String("title", "", "artifact title")
	kind := fs.String("kind", "file", "artifact kind")
	contentType := fs.String("content-type", "", "content type")
	filename := fs.String("name", "", "file name")
	fromPath := fs.String("from", "", "read artifact content from file")
	createdBy := fs.String("created-by", os.Getenv("CONOS_ACTOR"), "artifact creator")
	runID := fs.String("run-id", os.Getenv("CONOS_RUN_ID"), "run identifier")
	taskID := fs.String("task-id", "", "task identifier")
	audience := fs.String("audience", "user", "artifact audience")
	exposure := fs.String("exposure", string(artifacts.ExposureDashboardLocal), "private|dashboard_local|authenticated_dashboard")
	fs.Parse(args)

	var content []byte
	var err error
	if *fromPath != "" {
		content, err = os.ReadFile(*fromPath)
	} else {
		content, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	artifact, err := artifacts.Create(artifacts.CreateInput{
		ArtifactsRoot: artifactCreateRoot,
		StatusRoot:    artifactStatusRoot,
		Title:         *title,
		Kind:          *kind,
		ContentType:   *contentType,
		Filename:      *filename,
		CreatedBy:     *createdBy,
		RunID:         *runID,
		TaskID:        *taskID,
		Audience:      *audience,
		Exposure:      artifacts.Exposure(*exposure),
		Content:       content,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(artifact, "", "  ")
	fmt.Println(string(data))
}

func artifactShowIn(root, id string) (*artifacts.Artifact, error) {
	return artifacts.Show(root, id)
}

func artifactShow(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact show <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(artifact, "", "  ")
	fmt.Println(string(data))
}

func artifactLink(args []string) {
	fs := flag.NewFlagSet("artifact link", flag.ExitOnError)
	baseURL := fs.String("base-url", strings.TrimRight(os.Getenv("CONOS_BASE_URL"), "/"), "base URL for the artifact link")
	ttl := fs.Duration("ttl", time.Hour, "signed link TTL")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact link [--base-url URL] [--ttl 1h] <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	link, err := artifacts.MintSignedLink(*baseURL, artifact, bytesTrimSpace(secret), *ttl, time.Now().UTC())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := artifacts.Save(artifact); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data, _ := json.MarshalIndent(link, "", "  ")
	fmt.Println(string(data))
}

func artifactVerify(args []string) {
	fs := flag.NewFlagSet("artifact verify", flag.ExitOnError)
	exp := fs.String("exp", "", "expiry unix timestamp")
	sig := fs.String("sig", "", "hex signature")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	fs.Parse(args)
	if fs.NArg() != 1 || *exp == "" || *sig == "" {
		fmt.Fprintln(os.Stderr, "usage: conctl artifact verify --exp <unix> --sig <hex> <artifact-id>")
		os.Exit(1)
	}
	artifact, err := artifactShowIn(artifactShowRoot, fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := artifacts.VerifySignedLink(artifact, bytesTrimSpace(secret), *exp, *sig, time.Now().UTC()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("ok")
}

func runArtifactAuth(args []string) {
	fs := flag.NewFlagSet("artifact-auth", flag.ExitOnError)
	socketPath := fs.String("socket", artifacts.DefaultAuthSocket, "Unix socket path")
	secretFile := fs.String("secret-file", strutil.FirstNonEmpty(os.Getenv("CONOS_ARTIFACT_SIGNING_KEY_FILE"), "/etc/conos/artifact-signing.key"), "path to HMAC signing key")
	root := fs.String("artifacts-root", artifactShowRoot, "artifacts root directory")
	fs.Parse(args)

	if err := artifacts.ListenAndServeAuth(artifacts.AuthConfig{
		SocketPath:    *socketPath,
		SecretFile:    *secretFile,
		ArtifactsRoot: *root,
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "warning: writing %s: %v\n", path, err)
			}
		}
	}

	// Write healthcheck timer units
	hcUnits := bootstrap.GenerateHealthcheckUnits(cfg.Contracts.System.HealthcheckInterval)
	for name, content := range hcUnits {
		path := "/etc/systemd/system/" + name
		fmt.Printf("+ write %s\n", path)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: writing %s: %v\n", path, err)
		}
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
				if err := os.WriteFile(dst, data, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "warning: writing %s: %v\n", dst, err)
				}
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

	var briefOutput string
	if cfgPath := os.Getenv("CONOS_CONFIG"); cfgPath != "" {
		if cfg, err := config.Parse(cfgPath); err == nil {
			briefOutput = cfg.Contracts.BriefOutput
		}
	} else if cfg, err := config.Parse("/etc/conos/conos.toml"); err == nil {
		briefOutput = cfg.Contracts.BriefOutput
	}

	opts := contracts.EvalOptions{
		RunID: os.Getenv("CONOS_RUN_ID"),
		Actor: os.Getenv("CONOS_ACTOR"),
	}
	if opts.RunID == "" {
		opts.RunID = fmt.Sprintf("hc-%d", time.Now().Unix())
	}
	if opts.Actor == "" {
		opts.Actor = "conctl:healthcheck"
	}

	if err := healthcheckInWithOptions(contractsDir, logPath, briefOutput, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func healthcheckIn(contractsDir, logPath, briefOutput string) error {
	return healthcheckInWithOptions(contractsDir, logPath, briefOutput, contracts.EvalOptions{})
}

func healthcheckInWithOptions(contractsDir, logPath, briefOutput string, opts contracts.EvalOptions) error {
	allContracts, err := sharedcontracts.LoadDir(contractsDir)
	if err != nil {
		return fmt.Errorf("healthcheck: loading contracts: %w", err)
	}

	if len(allContracts) == 0 {
		fmt.Println("healthcheck: no contracts found")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Run audit in a goroutine so the context timeout covers contract evaluation.
	// RunAudit doesn't accept a context, so we use select to enforce the deadline.
	type auditResult struct {
		result sharedcontracts.AuditResult
	}
	ch := make(chan auditResult, 1)
	go func() {
		r := sharedcontracts.RunAudit(allContracts, selectedContractTags(), contractsDir)
		ch <- auditResult{result: r}
	}()

	var sharedResult sharedcontracts.AuditResult
	select {
	case ar := <-ch:
		sharedResult = ar.result
	case <-ctx.Done():
		return fmt.Errorf("healthcheck: contract evaluation timed out after 120s")
	}
	result := contracts.FromSharedAudit(sharedResult, opts)

	// Write to log file
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		contracts.WriteLog(result, f)
		f.Close()
	}

	// Also write to stdout (for journalctl)
	contracts.WriteLog(result, os.Stdout)

	// Persist a full state snapshot and append-only failure diff log.
	if err := contracts.PersistSnapshotAndDiff(filepath.Dir(logPath), result); err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: snapshot persistence failed: %v\n", err)
	}

	// Dispatch failure actions
	contractIndex := make(map[string]*sharedcontracts.Contract, len(allContracts))
	for _, c := range allContracts {
		contractIndex[c.ID] = c
	}
	for _, cr := range result.Results {
		if cr.Status == "pass" || cr.Status == "skip" || cr.Status == "exempt" {
			continue
		}
		c := contractIndex[cr.ContractID]
		if c == nil {
			continue
		}
		for _, ch := range c.Checks {
			if ch.Name == cr.CheckName {
				cmds, err := contracts.DispatchAction(ctx, contracts.FailAction{
					Action: string(ch.OnFail),
				}, "global", &contracts.DefaultExecutor{})
				if err != nil {
					fmt.Fprintf(os.Stderr, "healthcheck: action dispatch for %s: %v\n", c.ID, err)
				}
				for _, cmd := range cmds {
					fmt.Printf("  ACTION: %s\n", cmd)
				}
			}
		}
	}

	// Write system-state.md for operator agents (configurable via contracts.brief_output)
	if briefOutput != "" {
		if err := writeBriefOutput(briefOutput, result); err != nil {
			fmt.Fprintf(os.Stderr, "healthcheck: writing brief output: %v\n", err)
		}
	}

	// Auto-complete task-contracts whose completion predicates are satisfied
	completedTasks, tcErr := contracts.EvaluateTaskCompletions(taskContractsRoot, result, opts.Actor, time.Now())
	if tcErr != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: task completion eval: %v\n", tcErr)
	}
	for _, id := range completedTasks {
		fmt.Printf("  TASK-COMPLETE: %s\n", id)
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

func runManifest(args []string) {
	if len(args) == 0 || args[0] != "show" {
		fmt.Fprintln(os.Stderr, "usage: conctl manifest show")
		os.Exit(1)
	}
	cfg := loadConfig()
	m := bootstrap.FromConfig(cfg)
	data, err := yaml.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(data))
}

func runDoctor() {
	contractsDir := "/srv/conos/contracts"
	if env := os.Getenv("CONOS_CONTRACTS_DIR"); env != "" {
		contractsDir = env
	}

	allContracts, err := sharedcontracts.LoadDir(contractsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "doctor: loading contracts: %v\n", err)
		os.Exit(1)
	}
	opts := contracts.EvalOptions{
		RunID: os.Getenv("CONOS_RUN_ID"),
		Actor: os.Getenv("CONOS_ACTOR"),
	}
	if opts.RunID == "" {
		opts.RunID = fmt.Sprintf("doctor-%d", time.Now().Unix())
	}
	if opts.Actor == "" {
		opts.Actor = "conctl:doctor"
	}

	sharedResult := sharedcontracts.RunAudit(allContracts, selectedContractTags(), contractsDir)
	result := contracts.FromSharedAudit(sharedResult, opts)
	fmt.Print(contracts.FormatDoctorReport(result))

	// Manifest verification
	cfg := loadConfig()
	m := bootstrap.FromConfig(cfg)
	findings := bootstrap.VerifyLocal(m)
	manifestFails := 0
	for _, f := range findings {
		if f.Status == "fail" {
			manifestFails++
			fmt.Printf("MANIFEST %s [%s] %s\n", f.Status, f.Severity, f.What)
		}
	}
	if manifestFails > 0 {
		fmt.Fprintf(os.Stderr, "\nmanifest: %d finding(s)\n", manifestFails)
	}

	if result.Failed > 0 {
		os.Exit(1)
	}
}

func selectedContractTags() []string {
	if raw := strings.TrimSpace(os.Getenv("CONOS_CONTRACT_TAGS")); raw != "" {
		var tags []string
		for _, p := range strings.Split(raw, ",") {
			tag := strings.TrimSpace(p)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
		if len(tags) > 0 {
			return tags
		}
	}
	return []string{"schedule"}
}

// writeBriefOutput writes a markdown system-state summary to path.
// This file is injected into operator-tier agent prompts as ambient context.
func writeBriefOutput(path string, result contracts.RunResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("# System State\n\n")
	b.WriteString(fmt.Sprintf("_Updated: %s_\n\n", result.Timestamp.UTC().Format(time.RFC3339)))

	if result.Failed == 0 {
		b.WriteString(fmt.Sprintf("All %d checks passed.\n", result.Passed))
	} else {
		b.WriteString(fmt.Sprintf("Checked %d contracts. **%d failed**, %d passed.\n\n",
			result.Passed+result.Failed, result.Failed, result.Passed))
		b.WriteString("## Failed Checks\n\n")
		for _, cr := range result.Results {
			if cr.Passed {
				continue
			}
			b.WriteString(fmt.Sprintf("### %s: %s\n\n", cr.ContractID, cr.CheckName))
			if cr.Output != "" {
				b.WriteString(fmt.Sprintf("**Output:** %s\n\n", cr.Output))
			}
			if cr.Error != nil {
				b.WriteString(fmt.Sprintf("**Error:** %v\n\n", cr.Error))
			}
		}
	}

	return os.WriteFile(path, []byte(b.String()), 0644)
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
func dropTask(message string) {
	if err := dropTaskTo("/srv/conos/inbox", message); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write task: %v\n", err)
		os.Exit(1)
	}
}

func dropTaskTo(inbox, message string) error {
	taskID := fmt.Sprintf("%d", time.Now().UnixMicro())
	taskPath := filepath.Join(inbox, taskID+".task")
	if err := os.WriteFile(taskPath, []byte(message), 0644); err != nil {
		return err
	}
	fmt.Printf("Task %s.task dropped into inbox\n", taskID)
	return nil
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

func bytesTrimSpace(b []byte) []byte {
	return bytes.TrimSpace(b)
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

func dropTaskToAgent(agentsDir, agentName, message string) error {
	inbox := filepath.Join(agentsDir, agentName, "inbox")
	if _, err := os.Stat(inbox); err != nil {
		return fmt.Errorf("agent %q not found (no inbox at %s)", agentName, inbox)
	}
	taskID := fmt.Sprintf("%d", time.Now().UnixMicro())
	taskPath := filepath.Join(inbox, taskID+".task")
	return os.WriteFile(taskPath, []byte(message), 0644)
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
