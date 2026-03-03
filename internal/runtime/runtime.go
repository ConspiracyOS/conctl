package runtime

import (
	"context"

	"github.com/ConspiracyOS/conctl/internal/config"
)

// Runtime executes an agent prompt and returns the response.
type Runtime interface {
	Invoke(ctx context.Context, prompt, sessionKey string) (string, error)
}

// New returns the appropriate runtime for an agent based on its runner config.
// workspace is the agent's working directory; the caller computes it from Dirs.
// "picoclaw" (the default) uses the in-process PicoClaw library.
// Any other value uses the exec runtime with that value as the command.
func New(agent config.AgentConfig, workspace string) Runtime {
	switch agent.Runner {
	case "picoclaw", "":
		return &PicoClaw{Agent: agent, Workspace: workspace}
	default:
		return &Exec{
			Cmd:       agent.Runner,
			Args:      agent.RunnerArgs,
			Workspace: workspace,
		}
	}
}
