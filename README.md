# conctl — ConspiracyOS inner runtime

`conctl` is the control binary that runs **inside** the ConspiracyOS container or VM.
It is not user-facing — operators use [`conos`](../conos) instead.

## What it does

`conctl bootstrap` provisions the full Linux environment on first boot:
Linux users (`a-<name>`), `/srv/conos/` directory tree, POSIX ACLs,
sudoers rules, and systemd path/service/timer units per agent.
Bootstrap is idempotent — safe to re-run after config changes.

All other subcommands are invoked by systemd units, not humans:

| Command | Triggered by |
|---------|-------------|
| `conctl run <agent>` | systemd path unit (inbox watcher) |
| `conctl route-inbox` | `conos-outer-inbox.service` |
| `conctl healthcheck` | `conos-healthcheck.timer` |
| `conctl status` | `conos status` (via conos) |
| `conctl logs` | `conos agent logs` (via conos) |
| `conctl responses` | `conos agent` (via conos) |

## Runners

`conctl` supports two runner types:

**PicoClaw** (default, built-in): compiled into the binary. Talks to OpenRouter,
Anthropic, or OpenAI. Configure via `[base]` in `conos.toml`.

**Exec** (BYOR): any stdin/stdout binary on PATH. Set `runner = "my-binary"` on
an agent. Per-agent env vars inject credentials via systemd `Environment=` directives.

```toml
[[agents]]
name = "analyst"
runner = "ollama-run-llama3"
environment = ["OLLAMA_HOST=http://192.168.64.1:11434"]
```

## Configuration

Config file: `/etc/conos/conos.toml` (inside container/VM).
See [`configs/example.toml`](./configs/example.toml) for all options.

Resolution order: `agent > base.<tier> > base`

## Tiers

Tiers are permission profiles enforced by systemd hardening and Linux groups:

| Tier | Systemd hardening | Can write to |
|------|------------------|--------------|
| `officer` | NoNewPrivileges, ProtectSystem=strict | agents/, artifacts/, audit/ |
| `operator` | NoNewPrivileges, ProtectSystem=strict | agents/, artifacts/, audit/ |
| `worker` | + BindReadOnlyPaths | own inbox/outbox only |
| (sysadmin role) | none | broad: etc/conos, systemd, sudoers |

## Build

```bash
go build -o conctl ./cmd/conctl/

# Cross-compile for container
GOOS=linux GOARCH=arm64 go build -o conctl-linux-arm64 ./cmd/conctl/
GOOS=linux GOARCH=amd64 go build -o conctl-linux-amd64 ./cmd/conctl/
```

## Test

```bash
go test ./...
go vet ./...
```
