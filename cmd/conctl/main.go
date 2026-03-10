package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ConspiracyOS/conctl/internal/config"
)

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
		fmt.Fprintln(os.Stderr, "  brief                             System state brief for agents")
		fmt.Fprintln(os.Stderr, "  package install|remove|list        Manage agent packages")
		fmt.Fprintln(os.Stderr, "  protocol check|list               Evaluate protocol contracts")
		fmt.Fprintln(os.Stderr, "  kill <agent>                      Stop a running agent's systemd units")
		fmt.Fprintln(os.Stderr, "  clear-sessions [agent]            Clear agent session files")
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
	case "brief":
		runBrief()
	case "package":
		runPackage(os.Args[2:])
	case "protocol":
		runProtocol(os.Args[2:])
	case "kill":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: conctl kill <agent-name>")
			os.Exit(1)
		}
		if err := killAgentUnits(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	case "clear-sessions":
		agent := ""
		if len(os.Args) > 2 {
			agent = os.Args[2]
		}
		if err := clearSessions(agent); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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
