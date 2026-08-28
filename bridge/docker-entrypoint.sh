#!/bin/sh
set -eu

supplementary_gids="$(id -G dahuabridge 2>/dev/null | tr ' ' ',')"

add_device_gid() {
    device_path="$1"

    if [ ! -e "$device_path" ]; then
        return 0
    fi

    gid="$(stat -c '%g' "$device_path" 2>/dev/null || true)"
    if [ -z "$gid" ]; then
        return 0
    fi

    case ",$supplementary_gids," in
        *",$gid,"*) ;;
        *) supplementary_gids="$supplementary_gids,$gid" ;;
    esac
}

if [ "$(id -u)" = "0" ]; then
    for device_path in /dev/dri/renderD* /dev/dri/card*; do
        add_device_gid "$device_path"
    done

    exec setpriv \
        --reuid=dahuabridge \
        --regid=dahuabridge \
        --groups="$supplementary_gids" \
        --no-new-privs \
        /app/dahuabridge "$@"
fi

exec /app/dahuabridge "$@"
