package runtime

import (
	"testing"

	"github.com/ConspiracyOS/conctl/internal/config"
)

func TestBuildPicoConfig(t *testing.T) {
	agent := config.AgentConfig{
		Name:     "concierge",
		Tier:     "operator",
		Runner:   "picoclaw",
		Provider: "openrouter",
		Model:    "google/gemini-2.0-flash-001",
	}

	t.Setenv("CONOS_OPENROUTER_API_KEY", "sk-or-test-key")

	ws := "/srv/conos/agents/concierge/workspace"
	pcfg := BuildPicoConfig(agent, ws)

	if pcfg.Providers.OpenRouter.APIKey != "sk-or-test-key" {
		t.Errorf("expected OpenRouter key, got %q", pcfg.Providers.OpenRouter.APIKey)
	}
	if pcfg.Agents.Defaults.Model != "google/gemini-2.0-flash-001" {
		t.Errorf("expected model, got %q", pcfg.Agents.Defaults.Model)
	}
	if pcfg.Agents.Defaults.Workspace != ws {
		t.Errorf("expected workspace %q, got %q", ws, pcfg.Agents.Defaults.Workspace)
	}
	if pcfg.Agents.Defaults.RestrictToWorkspace {
		t.Error("expected RestrictToWorkspace=false")
	}
	if pcfg.Agents.Defaults.MaxToolIterations != 200 {
		t.Errorf("expected MaxToolIterations=200, got %d", pcfg.Agents.Defaults.MaxToolIterations)
	}
}

func TestBuildPicoConfigDefaultModel(t *testing.T) {
	agent := config.AgentConfig{Name: "sysadmin"}
	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Agents.Defaults.Model != "anthropic/claude-sonnet-4.6" {
		t.Errorf("expected default model, got %q", pcfg.Agents.Defaults.Model)
	}
}

func TestBuildPicoConfigAPIKeyFromEnv(t *testing.T) {
	agent := config.AgentConfig{
		Name:      "test",
		Provider:  "anthropic",
		APIKeyEnv: "MY_CUSTOM_KEY",
	}

	t.Setenv("MY_CUSTOM_KEY", "sk-ant-custom")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.Anthropic.APIKey != "sk-ant-custom" {
		t.Errorf("expected custom key, got %q", pcfg.Providers.Anthropic.APIKey)
	}
}

func TestBuildPicoConfig_OpenAI_WithAPIKeyEnv(t *testing.T) {
	// case "openai" with APIKeyEnv set — apiKey != "" so no fallback env read.
	agent := config.AgentConfig{
		Name:      "test",
		Provider:  "openai",
		APIKeyEnv: "MY_OPENAI_KEY",
	}
	t.Setenv("MY_OPENAI_KEY", "sk-oai-custom")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.OpenAI.APIKey != "sk-oai-custom" {
		t.Errorf("expected OpenAI key from APIKeyEnv, got %q", pcfg.Providers.OpenAI.APIKey)
	}
}

func TestBuildPicoConfig_OpenAI_FallbackEnv(t *testing.T) {
	// case "openai" with no APIKeyEnv — falls back to CONOS_AUTH_OPENAI.
	agent := config.AgentConfig{
		Name:     "test",
		Provider: "openai",
	}
	t.Setenv("CONOS_AUTH_OPENAI", "sk-oai-fallback")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.OpenAI.APIKey != "sk-oai-fallback" {
		t.Errorf("expected OpenAI fallback key, got %q", pcfg.Providers.OpenAI.APIKey)
	}
}

func TestBuildPicoConfig_Anthropic_FallbackEnv(t *testing.T) {
	// case "anthropic" with no APIKeyEnv — falls back to CONOS_AUTH_ANTHROPIC.
	agent := config.AgentConfig{
		Name:     "test",
		Provider: "anthropic",
	}
	t.Setenv("CONOS_AUTH_ANTHROPIC", "sk-ant-fallback")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.Anthropic.APIKey != "sk-ant-fallback" {
		t.Errorf("expected Anthropic fallback key, got %q", pcfg.Providers.Anthropic.APIKey)
	}
}

func TestBuildPicoConfig_Default_OpenRouterKey(t *testing.T) {
	// default case: CONOS_OPENROUTER_API_KEY non-empty — first if body executes.
	agent := config.AgentConfig{Name: "test"}
	t.Setenv("CONOS_OPENROUTER_API_KEY", "sk-or-default")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "")
	t.Setenv("CONOS_AUTH_OPENAI", "")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.OpenRouter.APIKey != "sk-or-default" {
		t.Errorf("expected OpenRouter key from default, got %q", pcfg.Providers.OpenRouter.APIKey)
	}
}

func TestBuildPicoConfig_Default_OpenAIKey(t *testing.T) {
	// default case: CONOS_AUTH_OPENAI non-empty and others empty — third else-if body.
	agent := config.AgentConfig{Name: "test"}
	t.Setenv("CONOS_OPENROUTER_API_KEY", "")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "")
	t.Setenv("CONOS_AUTH_OPENAI", "sk-oai-default")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.OpenAI.APIKey != "sk-oai-default" {
		t.Errorf("expected OpenAI key from default, got %q", pcfg.Providers.OpenAI.APIKey)
	}
}

func TestBuildPicoConfigLegacyFallback(t *testing.T) {
	// No provider set — falls back to legacy env var cascade
	agent := config.AgentConfig{Name: "test"}

	t.Setenv("CONOS_OPENROUTER_API_KEY", "")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "")
	t.Setenv("CONOS_AUTH_OPENAI", "")

	pcfg := BuildPicoConfig(agent, "")
	if pcfg.Providers.OpenRouter.APIKey != "" {
		t.Error("expected no OpenRouter key")
	}

	t.Setenv("CONOS_AUTH_ANTHROPIC", "sk-ant-key")
	t.Setenv("CONOS_AUTH_OPENAI", "sk-oai-key")

	pcfg = BuildPicoConfig(agent, "")
	if pcfg.Providers.Anthropic.APIKey != "sk-ant-key" {
		t.Errorf("expected Anthropic key, got %q", pcfg.Providers.Anthropic.APIKey)
	}
}
