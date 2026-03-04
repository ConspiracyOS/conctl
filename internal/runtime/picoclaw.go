package runtime

import (
	"context"
	"fmt"
	"os"

	pcagent "github.com/sipeed/picoclaw/pkg/agent"
	"github.com/sipeed/picoclaw/pkg/bus"
	pcconfig "github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"

	conconfig "github.com/ConspiracyOS/conctl/internal/config"
)

// PicoClaw runs agents using the in-process PicoClaw library.
type PicoClaw struct {
	Agent     conconfig.AgentConfig
	Workspace string
}

func (p *PicoClaw) Invoke(ctx context.Context, prompt, sessionKey string) (string, error) {
	cfg := BuildPicoConfig(p.Agent, p.Workspace)
	// Remove secret env vars now that API keys are captured in cfg.
	// Tool subprocesses spawned by PicoClaw inherit the process environment;
	// clearing here prevents secrets from reaching the model via tool output.
	ClearSecretEnv(p.Agent.APIKeyEnv)

	provider, err := providers.CreateProvider(cfg)
	if err != nil {
		return "", fmt.Errorf("creating LLM provider: %w", err)
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	loop := pcagent.NewAgentLoop(cfg, msgBus, provider)

	return loop.ProcessDirect(ctx, prompt, sessionKey)
}

// BuildPicoConfig creates a PicoClaw config from a ConspiracyOS agent config.
// workspace is the agent's working directory (e.g. /srv/conos/agents/<name>/workspace).
func BuildPicoConfig(agent conconfig.AgentConfig, workspace string) *pcconfig.Config {
	model := agent.Model
	if model == "" {
		model = "anthropic/claude-sonnet-4.6"
	}

	cfg := pcconfig.DefaultConfig()
	cfg.Agents.Defaults.Workspace = workspace
	cfg.Agents.Defaults.RestrictToWorkspace = false
	cfg.Agents.Defaults.Model = model
	cfg.Agents.Defaults.MaxTokens = 8192
	cfg.Agents.Defaults.MaxToolIterations = 200

	// Resolve API key from config. The agent's APIKeyEnv field names the env var.
	// Falls back to legacy env vars for backwards compatibility.
	apiKey := ""
	if agent.APIKeyEnv != "" {
		apiKey = os.Getenv(agent.APIKeyEnv)
	}

	switch agent.Provider {
	case "openrouter":
		if apiKey == "" {
			apiKey = os.Getenv("CONOS_OPENROUTER_API_KEY")
		}
		cfg.Providers.OpenRouter = pcconfig.ProviderConfig{APIKey: apiKey}
	case "anthropic":
		if apiKey == "" {
			apiKey = os.Getenv("CONOS_AUTH_ANTHROPIC")
		}
		cfg.Providers.Anthropic = pcconfig.ProviderConfig{APIKey: apiKey}
	case "openai":
		if apiKey == "" {
			apiKey = os.Getenv("CONOS_AUTH_OPENAI")
		}
		cfg.Providers.OpenAI = pcconfig.ProviderConfig{APIKey: apiKey}
	default:
		// No provider specified or unknown — try legacy env var cascade
		if key := os.Getenv("CONOS_OPENROUTER_API_KEY"); key != "" {
			cfg.Providers.OpenRouter = pcconfig.ProviderConfig{APIKey: key}
		} else if key := os.Getenv("CONOS_AUTH_ANTHROPIC"); key != "" {
			cfg.Providers.Anthropic = pcconfig.ProviderConfig{APIKey: key}
		} else if key := os.Getenv("CONOS_AUTH_OPENAI"); key != "" {
			cfg.Providers.OpenAI = pcconfig.ProviderConfig{APIKey: key}
		}
	}

	return cfg
}
