#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
if [[ ! -f .env.prod ]]; then
  echo 'Missing .env.prod: copy .env.prod.example and configure credentials on the server.' >&2
  exit 1
fi
chmod 600 .env.prod
mkdir -p data/starter-corpus

compose=(docker compose -f docker-compose.prod.yml --env-file .env.prod)
"${compose[@]}" config --quiet
# Complete image builds before replacing any running containers.
"${compose[@]}" build --pull api worker frontend
"${compose[@]}" pull db
"${compose[@]}" up -d --remove-orphans --wait --wait-timeout 180
"${compose[@]}" ps
