package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMinimalConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[system]
name = "test-conspiracy"

[[agents]]
name = "concierge"
tier = "operator"
mode = "on-demand"
instructions = "You are the Concierge."

[[agents]]
name = "sysadmin"
tier = "operator"
mode = "on-demand"
instructions = "You are the Sysadmin."
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.System.Name != "test-conspiracy" {
		t.Errorf("expected name 'test-conspiracy', got %q", cfg.System.Name)
	}
	if len(cfg.Agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(cfg.Agents))
	}
	if cfg.Agents[0].Name != "concierge" {
		t.Errorf("expected first agent 'concierge', got %q", cfg.Agents[0].Name)
	}
}

func TestParseBaseConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[system]
name = "base-test"

[base]
runner = "picoclaw"
provider = "openrouter"
api_key_env = "CONOS_API_KEY"

[base.operator]
model = "anthropic/claude-sonnet-4.6"

[base.officer]
model = "anthropic/claude-opus-4-6"
api_key_env = "CON_OFFICER_KEY"

[[agents]]
name = "concierge"
tier = "operator"
mode = "on-demand"

[[agents]]
name = "strategist"
tier = "officer"
mode = "on-demand"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Operator resolves model from base.operator
	op := cfg.ResolvedAgent("concierge")
	if op.Model != "anthropic/claude-sonnet-4.6" {
		t.Errorf("operator model: expected sonnet, got %q", op.Model)
	}
	if op.Provider != "openrouter" {
		t.Errorf("operator provider: expected openrouter, got %q", op.Provider)
	}
	if op.APIKeyEnv != "CONOS_API_KEY" {
		t.Errorf("operator api_key_env: expected CONOS_API_KEY, got %q", op.APIKeyEnv)
	}

	// Officer resolves from base.officer, with tier-specific api_key_env
	off := cfg.ResolvedAgent("strategist")
	if off.Model != "anthropic/claude-opus-4-6" {
		t.Errorf("officer model: expected opus, got %q", off.Model)
	}
	if off.APIKeyEnv != "CON_OFFICER_KEY" {
		t.Errorf("officer api_key_env: expected CON_OFFICER_KEY, got %q", off.APIKeyEnv)
	}
	if off.Provider != "openrouter" {
		t.Errorf("officer provider: expected openrouter (from base), got %q", off.Provider)
	}
}

func TestParseLegacyDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[system]
name = "legacy-test"

[defaults.operator]
model = "anthropic/claude-sonnet-4.6"

[[agents]]
name = "concierge"
tier = "operator"
mode = "on-demand"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("concierge")
	if agent.Model != "anthropic/claude-sonnet-4.6" {
		t.Errorf("expected model from legacy defaults, got %q", agent.Model)
	}
	if agent.Runner != "picoclaw" {
		t.Errorf("expected default runner picoclaw, got %q", agent.Runner)
	}
}

func TestResolvedAgentOverridesBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[base]
provider = "openrouter"
model = "default-model"

[base.worker]
model = "cheap-model"

[[agents]]
name = "special-worker"
tier = "worker"
mode = "on-demand"
model = "expensive-model"
provider = "anthropic"
api_key_env = "SPECIAL_KEY"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("special-worker")
	if agent.Model != "expensive-model" {
		t.Errorf("agent model should override tier, got %q", agent.Model)
	}
	if agent.Provider != "anthropic" {
		t.Errorf("agent provider should override base, got %q", agent.Provider)
	}
	if agent.APIKeyEnv != "SPECIAL_KEY" {
		t.Errorf("agent api_key_env should override, got %q", agent.APIKeyEnv)
	}
}

func TestParseValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")

	// Missing agent name
	os.WriteFile(path, []byte(`
[system]
name = "bad"

[[agents]]
tier = "operator"
mode = "on-demand"
`), 0644)

	_, err := Parse(path)
	if err == nil {
		t.Error("expected validation error for agent without name")
	}
}

