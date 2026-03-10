package main

import (
	"os"
	"path/filepath"
	"testing"
)

type artifactJSON struct {
	ID       string `json:"artifact_id"`
	Title    string `json:"title"`
	LinkPath string `json:"link_path"`
}

type signedLinkJSON struct {
	URL string `json:"url"`
}

type taskContractJSON struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

func TestLoadConfig_FromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conos.toml")
	os.WriteFile(path, []byte(`[[agents]]
name = "test"
tier = "worker"
`), 0644)
	t.Setenv("CONOS_CONFIG", path)

	cfg := loadConfig()
	if len(cfg.Agents) != 1 || cfg.Agents[0].Name != "test" {
		t.Errorf("loadConfig returned unexpected config: %+v", cfg)
	}
}
