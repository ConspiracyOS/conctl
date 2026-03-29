package config

import (
	"testing"
)

func TestResolvedAgent_TierOverridesBase(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:    "base-runner",
			Provider:  "base-provider",
			Model:     "base-model",
			APIKeyEnv: "BASE_KEY",
			APIBase:   "https://base.example.com",
			Officer: TierConfig{
				Runner:    "officer-runner",
				Provider:  "officer-provider",
				Model:     "officer-model",
				APIKeyEnv: "OFFICER_KEY",
				APIBase:   "https://officer.example.com",
			},
		},
		Agents: []AgentConfig{
			{Name: "lead", Tier: "officer"},
		},
	}

	got := cfg.ResolvedAgent("lead")

	if got.Runner != "officer-runner" {
		t.Errorf("Runner = %q, want %q", got.Runner, "officer-runner")
	}
	if got.Provider != "officer-provider" {
		t.Errorf("Provider = %q, want %q", got.Provider, "officer-provider")
	}
	if got.Model != "officer-model" {
		t.Errorf("Model = %q, want %q", got.Model, "officer-model")
	}
	if got.APIKeyEnv != "OFFICER_KEY" {
		t.Errorf("APIKeyEnv = %q, want %q", got.APIKeyEnv, "OFFICER_KEY")
	}
	if got.APIBase != "https://officer.example.com" {
		t.Errorf("APIBase = %q, want %q", got.APIBase, "https://officer.example.com")
	}
}

func TestResolvedAgent_AgentOverridesTierAndBase(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:   "base-runner",
			Provider: "base-provider",
			Model:    "base-model",
			Worker: TierConfig{
				Runner:   "worker-runner",
				Provider: "worker-provider",
				Model:    "worker-model",
			},
		},
		Agents: []AgentConfig{
			{
				Name:     "custom",
				Tier:     "worker",
				Runner:   "agent-runner",
				Provider: "agent-provider",
				Model:    "agent-model",
			},
		},
	}

	got := cfg.ResolvedAgent("custom")

	if got.Runner != "agent-runner" {
		t.Errorf("Runner = %q, want %q", got.Runner, "agent-runner")
	}
	if got.Provider != "agent-provider" {
		t.Errorf("Provider = %q, want %q", got.Provider, "agent-provider")
	}
	if got.Model != "agent-model" {
		t.Errorf("Model = %q, want %q", got.Model, "agent-model")
	}
}

func TestResolvedAgent_OAuthDoesNotOverrideNonPicoclawRunner(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{
				Name:      "explicit-runner",
				APIKeyEnv: "CLAUDE_CODE_OAUTH_TOKEN",
				Runner:    "custom-runner",
			},
		},
	}

	got := cfg.ResolvedAgent("explicit-runner")

	if got.Runner != "custom-runner" {
		t.Errorf("Runner = %q, want %q (explicit non-picoclaw runner should not be overridden)", got.Runner, "custom-runner")
	}
}

func TestResolvedAgent_OAuthOverridesPicoclaw(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{
				Name:      "pico-oauth",
				APIKeyEnv: "CLAUDE_CODE_OAUTH_TOKEN",
				Runner:    "picoclaw",
			},
		},
	}

	got := cfg.ResolvedAgent("pico-oauth")

	if got.Runner != "claude-code" {
		t.Errorf("Runner = %q, want %q (picoclaw should be overridden by OAuth)", got.Runner, "claude-code")
	}
}

func TestResolvedAgent_OAuthByTokenPrefix_InheritedFromBase(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			APIKeyEnv: "TEST_OAUTH_KEY",
		},
		Agents: []AgentConfig{
			{Name: "token-agent"},
		},
	}

	t.Setenv("TEST_OAUTH_KEY", "sk-ant-oat-something-secret")

	got := cfg.ResolvedAgent("token-agent")

	if got.Runner != "claude-code" {
		t.Errorf("Runner = %q, want %q for OAuth token prefix inherited from base", got.Runner, "claude-code")
	}
	if len(got.RunnerArgs) != 1 || got.RunnerArgs[0] != "--print" {
		t.Errorf("RunnerArgs = %v, want [--print] for OAuth", got.RunnerArgs)
	}
}

