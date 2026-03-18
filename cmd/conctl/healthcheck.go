package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"

	"github.com/ConspiracyOS/conctl/internal/bootstrap"
	"github.com/ConspiracyOS/conctl/internal/config"
	"github.com/ConspiracyOS/conctl/internal/contracts"

	"gopkg.in/yaml.v3"
)

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
	m := bootstrap.FromConfig(cfg, bootstrap.BootstrapOptions{})
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

func runBrief() {
	contractsDir := "/srv/conos/contracts"
	if env := os.Getenv("CONOS_CONTRACTS_DIR"); env != "" {
		contractsDir = env
	}

	allContracts, err := sharedcontracts.LoadDir(contractsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "brief: loading contracts: %v\n", err)
		os.Exit(1)
	}

	result := sharedcontracts.RunAudit(allContracts, selectedContractTags(), contractsDir)
	opts := contracts.EvalOptions{
		RunID: fmt.Sprintf("brief-%d", time.Now().Unix()),
		Actor: "conctl:brief",
	}
	briefResult := contracts.FromSharedAudit(result, opts)
	fmt.Print(contracts.FormatDoctorReport(briefResult))
}

func runManifest(args []string) {
	if len(args) == 0 || args[0] != "show" {
		fmt.Fprintln(os.Stderr, "usage: conctl manifest show")
		os.Exit(1)
	}
	cfg := loadConfig()
	m := bootstrap.FromConfig(cfg, bootstrap.BootstrapOptions{})
	data, err := yaml.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(string(data))
}
