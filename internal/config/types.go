package config

// Config is the top-level ConspiracyOS configuration.
type Config struct {
	System     SystemConfig     `toml:"system"`
	Deployment DeploymentConfig `toml:"deployment"`
	Infra      InfraConfig      `toml:"infra"`
	Base       BaseConfig       `toml:"base"`
	Network    NetworkConfig    `toml:"network"`
	Contracts  ContractsConfig  `toml:"contracts"`
	Dashboard  DashboardConfig  `toml:"dashboard"`
	Agents     []AgentConfig    `toml:"agents"`
}

type SystemConfig struct {
	Name string `toml:"name"`
}

type DeploymentConfig struct {
	Mode string `toml:"mode"` // local_trusted | authenticated
}

type InfraConfig struct {
	TailscaleHostname    string   `toml:"tailscale_hostname"`
	TailscaleLoginServer string   `toml:"tailscale_login_server"`
	SSHAuthorizedKeys    []string `toml:"ssh_authorized_keys"`
	SSHPort              int      `toml:"ssh_port"`
}

// BaseConfig holds defaults that apply to all agents.
// Tier sub-sections override base values for agents of that tier.
// Resolution: agent > base.<tier> > base.
type BaseConfig struct {
	Runner    string `toml:"runner"`
	Provider  string `toml:"provider"`
	Model     string `toml:"model"`
	APIKeyEnv string `toml:"api_key_env"`

	Officer TierConfig `toml:"officer"`
	Worker  TierConfig `toml:"worker"`

	// Deprecated: use top-level base fields. Kept for backwards compatibility.
	Operator TierConfig `toml:"operator"`
}

// TierConfig overrides base values for a specific tier.
// Empty fields fall through to BaseConfig.
type TierConfig struct {
	Runner    string `toml:"runner"`
	Provider  string `toml:"provider"`
	Model     string `toml:"model"`
	APIKeyEnv string `toml:"api_key_env"`
}

type NetworkConfig struct {
	OutboundFilter string `toml:"outbound_filter"`
}

type ContractsConfig struct {
	System                 SystemContracts `toml:"system"`
	BriefOutput            string          `toml:"brief_output"`               // path to write system-state.md after healthcheck (empty = disabled)
	DailyBudgetUSD         float64         `toml:"daily_budget_usd"`           // 0 disables budget enforcement
	EstimatedCostPerRunUSD float64         `toml:"estimated_cost_per_run_usd"` // simple fallback cost model when provider metrics are unavailable
}

type SystemContracts struct {
	DiskMinFreePct      int     `toml:"disk_min_free_pct"`
	MemMinFreePct       int     `toml:"mem_min_free_pct"`
	MaxLoadFactor       float64 `toml:"max_load_factor"`
	MaxSessionMin       int     `toml:"max_session_min"`
	HealthcheckInterval string  `toml:"healthcheck_interval"`
}

type DashboardConfig struct {
	Enabled bool   `toml:"enabled"`
	Port    int    `toml:"port"`
	Bind    string `toml:"bind"`
}

type AgentConfig struct {
	Name         string   `toml:"name"`
	Tier         string   `toml:"tier"`
	Roles        []string `toml:"roles"`
	Groups       []string `toml:"groups"`
	Scopes       []string `toml:"scopes"`
	Mode         string   `toml:"mode"`
	Cron         string   `toml:"cron"`
	Runner       string   `toml:"runner"`
	RunnerArgs   []string `toml:"runner_args"`
	Environment  []string `toml:"environment"`
	Provider     string   `toml:"provider"`
	Model        string   `toml:"model"`
	APIKeyEnv    string   `toml:"api_key_env"`
	MaxSessions  int      `toml:"max_sessions"`
	Instructions string   `toml:"instructions"`
}

// ResolvedAgent returns an AgentConfig with base and tier defaults applied.
// Resolution: agent > base.<tier> > base.
func (c *Config) ResolvedAgent(name string) AgentConfig {
	for _, a := range c.Agents {
		if a.Name == name {
			resolved := a

			// Get tier config
			tier := c.tierConfig(resolved.Tier)

			// Apply tier defaults (tier > base)
			if resolved.Runner == "" {
				resolved.Runner = firstNonEmpty(tier.Runner, c.Base.Runner)
			}
			if resolved.Provider == "" {
				resolved.Provider = firstNonEmpty(tier.Provider, c.Base.Provider)
			}
			if resolved.Model == "" {
				resolved.Model = firstNonEmpty(tier.Model, c.Base.Model)
			}
			if resolved.APIKeyEnv == "" {
				resolved.APIKeyEnv = firstNonEmpty(tier.APIKeyEnv, c.Base.APIKeyEnv)
			}

			// Claude Code OAuth tokens only work with the Claude Code CLI.
			// Force the runner so PicoClaw doesn't try to use an OAuth token as an API key.
			if resolved.APIKeyEnv == "CLAUDE_CODE_OAUTH_TOKEN" && (resolved.Runner == "" || resolved.Runner == "picoclaw") {
				resolved.Runner = "claude-code"
				if len(resolved.RunnerArgs) == 0 {
					resolved.RunnerArgs = []string{"--print"}
				}
			}

			// Global defaults (fallbacks when nothing is configured)
			if resolved.Runner == "" {
				resolved.Runner = "picoclaw"
			}
			if resolved.Provider == "" {
				resolved.Provider = "openrouter"
			}
			if resolved.APIKeyEnv == "" {
				resolved.APIKeyEnv = "CONOS_API_KEY"
			}
			if resolved.MaxSessions == 0 {
				resolved.MaxSessions = 1
			}
			if resolved.Mode == "" {
				resolved.Mode = "on-demand"
			}
			if resolved.Tier == "" {
				resolved.Tier = "worker"
			}

			return resolved
		}
	}
	return AgentConfig{}
}

// tierConfig returns the TierConfig for a given tier name.
func (c *Config) tierConfig(tier string) TierConfig {
	switch tier {
	case "officer":
		return c.Base.Officer
	case "operator":
		return c.Base.Operator
	case "worker":
		return c.Base.Worker
	default:
		return TierConfig{}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
