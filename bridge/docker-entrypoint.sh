#!/bin/sh
set -eu

ensure_device_group() {
    device_path="$1"
    group_name="$2"

    if [ ! -e "$device_path" ]; then
        return 0
    fi

    gid="$(stat -c '%g' "$device_path" 2>/dev/null || true)"
    if [ -z "$gid" ]; then
        return 0
    fi

    existing_group="$(getent group "$gid" | cut -d: -f1 || true)"
    if [ -n "$existing_group" ]; then
        usermod -a -G "$existing_group" dahuabridge >/dev/null 2>&1 || true
        return 0
    fi

    groupadd --gid "$gid" "$group_name" >/dev/null 2>&1 || true
    usermod -a -G "$group_name" dahuabridge >/dev/null 2>&1 || true
}

if [ "$(id -u)" = "0" ]; then
    ensure_device_group /dev/dri/renderD128 render
    ensure_device_group /dev/dri/card0 video
    exec gosu dahuabridge /app/dahuabridge "$@"
fi

exec /app/dahuabridge "$@"
