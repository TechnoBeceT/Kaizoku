#!/bin/sh
set -e

PUID=${PUID:-0}
PGID=${PGID:-0}

if [ "$PUID" -eq 0 ] && [ "$PGID" -eq 0 ]; then
    exec /app/kaizoku
fi

# Create group if GID doesn't exist
if ! getent group "$PGID" > /dev/null 2>&1; then
    groupadd -g "$PGID" kaizoku
fi
GROUP_NAME=$(getent group "$PGID" | cut -d: -f1)

# Create user if UID doesn't exist
if ! id "$PUID" > /dev/null 2>&1; then
    useradd -u "$PUID" -g "$PGID" -d /app -s /bin/sh -M kaizoku
fi
USER_NAME=$(id -nu "$PUID")

# Ensure config directory is writable
chown "$PUID:$PGID" /config 2>/dev/null || true

exec gosu "$USER_NAME" /app/kaizoku
