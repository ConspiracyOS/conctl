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
	// cfg is built once on the first Invoke call. API key env vars are
	// captured into cfg and then cleared from the process environment so
	// tool subprocesses cannot inherit them. Caching cfg here ensures the
	// key survives across multiple Invoke calls (e.g. in runAgentLoop).
	cfg *pcconfig.Config
}

func (p *PicoClaw) Invoke(ctx context.Context, prompt, sessionKey string) (string, error) {
	if p.cfg == nil {
		p.cfg = BuildPicoConfig(p.Agent, p.Workspace)
		// Remove secret env vars now that API keys are captured in cfg.
		// Tool subprocesses spawned by PicoClaw inherit the process environment;
		// clearing here prevents secrets from reaching the model via tool output.
		ClearSecretEnv(p.Agent.APIKeyEnv)
	}

	// When the Anthropic provider is configured with a key (or OAuth token),
	// use NewClaudeProvider directly — it sends Authorization: Bearer <token>
	// via the Anthropic SDK, which supports both API keys and OAuth tokens.
	// CreateProvider routes anthropic+apiKey through the OpenAI-compatible HTTP
	// provider, which uses the wrong endpoint format for native Anthropic API.
	var provider providers.LLMProvider
	if p.cfg.Providers.Anthropic.APIKey != "" {
		provider = providers.NewClaudeProvider(p.cfg.Providers.Anthropic.APIKey)
	} else {
		var err error
		provider, err = providers.CreateProvider(p.cfg)
		if err != nil {
			return "", fmt.Errorf("creating LLM provider: %w", err)
		}
	}

	msgBus := bus.NewMessageBus()
	defer msgBus.Close()

	loop := pcagent.NewAgentLoop(p.cfg, msgBus, provider)

	return loop.ProcessDirect(ctx, prompt, sessionKey)
}

// BuildPicoConfig creates a PicoClaw config from a ConspiracyOS agent config.
// workspace is the agent's working directory (e.g. /srv/conos/agents/<name>/workspace).
func BuildPicoConfig(agent conconfig.AgentConfig, workspace string) *pcconfig.Config {
	model := agent.Model
	if model == "" {
		model = "claude-sonnet-4-6"
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
		cfg.Providers.OpenRouter = pcconfig.ProviderConfig{APIKey: apiKey, APIBase: agent.APIBase}
	case "anthropic":
		if apiKey == "" {
			apiKey = os.Getenv("CONOS_AUTH_ANTHROPIC")
		}
		cfg.Providers.Anthropic = pcconfig.ProviderConfig{APIKey: apiKey, APIBase: agent.APIBase}
	case "openai":
		if apiKey == "" {
			apiKey = os.Getenv("CONOS_AUTH_OPENAI")
		}
		cfg.Providers.OpenAI = pcconfig.ProviderConfig{APIKey: apiKey, APIBase: agent.APIBase}
	case "ollama":
		if apiKey == "" {
			apiKey = "ollama" // Ollama doesn't require auth but PicoClaw needs a non-empty key
		}
		base := agent.APIBase
		if base == "" {
			base = "http://localhost:11434/v1"
		}
		cfg.Providers.Ollama = pcconfig.ProviderConfig{APIKey: apiKey, APIBase: base}
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