func TestValidateRunnerProviderCompat(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		toml    string
		wantErr bool
	}{
		{
			name: "picoclaw+openrouter ok",
			toml: `[[agents]]
name = "a"
tier = "operator"
runner = "picoclaw"
provider = "openrouter"`,
			wantErr: false,
		},
		{
			name: "picoclaw+invalid provider errors",
			toml: `[[agents]]
name = "a"
tier = "operator"
runner = "picoclaw"
provider = "unsupported_provider"`,
			wantErr: true,
		},
		{
			name: "exec runner+any provider ok (BYOR)",
			toml: `[[agents]]
name = "a"
tier = "worker"
runner = "gemini"
provider = "openrouter"`,
			wantErr: false,
		},
		{
			name: "exec runner+no provider ok (BYOR)",
			toml: `[[agents]]
name = "a"
tier = "worker"
runner = "ollama-run-gemma3"`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".toml")
			os.WriteFile(path, []byte(tt.toml), 0644)
			_, err := Parse(path)
			if tt.wantErr && err == nil {
				t.Error("expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestENVOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[system]
name = "from-config"
`), 0644)

	t.Setenv("CONOS_SYSTEM_NAME", "from-env")

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.System.Name != "from-env" {
		t.Errorf("expected env override 'from-env', got %q", cfg.System.Name)
	}
}

func TestParseInvalidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	os.WriteFile(path, []byte("{ not valid toml"), 0644)
	_, err := Parse(path)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestParseDeploymentDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "a"
tier = "worker"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Deployment.Mode != "local_trusted" {
		t.Fatalf("deployment mode = %q, want local_trusted", cfg.Deployment.Mode)
	}
}

func TestParseDeploymentInvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[deployment]
mode = "internet"

[[agents]]
name = "a"
tier = "worker"
`), 0644)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected deployment mode validation error")
	}
}

func TestParseDeploymentLocalTrustedBindGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[deployment]
mode = "local_trusted"

[dashboard]
enabled = true
bind = "0.0.0.0"

[[agents]]
name = "a"
tier = "worker"
`), 0644)

	_, err := Parse(path)
	if err == nil {
		t.Fatal("expected dashboard bind guard error")
	}
}

func TestMigrateLegacyDefaults_OfficerAndWorker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[system]
name = "legacy-all"

[defaults.officer]
model = "officer-model"

[defaults.worker]
model = "worker-model"

[[agents]]
name = "ceo"
tier = "officer"

[[agents]]
name = "drone"
tier = "worker"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.ResolvedAgent("ceo").Model != "officer-model" {
		t.Errorf("officer model from legacy defaults: got %q", cfg.ResolvedAgent("ceo").Model)
	}
	if cfg.ResolvedAgent("drone").Model != "worker-model" {
		t.Errorf("worker model from legacy defaults: got %q", cfg.ResolvedAgent("drone").Model)
	}
}

func TestMigrateLegacyDefaults_DoesNotOverwriteBase(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[base.operator]
model = "explicit-model"

[defaults.operator]
model = "legacy-model"

[[agents]]
name = "concierge"
tier = "operator"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	agent := cfg.ResolvedAgent("concierge")
	if agent.Model != "explicit-model" {
		t.Errorf("base.operator should not be overwritten by legacy defaults: got %q", agent.Model)
	}
}

func TestENVOverrides_TailscaleAndSSH(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`[[agents]]
name = "a"
tier = "worker"
`), 0644)

	t.Setenv("CONOS_INFRA_TAILSCALE_HOSTNAME", "ts-host")
	t.Setenv("CONOS_INFRA_TAILSCALE_LOGIN_SERVER", "http://ts-server")
	t.Setenv("CONOS_SSH_AUTHORIZED_KEYS", "ssh-rsa AAAA\nssh-rsa BBBB")

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Infra.TailscaleHostname != "ts-host" {
		t.Errorf("tailscale hostname: got %q", cfg.Infra.TailscaleHostname)
	}
	if cfg.Infra.TailscaleLoginServer != "http://ts-server" {
		t.Errorf("tailscale login server: got %q", cfg.Infra.TailscaleLoginServer)
	}
	if len(cfg.Infra.SSHAuthorizedKeys) != 2 {
		t.Errorf("ssh authorized keys: got %d, want 2", len(cfg.Infra.SSHAuthorizedKeys))
	}
}

func TestValidate_InvalidTier(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "badagent"
tier = "superoperator"
`), 0644)
	_, err := Parse(path)
	if err == nil {
		t.Error("expected error for invalid tier")
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "badagent"
tier = "worker"
mode = "freestyle"
`), 0644)
	_, err := Parse(path)
	if err == nil {
		t.Error("expected error for invalid mode")
	}
}

func TestValidate_CronMissingExpression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "badagent"
tier = "worker"
mode = "cron"
`), 0644)
	_, err := Parse(path)
	if err == nil {
		t.Error("expected error for cron mode without cron expression")
	}
}

