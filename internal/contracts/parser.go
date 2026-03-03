package contracts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadDir reads all .yaml files from dir and returns parsed contracts.
func LoadDir(dir string) ([]Contract, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading contracts dir: %w", err)
	}

	var contracts []Contract
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		c, err := LoadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", e.Name(), err)
		}
		contracts = append(contracts, c)
	}
	return contracts, nil
}

// LoadFile reads and parses a single contract YAML file.
func LoadFile(path string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, fmt.Errorf("reading %s: %w", path, err)
	}

	var c Contract
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Contract{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := validate(c); err != nil {
		return Contract{}, fmt.Errorf("validating %s: %w", filepath.Base(path), err)
	}

	return c, nil
}

// validate checks a contract for structural correctness.
func validate(c Contract) error {
	if c.ID == "" {
		return fmt.Errorf("contract missing id")
	}
	if c.Type != "detective" && c.Type != "preventive" {
		return fmt.Errorf("contract %s: type must be 'detective' or 'preventive', got %q", c.ID, c.Type)
	}

	// Preventive contracts are registry-only, no checks needed
	if c.Type == "preventive" {
		return nil
	}

	// Detective contracts must have at least one check
	if len(c.Checks) == 0 {
		return fmt.Errorf("contract %s: detective contract must have at least one check", c.ID)
	}

	for i, ch := range c.Checks {
		if ch.Command == nil && ch.Script == nil {
			return fmt.Errorf("contract %s check %d (%s): must have command or script", c.ID, i, ch.Name)
		}
		if ch.Command != nil && ch.Script != nil {
			return fmt.Errorf("contract %s check %d (%s): cannot have both command and script", c.ID, i, ch.Name)
		}
		if ch.OnFail.Action != "" && !validActions[ch.OnFail.Action] {
			return fmt.Errorf("contract %s check %d (%s): invalid action %q", c.ID, i, ch.Name, ch.OnFail.Action)
		}
	}

	return nil
}

// HasSchedule reports whether the contract has a scheduling directive.
// Accepts both new (trigger: schedule) and old (frequency: ...) formats.
func (c *Contract) HasSchedule() bool {
	return c.Trigger == "schedule" || c.Frequency != ""
}

// IsGlobal reports whether the contract has system-wide scope.
// Accepts both new ("global") and old ("system") scope values.
func (c *Contract) IsGlobal() bool {
	return c.Scope == "global" || c.Scope == "system"
}
