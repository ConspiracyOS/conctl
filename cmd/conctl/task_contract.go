package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ConspiracyOS/conctl/internal/contracts"
)

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
