package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var validPackageName = regexp.MustCompile(`^[a-z0-9][a-z0-9.+\-]+$`)

func runPackage(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: conctl package <install|remove|list> [args]")
		os.Exit(1)
	}

	switch args[0] {
	case "install":
		runPackageInstall(args[1:])
	case "remove":
		runPackageRemove(args[1:])
	case "list":
		runPackageList(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown package subcommand: %s\n", args[0])
		os.Exit(1)
	}
}

func runPackageInstall(args []string) {
	fs := flag.NewFlagSet("package install", flag.ExitOnError)
	agent := fs.String("agent", "", "agent the package is for (informational)")
	save := fs.Bool("save", false, "persist to conos.toml")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl package install <package> --agent <name> [--save]")
		os.Exit(1)
	}
	pkg := fs.Arg(0)

	if !validPackageName.MatchString(pkg) {
		fmt.Fprintf(os.Stderr, "error: invalid package name %q\n", pkg)
		os.Exit(1)
	}

	// Install via apt-get
	cmd := exec.Command("apt-get", "install", "-y", "--no-install-recommends", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: apt-get install failed: %v\n", err)
		os.Exit(1)
	}

	// Log to audit trail
	logPackageAction("install", pkg, *agent)

	if *save {
		if *agent == "" {
			fmt.Fprintln(os.Stderr, "error: --agent is required with --save")
			os.Exit(1)
		}
		if err := savePackageToConfig(pkg, *agent); err != nil {
			fmt.Fprintf(os.Stderr, "warning: installed but failed to save to config: %v\n", err)
		} else {
			fmt.Printf("Saved %s to conos.toml for agent %s\n", pkg, *agent)
		}
	}

	fmt.Printf("Installed %s\n", pkg)
}

func runPackageRemove(args []string) {
	fs := flag.NewFlagSet("package remove", flag.ExitOnError)
	agent := fs.String("agent", "", "agent the package was for")
	save := fs.Bool("save", false, "remove from conos.toml")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: conctl package remove <package> [--agent <name>] [--save]")
		os.Exit(1)
	}
	pkg := fs.Arg(0)

	if !validPackageName.MatchString(pkg) {
		fmt.Fprintf(os.Stderr, "error: invalid package name %q\n", pkg)
		os.Exit(1)
	}

	cmd := exec.Command("apt-get", "remove", "-y", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: apt-get remove failed: %v\n", err)
		os.Exit(1)
	}

	logPackageAction("remove", pkg, *agent)

	if *save && *agent != "" {
		if err := removePackageFromConfig(pkg, *agent); err != nil {
			fmt.Fprintf(os.Stderr, "warning: removed but failed to update config: %v\n", err)
		}
	}

	fmt.Printf("Removed %s\n", pkg)
}

func runPackageList(args []string) {
	fs := flag.NewFlagSet("package list", flag.ExitOnError)
	agent := fs.String("agent", "", "show packages for a specific agent")
	fs.Parse(args)

	cfg := loadConfig()

	for _, a := range cfg.Agents {
		if *agent != "" && a.Name != *agent {
			continue
		}
		if len(a.Packages) > 0 {
			fmt.Printf("%s: %s\n", a.Name, strings.Join(a.Packages, ", "))
		}
	}
}

func logPackageAction(action, pkg, agent string) {
	logPath := "/srv/conos/logs/audit/package.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	agentStr := ""
	if agent != "" {
		agentStr = fmt.Sprintf(" agent=%s", agent)
	}
	fmt.Fprintf(f, "%s [package:%s] %s%s\n",
		time.Now().UTC().Format(time.RFC3339), action, pkg, agentStr)
}

func savePackageToConfig(pkg, agent string) error {
	// Read the config file, find the agent's packages line, append.
	// This is a minimal TOML editor — finds the [[agents]] block by name
	// and adds/updates the packages field.
	configPath := "/etc/conos/conos.toml"
	if env := os.Getenv("CONOS_CONFIG"); env != "" {
		configPath = env
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	inAgent := false
	agentFound := false
	packagesLine := -1
	insertAfter := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[agents]]" {
			inAgent = false
			agentFound = false
		}
		if inAgent && agentFound {
			if strings.HasPrefix(trimmed, "packages") {
				packagesLine = i
				break
			}
			if strings.HasPrefix(trimmed, "[[") || trimmed == "" {
				// End of agent block without packages — insert here
				insertAfter = i - 1
				break
			}
			insertAfter = i
		}
		if strings.HasPrefix(trimmed, "name") && strings.Contains(trimmed, fmt.Sprintf("%q", agent)) {
			inAgent = true
			agentFound = true
			insertAfter = i
		}
	}

	if !agentFound {
		return fmt.Errorf("agent %q not found in config", agent)
	}

	if packagesLine >= 0 {
		// Update existing packages line
		existing := lines[packagesLine]
		// Parse existing packages
		if strings.Contains(existing, pkg) {
			return nil // already present
		}
		// Add to the array
		idx := strings.LastIndex(existing, "]")
		if idx > 0 {
			lines[packagesLine] = existing[:idx] + fmt.Sprintf(", %q]", pkg)
		}
	} else if insertAfter >= 0 {
		// Insert new packages line
		newLine := fmt.Sprintf("packages = [%q]", pkg)
		lines = append(lines[:insertAfter+1], append([]string{newLine}, lines[insertAfter+1:]...)...)
	}

	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644)
}

func removePackageFromConfig(pkg, agent string) error {
	configPath := "/etc/conos/conos.toml"
	if env := os.Getenv("CONOS_CONFIG"); env != "" {
		configPath = env
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	inAgent := false
	agentFound := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[[agents]]" {
			inAgent = false
			agentFound = false
		}
		if inAgent && agentFound && strings.HasPrefix(trimmed, "packages") {
			// Remove the package from the array
			newLine := strings.Replace(line, fmt.Sprintf(", %q", pkg), "", 1)
			newLine = strings.Replace(newLine, fmt.Sprintf("%q, ", pkg), "", 1)
			newLine = strings.Replace(newLine, fmt.Sprintf("%q", pkg), "", 1)
			lines[i] = newLine
			break
		}
		if strings.HasPrefix(trimmed, "name") && strings.Contains(trimmed, fmt.Sprintf("%q", agent)) {
			inAgent = true
			agentFound = true
		}
	}

	return os.WriteFile(configPath, []byte(strings.Join(lines, "\n")), 0644)
}