func TestResolvedAgent_ClaudeDefaultsToStatelessThreadedContext(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{
				Name:   "claude-agent",
				Runner: "claude-code",
			},
		},
	}

	got := cfg.ResolvedAgent("claude-agent")

	if got.SessionStrategy != "stateless" {
		t.Errorf("SessionStrategy = %q, want %q", got.SessionStrategy, "stateless")
	}
	if got.RecentTurns != 8 {
		t.Errorf("RecentTurns = %d, want %d", got.RecentTurns, 8)
	}
	if got.BriefMaxBytes != 16*1024 {
		t.Errorf("BriefMaxBytes = %d, want %d", got.BriefMaxBytes, 16*1024)
	}
}

func TestResolvedAgent_MultipleAgents_ResolvesCorrectOne(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner: "default-runner",
		},
		Agents: []AgentConfig{
			{Name: "alpha", Tier: "officer", Model: "alpha-model"},
			{Name: "beta", Tier: "worker", Model: "beta-model"},
			{Name: "gamma", Tier: "operator"},
		},
	}

	tests := []struct {
		name      string
		wantModel string
		wantTier  string
	}{
		{"alpha", "alpha-model", "officer"},
		{"beta", "beta-model", "worker"},
		{"gamma", "", "operator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cfg.ResolvedAgent(tt.name)
			if got.Name != tt.name {
				t.Errorf("Name = %q, want %q", got.Name, tt.name)
			}
			if got.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", got.Model, tt.wantModel)
			}
			if got.Tier != tt.wantTier {
				t.Errorf("Tier = %q, want %q", got.Tier, tt.wantTier)
			}
			if got.Runner != "default-runner" {
				t.Errorf("Runner = %q, want %q", got.Runner, "default-runner")
			}
		})
	}
}

func TestResolvedAgent_PartialTierFallsThrough(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:    "base-runner",
			Provider:  "base-provider",
			Model:     "base-model",
			APIKeyEnv: "BASE_KEY",
			APIBase:   "https://base.example.com",
			Worker: TierConfig{
				Model: "worker-model",
				// Other fields empty -- should fall through to base.
			},
		},
		Agents: []AgentConfig{
			{Name: "partial", Tier: "worker"},
		},
	}

	got := cfg.ResolvedAgent("partial")

	if got.Runner != "base-runner" {
		t.Errorf("Runner = %q, want %q (should fall through to base)", got.Runner, "base-runner")
	}
	if got.Provider != "base-provider" {
		t.Errorf("Provider = %q, want %q (should fall through to base)", got.Provider, "base-provider")
	}
	if got.Model != "worker-model" {
		t.Errorf("Model = %q, want %q (should come from tier)", got.Model, "worker-model")
	}
	if got.APIKeyEnv != "BASE_KEY" {
		t.Errorf("APIKeyEnv = %q, want %q (should fall through to base)", got.APIKeyEnv, "BASE_KEY")
	}
	if got.APIBase != "https://base.example.com" {
		t.Errorf("APIBase = %q, want %q (should fall through to base)", got.APIBase, "https://base.example.com")
	}
}

func TestResolvedAgent_PreservesNonInheritedFields(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{
				Name:         "full",
				Tier:         "officer",
				Roles:        []string{"admin"},
				Groups:       []string{"ops"},
				Scopes:       []string{"/srv"},
				Mode:         "always-on",
				Cron:         "*/5 * * * *",
				MaxSessions:  3,
				Instructions: "Do the thing.",
				Packages:     []string{"curl", "jq"},
				AllowedPorts: []int{443, 8080},
				Environment:  []string{"FOO=bar"},
			},
		},
	}

	got := cfg.ResolvedAgent("full")

	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", got.Roles)
	}
	if len(got.Groups) != 1 || got.Groups[0] != "ops" {
		t.Errorf("Groups = %v, want [ops]", got.Groups)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "/srv" {
		t.Errorf("Scopes = %v, want [/srv]", got.Scopes)
	}
	if got.Mode != "always-on" {
		t.Errorf("Mode = %q, want %q", got.Mode, "always-on")
	}
	if got.Cron != "*/5 * * * *" {
		t.Errorf("Cron = %q, want %q", got.Cron, "*/5 * * * *")
	}
	if got.MaxSessions != 3 {
		t.Errorf("MaxSessions = %d, want %d", got.MaxSessions, 3)
	}
	if got.Instructions != "Do the thing." {
		t.Errorf("Instructions = %q, want %q", got.Instructions, "Do the thing.")
	}
	if len(got.Packages) != 2 {
		t.Errorf("Packages = %v, want [curl jq]", got.Packages)
	}
	if len(got.AllowedPorts) != 2 {
		t.Errorf("AllowedPorts = %v, want [443 8080]", got.AllowedPorts)
	}
	if len(got.Environment) != 1 || got.Environment[0] != "FOO=bar" {
		t.Errorf("Environment = %v, want [FOO=bar]", got.Environment)
	}
}

