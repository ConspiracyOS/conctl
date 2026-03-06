package runtime

import (
	"os"
	"strings"
)

// knownSecretVars lists env var names that must not be visible to agent
// subprocesses or leaked into the LLM prompt via tool output.
var knownSecretVars = []string{
	"CONOS_OPENROUTER_API_KEY",
	"CONOS_AUTH_ANTHROPIC",
	"CONOS_AUTH_OPENAI",
	"TS_AUTHKEY",
	"TS_AUTH_KEY",
}

// ClearSecretEnv removes known secret env vars from the current process
// environment. Call this after API keys have been captured into config structs.
// extra allows callers to clear additional vars (e.g. per-agent APIKeyEnv names).
func ClearSecretEnv(extra ...string) {
	for _, name := range knownSecretVars {
		os.Unsetenv(name)
	}
	for _, name := range extra {
		if name != "" {
			os.Unsetenv(name)
		}
	}
}

// SanitizedEnv returns os.Environ() with known secret vars removed.
// Use this as cmd.Env when spawning subprocesses that should not inherit secrets.
// extra allows callers to filter additional vars (e.g. per-agent APIKeyEnv names).
func SanitizedEnv(extra ...string) []string {
	secret := make(map[string]bool, len(knownSecretVars)+len(extra))
	for _, name := range knownSecretVars {
		secret[name] = true
	}
	for _, name := range extra {
		if name != "" {
			secret[name] = true
		}
	}
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		if !secret[name] {
			out = append(out, e)
		}
	}
	return out
}