func TestResolvedAgent_NotFound(t *testing.T) {
	cfg := &Config{}
	agent := cfg.ResolvedAgent("nonexistent")
	if agent.Name != "" {
		t.Errorf("expected empty AgentConfig for unknown agent, got name %q", agent.Name)
	}
}

func TestResolvedAgent_DefaultsTierWhenEmpty(t *testing.T) {
	// Call ResolvedAgent directly (bypassing Parse/applyDefaults) to exercise
	// the resolved.Tier == "" fallback inside ResolvedAgent itself.
	cfg := &Config{
		Agents: []AgentConfig{
			{Name: "notier"},
		},
	}
	agent := cfg.ResolvedAgent("notier")
	if agent.Tier != "worker" {
		t.Errorf("ResolvedAgent should default empty tier to worker, got %q", agent.Tier)
	}
}

func TestTierConfig_UnknownTier(t *testing.T) {
	cfg := &Config{}
	tc := cfg.tierConfig("unknown-tier")
	if tc != (TierConfig{}) {
		t.Errorf("expected empty TierConfig for unknown tier, got %+v", tc)
	}
}

func TestApplyDefaults_NoTier(t *testing.T) {
	// Agent with no tier gets defaulted to "worker" by applyDefaults,
	// and ResolvedAgent also defaults resolved.Tier to "worker".
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "notierbot"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if cfg.Agents[0].Tier != "worker" {
		t.Errorf("applyDefaults should set empty tier to worker, got %q", cfg.Agents[0].Tier)
	}
	agent := cfg.ResolvedAgent("notierbot")
	if agent.Tier != "worker" {
		t.Errorf("ResolvedAgent should default empty tier to worker, got %q", agent.Tier)
	}
}

func TestResolvedAgentGlobalDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "bare"
tier = "operator"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("bare")
	if agent.Runner != "picoclaw" {
		t.Errorf("expected default runner picoclaw, got %q", agent.Runner)
	}
	if agent.Provider != "openrouter" {
		t.Errorf("expected default provider openrouter, got %q", agent.Provider)
	}
	if agent.APIKeyEnv != "CONOS_API_KEY" {
		t.Errorf("expected default api_key_env CONOS_API_KEY, got %q", agent.APIKeyEnv)
	}
	if agent.Mode != "on-demand" {
		t.Errorf("expected default mode on-demand, got %q", agent.Mode)
	}
	if agent.MaxSessions != 1 {
		t.Errorf("expected default max_sessions 1, got %d", agent.MaxSessions)
	}
}

func TestResolvedAgent_OAuthForcesClaudeCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "officer"
tier = "officer"
api_key_env = "CLAUDE_CODE_OAUTH_TOKEN"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("officer")
	if agent.Runner != "claude-code" {
		t.Errorf("expected runner forced to claude-code, got %q", agent.Runner)
	}
	if len(agent.RunnerArgs) == 0 || agent.RunnerArgs[0] != "--print" {
		t.Errorf("expected default runner_args [--print], got %v", agent.RunnerArgs)
	}
}

func TestResolvedAgent_OAuthPreservesExplicitArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[[agents]]
name = "officer"
tier = "officer"
api_key_env = "CLAUDE_CODE_OAUTH_TOKEN"
runner_args = ["--model", "claude-opus-4-6", "--print"]
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("officer")
	if agent.Runner != "claude-code" {
		t.Errorf("expected runner forced to claude-code, got %q", agent.Runner)
	}
	if len(agent.RunnerArgs) != 3 {
		t.Errorf("expected explicit runner_args preserved, got %v", agent.RunnerArgs)
	}
}

func TestResolvedAgent_OAuthTokenValueDetection(t *testing.T) {
	// When CONOS_API_KEY contains an OAuth token (sk-ant-oat prefix),
	// the runner should be forced to claude-code even though the env var
	// name is not CLAUDE_CODE_OAUTH_TOKEN.
	t.Setenv("CONOS_TEST_OAUTH_KEY", "sk-ant-oat-fake-token-for-test")

	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`
[base]
api_key_env = "CONOS_TEST_OAUTH_KEY"
provider = "anthropic"

[[agents]]
name = "concierge"
tier = "operator"
`), 0644)

	cfg, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	agent := cfg.ResolvedAgent("concierge")
	if agent.Runner != "claude-code" {
		t.Errorf("expected runner forced to claude-code for OAuth token value, got %q", agent.Runner)
	}
}
