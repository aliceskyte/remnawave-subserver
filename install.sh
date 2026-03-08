#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

log() {
  printf '%s\n' "==> $*"
}

die() {
  printf '%s\n' "ERROR: $*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"
}

detect_compose() {
  if docker compose version >/dev/null 2>&1; then
    echo "docker compose"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    echo "docker-compose"
    return
  fi
  die "docker compose not found (install Docker Compose v2 or docker-compose)"
}

require_cmd docker
COMPOSE_CMD="$(detect_compose)"

ENV_FILE_PATH="${SUBSERVER_ENV_FILE:-/etc/subserver/subserver.env}"
LEGACY_ENV_PATH="$ROOT_DIR/.env"
DATA_DIR="${SUBSERVER_DATA_DIR:-/var/lib/subserver}"
LEGACY_DATA_DIR="$ROOT_DIR/data"
CONFIG_FILE_PATH="${SUBSERVER_CONFIG_FILE:-$ROOT_DIR/configs/default.json}"

if [[ ! -f "$ENV_FILE_PATH" ]]; then
  if [[ -f "$LEGACY_ENV_PATH" ]]; then
    log "Migrating legacy .env to $ENV_FILE_PATH"
    install -D -m 600 "$LEGACY_ENV_PATH" "$ENV_FILE_PATH"
  else
    die "env file not found. Create $ENV_FILE_PATH from .env.example or set SUBSERVER_ENV_FILE."
  fi
fi

chmod 600 "$ENV_FILE_PATH"

if ! grep -qE '^[[:space:]]*PANEL_TOKEN=.*[^[:space:]]' "$ENV_FILE_PATH"; then
  die "PANEL_TOKEN is missing or empty in $ENV_FILE_PATH"
fi
if ! grep -qE '^[[:space:]]*ADMIN_TOKEN=.*[^[:space:]]' "$ENV_FILE_PATH"; then
  die "ADMIN_TOKEN is missing or empty in $ENV_FILE_PATH"
fi

if [[ ! -f "$CONFIG_FILE_PATH" ]]; then
  die "config file not found: $CONFIG_FILE_PATH"
fi

install -d -m 700 "$DATA_DIR"

if [[ "$DATA_DIR" != "$LEGACY_DATA_DIR" && -d "$LEGACY_DATA_DIR" ]]; then
  if [[ ! -e "$DATA_DIR/subserver.db" ]]; then
    log "Migrating legacy data directory to $DATA_DIR"
    cp -a "$LEGACY_DATA_DIR"/. "$DATA_DIR"/
  fi
fi

NETWORK_NAME="remnawave-network"
if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
  log "Creating docker network: $NETWORK_NAME"
  docker network create "$NETWORK_NAME" >/dev/null
fi

SUBSERVER_PORT_RAW="$(grep -E '^[[:space:]]*SUBSERVER_PORT=' "$ENV_FILE_PATH" | tail -n1 | cut -d= -f2- | tr -d '[:space:]' || true)"
if [[ -n "${SUBSERVER_PORT_RAW}" && "${SUBSERVER_PORT_RAW}" != "8080" ]]; then
  log "Warning: SUBSERVER_PORT is ${SUBSERVER_PORT_RAW}, but docker-compose.yml publishes 18080:8080 on localhost"
fi

log "Building and starting subserver..."
$COMPOSE_CMD -f "$ROOT_DIR/docker-compose.yml" up -d --build

log "Done. Admin UI: http://127.0.0.1:18080/admin/"
