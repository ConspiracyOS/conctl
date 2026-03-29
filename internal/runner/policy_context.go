package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ConspiracyOS/conctl/internal/config"
)

const (
	maxPolicyFileBytes  = 4 * 1024
	maxPolicyTotalBytes = 16 * 1024
)

type PolicyContext struct {
	Prompt string
	Refs   []string
}

type policySource struct {
	Label   string
	Ref     string
	Content string
}

// LoadPolicyContext collects standing policy files for an agent.
// Officers can update these files under /srv/conos/policy without changing code.
func LoadPolicyContext(policyDir string, agent config.AgentConfig) PolicyContext {
	sources := loadPolicySources(policyDir, agent)
	if len(sources) == 0 {
		return PolicyContext{}
	}

	var (
		b    strings.Builder
		refs []string
		size int
	)
	b.WriteString("Treat these standing guardrails as binding operating policy unless a direct human instruction overrides them.\n")
	size = b.Len()

	for _, source := range sources {
		section := fmt.Sprintf("\n\n## %s\n\n%s", source.Label, source.Content)
		if size+len(section) > maxPolicyTotalBytes {
			break
		}
		b.WriteString(section)
		size += len(section)
		refs = append(refs, source.Ref)
	}

	return PolicyContext{
		Prompt: strings.TrimSpace(b.String()),
		Refs:   refs,
	}
}

func loadPolicySources(policyDir string, agent config.AgentConfig) []policySource {
	if strings.TrimSpace(policyDir) == "" {
		return nil
	}

	var candidates []struct {
		label string
		ref   string
		path  string
	}
	candidates = append(candidates,
		struct {
			label string
			ref   string
			path  string
		}{
			label: "Global Policy",
			ref:   "global.md",
			path:  filepath.Join(policyDir, "global.md"),
		},
	)
	if agent.Tier != "" {
		ref := filepath.ToSlash(filepath.Join("tiers", agent.Tier+".md"))
		candidates = append(candidates, struct {
			label string
			ref   string
			path  string
		}{
			label: fmt.Sprintf("Tier Policy: %s", agent.Tier),
			ref:   ref,
			path:  filepath.Join(policyDir, "tiers", agent.Tier+".md"),
		})
	}

	for _, group := range uniqueSorted(agent.Groups) {
		ref := filepath.ToSlash(filepath.Join("groups", group+".md"))
		candidates = append(candidates, struct {
			label string
			ref   string
			path  string
		}{
			label: fmt.Sprintf("Group Policy: %s", group),
			ref:   ref,
			path:  filepath.Join(policyDir, "groups", group+".md"),
		})
	}

	ref := filepath.ToSlash(filepath.Join("agents", agent.Name+".md"))
	candidates = append(candidates, struct {
		label string
		ref   string
		path  string
	}{
		label: fmt.Sprintf("Agent Policy: %s", agent.Name),
		ref:   ref,
		path:  filepath.Join(policyDir, "agents", agent.Name+".md"),
	})

	var sources []policySource
	for _, candidate := range candidates {
		data, err := os.ReadFile(candidate.path)
		if err != nil {
			continue
		}
		content := clampString(strings.TrimSpace(string(data)), maxPolicyFileBytes)
		if content == "" {
			continue
		}
		sources = append(sources, policySource{
			Label:   candidate.label,
			Ref:     candidate.ref,
			Content: content,
		})
	}

	return sources
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