func TestResolvedAgent_RunnerArgsInheritedFromBase(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:     "claude",
			RunnerArgs: []string{"--print", "--allowedTools", "Bash"},
		},
		Agents: []AgentConfig{
			{Name: "concierge"},
		},
	}
	got := cfg.ResolvedAgent("concierge")
	if len(got.RunnerArgs) != 3 || got.RunnerArgs[0] != "--print" {
		t.Errorf("RunnerArgs = %v, want [--print --allowedTools Bash]", got.RunnerArgs)
	}
}

func TestResolvedAgent_RunnerArgsTierOverridesBase(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:     "claude",
			RunnerArgs: []string{"--print"},
			Operator: TierConfig{
				RunnerArgs: []string{"--print", "--verbose"},
			},
		},
		Agents: []AgentConfig{
			{Name: "concierge", Tier: "operator"},
		},
	}
	got := cfg.ResolvedAgent("concierge")
	if len(got.RunnerArgs) != 2 || got.RunnerArgs[1] != "--verbose" {
		t.Errorf("RunnerArgs = %v, want [--print --verbose]", got.RunnerArgs)
	}
}

func TestResolvedAgent_RunnerArgsAgentOverridesAll(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Runner:     "claude",
			RunnerArgs: []string{"--print"},
		},
		Agents: []AgentConfig{
			{Name: "concierge", RunnerArgs: []string{"--custom"}},
		},
	}
	got := cfg.ResolvedAgent("concierge")
	if len(got.RunnerArgs) != 1 || got.RunnerArgs[0] != "--custom" {
		t.Errorf("RunnerArgs = %v, want [--custom]", got.RunnerArgs)
	}
}

func TestTierConfig_AllTiers(t *testing.T) {
	cfg := &Config{
		Base: BaseConfig{
			Officer: TierConfig{
				Runner:    "officer-runner",
				Provider:  "officer-provider",
				Model:     "officer-model",
				APIKeyEnv: "OFFICER_KEY",
				APIBase:   "https://officer.example.com",
			},
			Operator: TierConfig{
				Runner:    "operator-runner",
				Provider:  "operator-provider",
				Model:     "operator-model",
				APIKeyEnv: "OPERATOR_KEY",
				APIBase:   "https://operator.example.com",
			},
			Worker: TierConfig{
				Runner:    "worker-runner",
				Provider:  "worker-provider",
				Model:     "worker-model",
				APIKeyEnv: "WORKER_KEY",
				APIBase:   "https://worker.example.com",
			},
		},
	}

	tests := []struct {
		tier         string
		wantRunner   string
		wantProvider string
		wantModel    string
		wantKeyEnv   string
		wantAPIBase  string
	}{
		{"officer", "officer-runner", "officer-provider", "officer-model", "OFFICER_KEY", "https://officer.example.com"},
		{"operator", "operator-runner", "operator-provider", "operator-model", "OPERATOR_KEY", "https://operator.example.com"},
		{"worker", "worker-runner", "worker-provider", "worker-model", "WORKER_KEY", "https://worker.example.com"},
		{"unknown", "", "", "", "", ""},
		{"", "", "", "", "", ""},
		{"OFFICER", "", "", "", "", ""},
	}

	for _, tt := range tests {
		name := tt.tier
		if name == "" {
			name = "empty"
		}
		t.Run("tier_"+name, func(t *testing.T) {
			got := cfg.tierConfig(tt.tier)
			if got.Runner != tt.wantRunner {
				t.Errorf("tierConfig(%q).Runner = %q, want %q", tt.tier, got.Runner, tt.wantRunner)
			}
			if got.Provider != tt.wantProvider {
				t.Errorf("tierConfig(%q).Provider = %q, want %q", tt.tier, got.Provider, tt.wantProvider)
			}
			if got.Model != tt.wantModel {
				t.Errorf("tierConfig(%q).Model = %q, want %q", tt.tier, got.Model, tt.wantModel)
			}
			if got.APIKeyEnv != tt.wantKeyEnv {
				t.Errorf("tierConfig(%q).APIKeyEnv = %q, want %q", tt.tier, got.APIKeyEnv, tt.wantKeyEnv)
			}
			if got.APIBase != tt.wantAPIBase {
				t.Errorf("tierConfig(%q).APIBase = %q, want %q", tt.tier, got.APIBase, tt.wantAPIBase)
			}
		})
	}
}
