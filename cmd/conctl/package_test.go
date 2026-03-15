package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no args",
			args: []string{},
			want: []string{},
		},
		{
			name: "only positional",
			args: []string{"curl", "jq"},
			want: []string{"curl", "jq"},
		},
		{
			name: "only flags",
			args: []string{"--agent", "concierge", "--save"},
			want: []string{"--agent", "concierge", "--save"},
		},
		{
			name: "positional then flags",
			args: []string{"curl", "--agent", "concierge"},
			want: []string{"--agent", "concierge", "curl"},
		},
		{
			name: "positional between flags",
			args: []string{"--agent", "concierge", "curl", "--save"},
			want: []string{"--agent", "concierge", "--save", "curl"},
		},
		{
			name: "flags already first",
			args: []string{"--agent", "concierge", "--save", "curl"},
			want: []string{"--agent", "concierge", "--save", "curl"},
		},
		{
			name: "single positional",
			args: []string{"curl"},
			want: []string{"curl"},
		},
		{
			name: "single flag without value",
			args: []string{"--save"},
			want: []string{"--save"},
		},
		{
			name: "flag at end with no value treated as boolean",
			args: []string{"curl", "--save"},
			want: []string{"--save", "curl"},
		},
		{
			name: "multiple positional args after flag",
			args: []string{"--agent", "concierge", "curl", "jq"},
			want: []string{"--agent", "concierge", "curl", "jq"},
		},
		{
			name: "dash-prefixed value consumed by flag",
			args: []string{"curl", "--agent", "--save"},
			// --agent sees --save as a flag, not as its value
			want: []string{"--agent", "--save", "curl"},
		},
		{
			name: "short flag with value",
			args: []string{"curl", "-a", "concierge"},
			want: []string{"-a", "concierge", "curl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgs(tt.args)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("reorderArgs(%v) = %v, want %v", tt.args, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("reorderArgs(%v) = %v, want %v", tt.args, got, tt.want)
				}
			}
		})
	}
}

func TestSavePackageToConfig(t *testing.T) {
	tests := []struct {
		name      string
		initial   string
		pkg       string
		agent     string
		wantErr   bool
		wantInErr string
		check     func(t *testing.T, content string)
	}{
		{
			name: "add package to agent without packages field",
			initial: `[system]
hostname = "conos"

[[agents]]
name = "concierge"
model = "claude-sonnet"
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if !strings.Contains(content, `packages = ["curl"]`) {
					t.Fatalf("expected packages line, got:\n%s", content)
				}
			},
		},
		{
			name: "append to existing packages array",
			initial: `[[agents]]
name = "concierge"
packages = ["curl"]
model = "claude-sonnet"
`,
			pkg:   "jq",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if !strings.Contains(content, `"curl"`) || !strings.Contains(content, `"jq"`) {
					t.Fatalf("expected both packages, got:\n%s", content)
				}
			},
		},
		{
			name: "duplicate package is a no-op",
			initial: `[[agents]]
name = "concierge"
packages = ["curl"]
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				count := strings.Count(content, `"curl"`)
				if count != 1 {
					t.Fatalf("expected exactly 1 occurrence of curl, got %d in:\n%s", count, content)
				}
			},
		},
		{
			name: "agent not found returns error",
			initial: `[[agents]]
name = "sysadmin"
`,
			pkg:       "curl",
			agent:     "concierge",
			wantErr:   true,
			wantInErr: "not found",
		},
		{
			name: "correct agent targeted among multiple",
			initial: `[[agents]]
name = "sysadmin"
packages = ["git"]

[[agents]]
name = "concierge"
model = "claude-sonnet"
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if !strings.Contains(content, `packages = ["curl"]`) {
					t.Fatalf("expected packages line for concierge, got:\n%s", content)
				}
				// sysadmin's packages should be unchanged
				if !strings.Contains(content, `packages = ["git"]`) {
					t.Fatalf("sysadmin packages should be unchanged, got:\n%s", content)
				}
			},
		},
		{
			name: "insert packages before empty line ending block",
			initial: `[[agents]]
name = "concierge"

[system]
hostname = "conos"
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if !strings.Contains(content, `packages = ["curl"]`) {
					t.Fatalf("expected packages line, got:\n%s", content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "conos.toml")
			if err := os.WriteFile(configPath, []byte(tt.initial), 0644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CONOS_CONFIG", configPath)

			err := savePackageToConfig(tt.pkg, tt.agent)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantInErr != "" && !strings.Contains(err.Error(), tt.wantInErr) {
					t.Fatalf("error %q should contain %q", err.Error(), tt.wantInErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, string(data))
		})
	}
}

func TestRemovePackageFromConfig(t *testing.T) {
	tests := []struct {
		name  string
		initial string
		pkg     string
		agent   string
		check   func(t *testing.T, content string)
	}{
		{
			name: "remove only package leaves empty array",
			initial: `[[agents]]
name = "concierge"
packages = ["curl"]
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if strings.Contains(content, `"curl"`) {
					t.Fatalf("curl should have been removed, got:\n%s", content)
				}
				if !strings.Contains(content, "packages = []") {
					t.Fatalf("expected empty packages array, got:\n%s", content)
				}
			},
		},
		{
			name: "remove one of multiple packages",
			initial: `[[agents]]
name = "concierge"
packages = ["curl", "jq", "git"]
`,
			pkg:   "jq",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if strings.Contains(content, `"jq"`) {
					t.Fatalf("jq should have been removed, got:\n%s", content)
				}
				if !strings.Contains(content, `"curl"`) || !strings.Contains(content, `"git"`) {
					t.Fatalf("curl and git should remain, got:\n%s", content)
				}
			},
		},
		{
			name: "remove non-existent package is a no-op",
			initial: `[[agents]]
name = "concierge"
packages = ["curl"]
`,
			pkg:   "jq",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if !strings.Contains(content, `"curl"`) {
					t.Fatalf("curl should still be present, got:\n%s", content)
				}
			},
		},
		{
			name: "correct agent targeted among multiple",
			initial: `[[agents]]
name = "sysadmin"
packages = ["git", "curl"]

[[agents]]
name = "concierge"
packages = ["curl", "jq"]
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				// concierge should have curl removed
				lines := strings.Split(content, "\n")
				inSysadmin := false
				inConcierge := false
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.Contains(trimmed, `"sysadmin"`) {
						inSysadmin = true
						inConcierge = false
					}
					if strings.Contains(trimmed, `"concierge"`) {
						inConcierge = true
						inSysadmin = false
					}
					if strings.HasPrefix(trimmed, "packages") {
						if inSysadmin && !strings.Contains(line, `"curl"`) {
							t.Fatalf("sysadmin should still have curl, got: %s", line)
						}
						if inConcierge && strings.Contains(line, `"curl"`) {
							t.Fatalf("concierge should not have curl, got: %s", line)
						}
					}
				}
			},
		},
		{
			name: "agent without packages field is a no-op",
			initial: `[[agents]]
name = "concierge"
model = "claude-sonnet"
`,
			pkg:   "curl",
			agent: "concierge",
			check: func(t *testing.T, content string) {
				if strings.Contains(content, "curl") {
					t.Fatalf("should not have added curl, got:\n%s", content)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "conos.toml")
			if err := os.WriteFile(configPath, []byte(tt.initial), 0644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CONOS_CONFIG", configPath)

			err := removePackageFromConfig(tt.pkg, tt.agent)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, string(data))
		})
	}
}
