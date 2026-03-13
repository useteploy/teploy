<p align="center">
  <h1 align="center">teploy</h1>
  <p align="center">Zero-downtime Docker deploys to any server via SSH.<br>Single binary. No management server. No dependencies.</p>
</p>

<p align="center">
  <a href="https://github.com/useteploy/teploy/releases"><img src="https://img.shields.io/github/v/release/useteploy/teploy" alt="Release"></a>
  <a href="https://github.com/useteploy/teploy/actions"><img src="https://github.com/useteploy/teploy/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/useteploy/teploy/blob/main/LICENSE"><img src="https://img.shields.io/github/license/useteploy/teploy" alt="License"></a>
</p>

---

## Why teploy?

Most deploy tools require either a management server (Coolify, Dokploy) or complex configuration (Kamal). Teploy is a single binary that deploys Docker containers to any server you can SSH into. Three lines of config, one command to deploy.

```yaml
# teploy.yml
app: myapp
domain: myapp.com
server: 1.2.3.4
```

```bash
teploy deploy
```

Your app is live with HTTPS, zero-downtime deploys, and automatic rollback on failure.

## Install

```bash
# macOS (Homebrew)
brew install useteploy/tap/teploy

# Windows (Scoop)
scoop bucket add teploy https://github.com/useteploy/scoop-bucket
scoop install teploy

# Download binary (macOS/Linux)
curl -fsSL https://github.com/useteploy/teploy/releases/latest/download/teploy_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/').tar.gz | tar xz
sudo mv teploy /usr/local/bin/

# From source
go install github.com/useteploy/teploy/cmd/teploy@latest
```

## Quickstart

```bash
# 1. Generate config
teploy init

# 2. Provision your server (installs Docker + Caddy)
teploy setup <your-server-ip>

# 3. Deploy
teploy deploy
```

## How it works

1. **Builds** your app (Dockerfile auto-detection or Nixpacks)
2. **Starts** a new container alongside the old one
3. **Health checks** the new container
4. **Routes traffic** via Caddy (automatic HTTPS)
5. **Stops** the old container — zero downtime
6. **Rolls back** automatically if anything fails

## Features

| Feature | Description |
|---|---|
| **Zero-downtime deploys** | New container starts and passes health checks before old one stops |
| **Automatic HTTPS** | Caddy provisions and renews TLS certificates |
| **Rollback** | `teploy rollback` reverts to the previous version instantly |
| **Multi-process** | Run web, worker, and scheduler from the same image |
| **Accessories** | Manage Postgres, Redis, etc. alongside your app |
| **Environment variables** | `teploy env set KEY=value` — stored securely on server |
| **Encrypted secrets** | `teploy secret set KEY value` — encrypted at rest |
| **Preview environments** | `teploy preview <branch>` — deploy branches to temporary URLs |
| **Multi-server** | Deploy to multiple servers with parallel rolling deploys |
| **Load balancing** | Automatic Caddy LB config across app servers |
| **Maintenance mode** | `teploy maintenance on` — instant 503 page |
| **Deploy hooks** | Run migrations before deploy, seed after deploy |
| **Asset bridging** | Shared volume prevents 404s on hashed assets during deploys |
| **Destinations** | `teploy deploy -d staging` — per-environment config overlays |
| **Notifications** | Webhook notifications on deploy/rollback/failure |
| **Backups** | `teploy backup` — volumes to S3 |
| **Templates** | One-command deploys for common self-hosted apps |
| **Deploy locking** | `teploy lock` — freeze deploys during incidents |

## Config

Minimum config is 3 lines. Everything else is optional:

```yaml
app: myapp
domain: myapp.com
server: 1.2.3.4
port: 3000
build_local: true
platform: linux/amd64
stop_timeout: 30

volumes:
  data: /app/data

processes:
  web: "npm start"
  worker: "npm run worker"

hooks:
  pre_deploy: "npm run migrate"
  post_deploy: "npm run seed"

accessories:
  postgres:
    image: postgres:16
    port: 5432
    env:
      POSTGRES_PASSWORD: secret

assets:
  path: /app/public/assets
  keep_days: 14

notifications:
  webhook: https://hooks.slack.com/services/xxx
```

TOML is also supported (`teploy.toml`).

## Commands

```
teploy init                    # generate config
teploy setup <server>          # provision server (Docker + Caddy)
teploy deploy                  # deploy app
teploy deploy -d staging       # deploy with environment overlay
teploy rollback                # revert to previous version
teploy stop / start / restart  # container lifecycle
teploy logs                    # tail container logs
teploy status                  # show running containers
teploy stats                   # CPU/RAM per container
teploy health                  # run health check
teploy log                     # deploy history
teploy env set KEY=value       # set environment variable
teploy env list                # list env vars (masked)
teploy secret set KEY value    # encrypted secrets
teploy maintenance on/off      # maintenance mode (503 page)
teploy lock / unlock           # freeze/unfreeze deploys
teploy exec <server> <cmd>     # run command on server
teploy scale <count>           # multi-server deploy + LB update
teploy preview <branch>        # preview environments
teploy validate                # check config and server readiness
teploy server add/remove/list  # manage server fleet
teploy accessory start/stop    # manage databases and caches
teploy backup                  # backup volumes to S3
teploy template deploy <name>  # deploy from community templates
```

## Multi-server deploys

```yaml
app: myapp
domain: myapp.com
servers:
  - app1
  - app2
  - app3
parallel: 2
```

```bash
teploy deploy          # deploys to all servers in parallel
teploy scale 3         # deploy to 3 app-role servers + update LB
```

## Destinations

Manage multiple environments with config overlays:

```bash
# teploy.yml         — base config
# teploy.staging.yml — staging overrides (domain, server, etc.)
teploy deploy -d staging
```

The overlay merges on top of base config — override only what differs per environment.

## Requirements

- A server with SSH access (any Linux VPS — Hetzner, DigitalOcean, Linode, etc.)
- That's it. `teploy setup` handles the rest.

## Comparison

| | teploy | Kamal | Coolify | Dokploy |
|---|---|---|---|---|
| Management server required | No | No | Yes | Yes |
| Config lines to deploy | 3 | ~20 | GUI | GUI |
| Single binary | Yes | No (Ruby) | No | No |
| Auto HTTPS | Caddy | Kamal Proxy | Traefik | Traefik |
| Build from source | Dockerfile + Nixpacks | Dockerfile only | Nixpacks | Nixpacks |
| Preview environments | Yes | No | Yes | Yes |
| Maintenance mode | Yes | No | No | No |
| Templates | Yes | No | Yes | Yes |

## License

[MIT](LICENSE)
