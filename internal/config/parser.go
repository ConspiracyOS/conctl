package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// legacyConfig handles the old [defaults.<tier>] format for backwards compatibility.
type legacyConfig struct {
	Defaults map[string]TierConfig `toml:"defaults"`
}

func Parse(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Backwards compat: migrate [defaults.<tier>] to [base.<tier>]
	migrateLegacyDefaults(path, &cfg)

	applyENVOverrides(&cfg)
	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// migrateLegacyDefaults reads [defaults.*] from the file and maps it to Base.
func migrateLegacyDefaults(path string, cfg *Config) {
	var legacy legacyConfig
	if _, err := toml.DecodeFile(path, &legacy); err != nil || legacy.Defaults == nil {
		return
	}

	for tier, td := range legacy.Defaults {
		tc := TierConfig{
			Runner:    td.Runner,
			Provider:  td.Provider,
			Model:     td.Model,
			APIKeyEnv: td.APIKeyEnv,
		}
		switch tier {
		case "officer":
			if cfg.Base.Officer == (TierConfig{}) {
				cfg.Base.Officer = tc
			}
		case "operator":
			if cfg.Base.Operator == (TierConfig{}) {
				cfg.Base.Operator = tc
			}
		case "worker":
			if cfg.Base.Worker == (TierConfig{}) {
				cfg.Base.Worker = tc
			}
		}
	}
}

func applyENVOverrides(cfg *Config) {
	if v := os.Getenv("CONOS_SYSTEM_NAME"); v != "" {
		cfg.System.Name = v
	}
	if v := os.Getenv("CONOS_INFRA_TAILSCALE_HOSTNAME"); v != "" {
		cfg.Infra.TailscaleHostname = v
	}
	if v := os.Getenv("CONOS_INFRA_TAILSCALE_LOGIN_SERVER"); v != "" {
		cfg.Infra.TailscaleLoginServer = v
	}
	if v := os.Getenv("CONOS_SSH_AUTHORIZED_KEYS"); v != "" {
		cfg.Infra.SSHAuthorizedKeys = strings.Split(v, "\n")
	}
}

func applyDefaults(cfg *Config) {
	if cfg.System.Name == "" {
		cfg.System.Name = "conspiracy"
	}
	if cfg.Network.OutboundFilter == "" {
		cfg.Network.OutboundFilter = "strict"
	}
	if cfg.Infra.SSHPort == 0 {
		cfg.Infra.SSHPort = 22
	}
	if cfg.Contracts.System.DiskMinFreePct == 0 {
		cfg.Contracts.System.DiskMinFreePct = 15
	}
	if cfg.Contracts.System.MemMinFreePct == 0 {
		cfg.Contracts.System.MemMinFreePct = 10
	}
	if cfg.Contracts.System.MaxLoadFactor == 0 {
		cfg.Contracts.System.MaxLoadFactor = 2.0
	}
	if cfg.Contracts.System.MaxSessionMin == 0 {
		cfg.Contracts.System.MaxSessionMin = 30
	}
	if cfg.Contracts.System.HealthcheckInterval == "" {
		cfg.Contracts.System.HealthcheckInterval = "60s"
	}
	if cfg.Dashboard.Port == 0 {
		cfg.Dashboard.Port = 8080
	}
	if cfg.Dashboard.Bind == "" {
		cfg.Dashboard.Bind = "0.0.0.0"
	}
	for i := range cfg.Agents {
		if cfg.Agents[i].Tier == "" {
			cfg.Agents[i].Tier = "worker"
		}
	}
}

func validate(cfg *Config) error {
	validTiers := map[string]bool{"officer": true, "operator": true, "worker": true}
	validModes := map[string]bool{"on-demand": true, "continuous": true, "cron": true}

	for i, a := range cfg.Agents {
		if a.Name == "" {
			return fmt.Errorf("agent[%d]: name is required", i)
		}
		if a.Tier != "" && !validTiers[a.Tier] {
			return fmt.Errorf("agent %q: invalid tier %q (must be officer/operator/worker)", a.Name, a.Tier)
		}
		if a.Mode != "" && !validModes[a.Mode] {
			return fmt.Errorf("agent %q: invalid mode %q (must be on-demand/continuous/cron)", a.Name, a.Mode)
		}
		if a.Mode == "cron" && a.Cron == "" {
			return fmt.Errorf("agent %q: cron mode requires a cron expression", a.Name)
		}
	}

	// Validate picoclaw provider compatibility at the resolved level.
	// Exec runners handle their own auth — no provider constraint applies.
	for _, a := range cfg.Agents {
		resolved := cfg.ResolvedAgent(a.Name)
		if err := validatePicoClawProvider(a.Name, resolved.Runner, resolved.Provider); err != nil {
			return err
		}
	}

	return nil
}

// validatePicoClawProvider checks that a picoclaw agent's provider is supported.
// Exec runners (any runner other than "picoclaw") handle their own auth — skip.
func validatePicoClawProvider(agentName, runner, provider string) error {
	if runner != "picoclaw" && runner != "" {
		return nil
	}
	validProviders := map[string]bool{
		"openrouter": true, "anthropic": true, "openai": true, "": true,
	}
	if !validProviders[provider] {
		return fmt.Errorf("agent %q: picoclaw does not support provider %q (use openrouter/anthropic/openai)", agentName, provider)
	}
	return nil
}
