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
| `conctl doctor` | on-demand sysadmin report |
| `conctl artifact create` | create an artifact manifest + file + local link path |
| `conctl artifact show` | inspect artifact metadata |
| `conctl artifact link` | mint a signed artifact link |
| `conctl artifact verify` | verify a signed artifact link |
| `conctl task-contract ...` | manage contract-backed task state |
| `conctl status` | `conos status` (via conos) |
| `conctl logs` | `conos agent logs` (via conos) |
| `conctl responses` | `conos agent` (via conos) |

`conctl healthcheck` now annotates every evaluation with `run_id` and `actor`
(from `CONOS_RUN_ID` / `CONOS_ACTOR`, or generated defaults) and persists:

- `contracts-state.latest.json` (latest full contract state snapshot)
- `contracts-state.diff.jsonl` (append-only failure/resolution diff ledger)

Contract selection is tag-driven (default tag set: `schedule`).
Override with `CONOS_CONTRACT_TAGS` (comma-separated).

`runner` now supports optional budget gating via config:
- `contracts.daily_budget_usd`
- `contracts.estimated_cost_per_run_usd`

Artifacts are stored under `/srv/conos/artifacts/` with a JSON manifest and, when
exposed, a stable local dashboard link path under `/artifacts/<artifact-id>/...`
published through `/srv/conos/status/`.

Signed artifact links use the root-owned HMAC key at `/etc/conos/artifact-signing.key`
(provisioned during bootstrap). This gives channels a stable way to mint/verify
user-facing links without exposing raw filesystem paths.

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
