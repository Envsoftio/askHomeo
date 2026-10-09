# Production deployment

Every push to `main` runs Go tests and the frontend build, then syncs the source to the server and rebuilds the production containers. The workflow can also be run manually against `main`. Deployments are serialized. Failed checks prevent deployment; image builds finish before containers are replaced. Container health checks must pass for deployment to succeed. There may be brief downtime during replacement; this is not a rolling deployment and failed replacements are not automatically rolled back.

## Server setup

Use a Linux server with Docker Engine, Docker Compose v2.24 or newer, Bash, rsync, and SSH. The deployment user must be able to run Docker without interactive sudo and write to the app directory. Docker access gives this SSH account control of the host; use a dedicated deployment account.

Choose a dedicated directory under `/home/USER`, `/srv`, or `/opt`, for example `/home/deploy/askhomeo`. Rsync deletes obsolete code in this directory, so do not use a directory shared with other applications. Create it as the deployment user and prepare the environment file **before the first workflow run**:

```sh
mkdir -p /home/deploy/askhomeo
# Copy the repository's .env.prod.example to this server directory first.
cd /home/deploy/askhomeo
cp .env.prod.example .env.prod
chmod 600 .env.prod
# Edit .env.prod with actual passwords, tokens, and the selected AI provider settings.
```

Use distinct random API and reviewer tokens of at least 24 characters. Use a URL-safe database password, for example one generated with `openssl rand -hex 32`. App credentials live only in `.env.prod` on the server; the workflow preserves this file and does not upload app credentials from GitHub. Changing `POSTGRES_PASSWORD` in the file does not update a password in an already initialized database: rotate the database role password as well.

The production example uses DeepInfra for chat and embeddings. Set `AI_PROVIDER=deepinfra`, `EMBEDDING_PROVIDER=deepinfra`, `CHAT_PROVIDER=deepinfra`, and `DEEPINFRA_API_KEY`; `AI_BASE_URL` is not needed. Both explicit provider selectors override `AI_PROVIDER`, so change all three when switching an existing installation. Use `zai-org/GLM-5.3` with `CHAT_REASONING_EFFORT=low`, and `BAAI/bge-m3` with plain input, 1,024 dimensions, and an explicit embedding revision. The key stays in the API and worker environments. For a local model server, set `AI_PROVIDER=lmstudio` and `AI_BASE_URL=http://127.0.0.1:1234/v1`, and configure the embedding/chat models and dimensions to match the models loaded on that server (see `.env.example`). The model IDs and embedding dimensions are required. `CHAT_REVISION` and `EMBEDDING_REVISION` are optional; when omitted, the backend records provider/model-specific unpinned revisions. Set a new embedding revision and reindex sources when changing the embedding provider, model, or input style. `ADMIN_USERNAME` and `ADMIN_PASSWORD` are required for administrator browser sign-in. `API_TOKEN` is optional for bearer API access by scripts and must have at least 24 characters when set. `REVIEWER_TOKEN` and `AUTH_PRINCIPALS_JSON` are optional; reviewer browser sign-in continues to accept reviewer tokens. Administrator sessions expire when the API restarts.

CORE-01 adds `model_calls` for one row per provider attempt. Before deploying this migration, back up PostgreSQL and the PDF asset volume together. Keep the previous image and non-secret model configuration for rollback. The migration is additive; changing the chat provider does not delete sources or citations. An existing embedding index remains tied to its recorded model and revision, so build a compatible READY index before asking against it.

Keep ports 5437, 8082, and 8088 free, or choose three distinct ports in `.env.prod`. These defaults differ from standard PostgreSQL's port so another host database can coexist. This configuration needs host ports above 1023 because it drops all capabilities and runs as non-root. Host networking shares the server's network namespace: omitting `ports` does not hide listeners. PostgreSQL binds to `127.0.0.1:5437`, and the API to `127.0.0.1:8082`; the worker has no listener. See Docker's [host networking and service settings](https://docs.docker.com/reference/compose-file/services/#network_mode).

The frontend defaults to `127.0.0.1:8088`. Put a host reverse proxy with your domain and TLS certificate in front of `http://127.0.0.1:8088`, and allow inbound 80/443 and SSH in the server firewall. To serve the frontend directly instead, set `FRONTEND_BIND_ADDR=0.0.0.0` and allow inbound `FRONTEND_PORT`; this serves HTTP, so use an HTTPS proxy for public production access. Neither the API nor PostgreSQL needs an inbound firewall rule.

If you use the optional starter corpus, upload it separately to `data/starter-corpus` and ensure UID 10001 can read its files and traverse its directories. The deployment script creates an empty directory if none exists. Normal PDF upload/import does not require the starter corpus.

## GitHub configuration

Create a GitHub environment named `production`, and add these secrets and variables there (repository-level settings also work). If you want automatic deployment on every push, avoid configuring required environment reviewers.

| Type | Name | Value |
| --- | --- | --- |
| Secret | `DEPLOY_HOST` | Server hostname or IPv4 address |
| Secret | `DEPLOY_USER` | SSH deployment username |
| Secret or variable | `DEPLOY_PORT` | SSH port; defaults to `22` (secret takes precedence) |
| Variable | `DEPLOY_PATH` | Dedicated absolute app directory, e.g. `/home/deploy/askhomeo` |
| Secret | `DEPLOY_PASSWORD` | SSH deployment user's password |

Password authentication must be enabled on the server. The workflow uses `sshpass` with `DEPLOY_PASSWORD` for both SSH and rsync, disables public-key authentication, and skips SSH host-key verification. No SSH key or known-hosts secret is required.

Push these changes to `main`, then inspect **Actions → Deploy production**. The workflow requires an existing `.env.prod` before syncing. Rsync preserves all `data/` contents, `.env*` files, and `*.env` files, while removing obsolete application files. Containers must be healthy within 180 seconds after starting; the worker is checked for running state because it has no HTTP endpoint. A successful deploy checks the API through the frontend proxy, but does not exercise model calls or document ingestion.

## Operations and persistent data

For a manual deployment or retry on the server:

```sh
cd /home/deploy/askhomeo
bash scripts/deploy-prod.sh
docker compose -f docker-compose.prod.yml --env-file .env.prod logs --tail 100
```

The fixed Compose project name is `askhomeo-prod`. Its named volumes are `askhomeo-prod_pgdata` (PostgreSQL) and `askhomeo-prod_assets` (uploaded PDFs). New volumes initialize with the image's non-root ownership: PostgreSQL UID 999 and app UID 10001. Images and code can be replaced without removing these volumes. Do not run `docker compose down -v` against production. Maintain backups of both volumes and the private environment file before deploying changes that migrate the database.

This production configuration uses separate volumes from the development Compose setup. If the server already has data from that setup, back up and restore the database and copy `data/runtime/assets` into the production asset volume with ownership `10001:10001` before switching traffic. Do not assume an existing development deployment's data will appear automatically. Follow the README's reindex instructions when switching embedding providers.

All services drop every Linux capability, deny new privileges, and have read-only root filesystems. Only named data volumes and temporary filesystems are writable; the starter corpus is read-only. Logs rotate at three 10 MB files per service. Backend temporary filesystems allow up to 1 GB per container for PDF processing, and the frontend allows 512 MB for upload buffering; size the server for concurrent ingestion and uploads. Host networking deliberately provides less network isolation than a private Docker bridge.
