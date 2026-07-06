#!/bin/sh
# Runs LibriNode as the PUID/PGID user (linuxserver-style), fixing /config
# ownership so bind mounts created by root are writable.
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ]; then
    if [ "$(id -u librinode)" != "$PUID" ] || [ "$(id -g librinode)" != "$PGID" ]; then
        deluser librinode 2>/dev/null || true
        addgroup -g "$PGID" librinode 2>/dev/null || true
        adduser -u "$PUID" -G librinode -D -H librinode 2>/dev/null || true
    fi
    mkdir -p /config
    chown -R "$PUID:$PGID" /config
    exec su-exec "$PUID:$PGID" /usr/local/bin/librinode "$@"
fi

exec /usr/local/bin/librinode "$@"
