#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
if [[ ! -f .env.prod ]]; then
  echo 'Missing .env.prod: copy .env.prod.example and configure credentials on the server.' >&2
  exit 1
fi
if [[ ! -r .env.prod ]]; then
  echo 'Cannot read .env.prod: give the deployment user read access on the server.' >&2
  exit 1
fi
mkdir -p data/starter-corpus

compose=(docker compose -f docker-compose.prod.yml --env-file .env.prod)
"${compose[@]}" config --quiet
# Complete image builds before replacing any running containers.
"${compose[@]}" build --pull api worker frontend
"${compose[@]}" pull db
if ! "${compose[@]}" up -d --remove-orphans --wait --wait-timeout 180; then
  echo 'Production containers did not become healthy. Current status:' >&2
  "${compose[@]}" ps --all >&2 || true
  echo 'Recent API and database logs (credentials redacted):' >&2
  logs="$("${compose[@]}" logs --no-color --tail 40 api db 2>&1 || true)"
  while IFS='=' read -r key value || [[ -n "$key" ]]; do
    case "$key" in
      POSTGRES_PASSWORD|ADMIN_PASSWORD|API_TOKEN|REVIEWER_TOKEN|OPENROUTER_API_KEY|DEEPINFRA_API_KEY|CHAT_API_KEY|EMBEDDING_API_KEY|AUTH_PRINCIPALS_JSON)
        value="${value%$'\r'}"
        value="${value#\"}"
        value="${value%\"}"
        value="${value#\'}"
        value="${value%\'}"
        if [[ -n "$value" ]]; then logs="${logs//"$value"/[REDACTED]}"; fi
        ;;
    esac
  done < .env.prod
  printf '%s\n' "$logs" >&2
  exit 1
fi
"${compose[@]}" ps
