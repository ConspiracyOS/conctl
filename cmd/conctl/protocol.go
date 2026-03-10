package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	sharedcontracts "github.com/ConspiracyOS/contracts/pkg/contracts"
)

func runProtocol(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conctl protocol <check|list> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "check":
		runProtocolCheck(args[1:])
	case "list":
		runProtocolList()
	default:
		fmt.Fprintf(os.Stderr, "unknown protocol subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func runProtocolCheck(args []string) {
	// Reorder args so contract ID (non-flag) can appear before flags.
	// flag.Parse stops at first non-flag, so move it to the end.
	var reordered []string
	var positional []string
	for i := 0; i < len(args); i++ {
		if strings.HasPrefix(args[i], "-") {
			reordered = append(reordered, args[i])
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				reordered = append(reordered, args[i+1])
				i++
			}
		} else {
			positional = append(positional, args[i])
		}
	}
	reordered = append(reordered, positional...)

	fs := flag.NewFlagSet("protocol check", flag.ExitOnError)
	exempt := fs.String("exempt", "", "check name to exempt")
	reason := fs.String("reason", "", "reason for exemption (required with --exempt)")
	fs.Parse(reordered)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl protocol check <contract-id> [--exempt <check> --reason <reason>]")
		os.Exit(1)
	}
	contractID := fs.Arg(0)

	if *exempt != "" && *reason == "" {
		fmt.Fprintln(os.Stderr, "error: --reason is required when using --exempt")
		os.Exit(1)
	}

	contractsDir := "/srv/conos/contracts"
	if env := os.Getenv("CONOS_CONTRACTS_DIR"); env != "" {
		contractsDir = env
	}

	allContracts, err := sharedcontracts.LoadDir(contractsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protocol: loading contracts: %v\n", err)
		os.Exit(1)
	}

	var target *sharedcontracts.Contract
	for _, c := range allContracts {
		if c.ID == contractID {
			target = c
			break
		}
	}
	if target == nil {
		fmt.Fprintf(os.Stderr, "protocol: contract %s not found\n", contractID)
		os.Exit(1)
	}

	if target.Type != "protocol" {
		fmt.Fprintf(os.Stderr, "warning: %s is type %q, not \"protocol\"\n", contractID, target.Type)
	}

	// Run checks for this single contract
	result := sharedcontracts.RunAudit([]*sharedcontracts.Contract{target}, nil, contractsDir)

	// Apply exemption if specified
	if *exempt != "" {
		for i, cr := range result.Results {
			if cr.CheckName == *exempt && (cr.Status == "halt" || cr.Status == "fail") {
				result.Results[i].Status = "exempt"
				result.Results[i].Message = fmt.Sprintf("exempt: %s", *reason)
				logProtocolExemption(contractID, *exempt, *reason)
			}
		}
		// Recount after exemption
		result = recount(result)
	}

	// Display results
	hasHalt := false
	for _, cr := range result.Results {
		status := strings.ToUpper(string(cr.Status))
		fmt.Printf("  %-6s  %s", status, cr.CheckName)
		if cr.Message != "" {
			fmt.Printf("  — %s", cr.Message)
		}
		if cr.What != "" && cr.Status != "pass" {
			fmt.Printf("  [%s]", cr.What)
		}
		fmt.Println()
		if cr.Status == "halt" {
			hasHalt = true
		}
	}

	fmt.Println()
	if hasHalt {
		fmt.Printf("BLOCKED: %s has %d halted check(s). Action must not proceed.\n", contractID, result.Halted)
		os.Exit(1)
	}
	fmt.Printf("OK: %s — all preconditions met.\n", contractID)
}

func runProtocolList() {
	contractsDir := "/srv/conos/contracts"
	if env := os.Getenv("CONOS_CONTRACTS_DIR"); env != "" {
		contractsDir = env
	}

	allContracts, err := sharedcontracts.LoadDir(contractsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "protocol: loading contracts: %v\n", err)
		os.Exit(1)
	}

	found := 0
	for _, c := range allContracts {
		if c.Type == "protocol" {
			found++
			trigger := c.Trigger
			if trigger == "" {
				trigger = "(no trigger)"
			}
			fmt.Printf("  %s  %s  trigger: %s\n", c.ID, c.Description, trigger)
		}
	}
	if found == 0 {
		fmt.Println("No protocol contracts found.")
	}
}

func logProtocolExemption(contractID, checkName, reason string) {
	logPath := "/srv/conos/logs/audit/protocol.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s [protocol:exempt] %s/%s reason=%q\n",
		time.Now().UTC().Format(time.RFC3339), contractID, checkName, reason)
}

func recount(result sharedcontracts.AuditResult) sharedcontracts.AuditResult {
	result.Passed = 0
	result.Failed = 0
	result.Warned = 0
	result.Exempt = 0
	result.Skipped = 0
	result.Halted = 0
	for _, cr := range result.Results {
		switch cr.Status {
		case "pass":
			result.Passed++
		case "fail":
			result.Failed++
		case "warn":
			result.Warned++
		case "exempt":
			result.Exempt++
		case "skip":
			result.Skipped++
		case "halt":
			result.Halted++
		}
	}
	return result
}
