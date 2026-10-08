#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
if [[ ! -f production.env ]]; then
  echo 'Missing production.env: copy production.env.example and configure credentials on the server.' >&2
  exit 1
fi
chmod 600 production.env
mkdir -p data/starter-corpus

compose=(docker compose -f docker-compose.prod.yml --env-file production.env)
"${compose[@]}" config --quiet
# Complete image builds before replacing any running containers.
"${compose[@]}" build --pull api worker frontend
"${compose[@]}" pull db
"${compose[@]}" up -d --remove-orphans --wait --wait-timeout 180
"${compose[@]}" ps
