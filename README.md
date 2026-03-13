# teploy

Zero-downtime Docker deploys to any server via SSH. Single binary, no management server.

## Install

```bash
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

That's it. Your app is live with HTTPS, zero-downtime deploys, and automatic rollback on failure.

## What it does

- Builds your app (Dockerfile or Nixpacks auto-detection)
- Starts a new container, runs health checks
- Routes traffic via Caddy (automatic HTTPS)
- Stops the old container — zero downtime
- Rolls back automatically if health checks fail

## Config

```yaml
# teploy.yml
app: myapp
domain: myapp.com
server: 1.2.3.4
```

That's the minimum. Everything else is optional:

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
teploy setup <server>          # provision server
teploy deploy                  # deploy app
teploy deploy -d staging       # deploy with destination overlay
teploy rollback                # revert to previous version
teploy stop / start / restart  # container lifecycle
teploy logs                    # tail container logs
teploy status                  # show running containers
teploy stats                   # CPU/RAM per container
teploy health                  # run health check
teploy log                     # deploy history
teploy env set KEY=value       # environment variables
teploy env list                # list env vars (masked)
teploy secret set KEY value    # encrypted secrets
teploy maintenance on/off      # maintenance mode (503 page)
teploy lock / unlock           # freeze/unfreeze deploys
teploy exec <server> <cmd>     # run command on server
teploy scale <count>           # multi-server deploy
teploy preview <branch>        # preview environments
teploy validate                # check config
teploy server add/remove/list  # manage server fleet
teploy accessory start/stop    # manage databases, caches
teploy backup                  # backup volumes to S3
teploy template deploy <name>  # deploy from templates
```

## Multi-server

```yaml
# teploy.yml
app: myapp
domain: myapp.com
servers:
  - app1
  - app2
  - app3
parallel: 2
```

```bash
teploy deploy          # deploys to all servers
teploy scale 3         # deploy to 3 app-role servers + update LB
```

## Destinations (environments)

```bash
# Base config: teploy.yml
# Staging overlay: teploy.staging.yml
teploy deploy -d staging
```

The overlay merges on top of the base config — override domain, server, env vars per environment.

## Requirements

- A server with SSH access (any Linux VPS)
- That's it. `teploy setup` installs Docker and Caddy for you.

## License

MIT
