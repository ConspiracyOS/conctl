package runtime

import (
	"os"
	"strings"
	"testing"
)

func TestClearSecretEnv_KnownVars(t *testing.T) {
	t.Setenv("CONOS_OPENROUTER_API_KEY", "sk-or-test")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "sk-ant-test")
	t.Setenv("CONOS_AUTH_OPENAI", "sk-oai-test")
	t.Setenv("TS_AUTHKEY", "tskey-test")

	ClearSecretEnv()

	for _, name := range []string{
		"CONOS_OPENROUTER_API_KEY",
		"CONOS_AUTH_ANTHROPIC",
		"CONOS_AUTH_OPENAI",
		"TS_AUTHKEY",
	} {
		if v := os.Getenv(name); v != "" {
			t.Errorf("expected %s to be cleared, got %q", name, v)
		}
	}
}

func TestClearSecretEnv_Extra(t *testing.T) {
	t.Setenv("MY_CUSTOM_API_KEY", "custom-secret")

	ClearSecretEnv("MY_CUSTOM_API_KEY")

	if v := os.Getenv("MY_CUSTOM_API_KEY"); v != "" {
		t.Errorf("expected MY_CUSTOM_API_KEY to be cleared, got %q", v)
	}
}

func TestClearSecretEnv_EmptyExtra(t *testing.T) {
	// Empty string in extra must not panic.
	ClearSecretEnv("")
	ClearSecretEnv("", "")
}

func TestSanitizedEnv_ExcludesSecrets(t *testing.T) {
	t.Setenv("CONOS_OPENROUTER_API_KEY", "sk-or-secret")
	t.Setenv("CONOS_AUTH_ANTHROPIC", "sk-ant-secret")
	t.Setenv("TS_AUTHKEY", "tskey-secret")
	t.Setenv("SAFE_VAR", "safe-value")

	env := SanitizedEnv()

	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		switch name {
		case "CONOS_OPENROUTER_API_KEY", "CONOS_AUTH_ANTHROPIC", "TS_AUTHKEY":
			t.Errorf("secret var %s must not appear in SanitizedEnv", name)
		}
	}

	found := false
	for _, e := range env {
		if e == "SAFE_VAR=safe-value" {
			found = true
		}
	}
	if !found {
		t.Error("SAFE_VAR should be present in SanitizedEnv")
	}
}

func TestSanitizedEnv_DoesNotMutateProcess(t *testing.T) {
	t.Setenv("CONOS_OPENROUTER_API_KEY", "sk-or-still-set")

	_ = SanitizedEnv()

	// SanitizedEnv must not unset vars from the process — it only filters the slice.
	if v := os.Getenv("CONOS_OPENROUTER_API_KEY"); v == "" {
		t.Error("SanitizedEnv must not modify the process environment")
	}
}
