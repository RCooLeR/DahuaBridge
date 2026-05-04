from __future__ import annotations

import logging
from collections.abc import Mapping
from datetime import datetime, timezone
from typing import Any

from homeassistant.core import HomeAssistant
from homeassistant.util import dt as dt_util

_LOGGER = logging.getLogger(__name__)

TIMEFRAME_LAYOUT = "%Y_%m_%d_%H_%M_%S"


def parse_query_datetime(
    hass: HomeAssistant,
    raw: str | None,
    field: str,
) -> datetime | None:
    if raw is None:
        return None

    value = raw.strip().lstrip("=")
    if not value:
        return None

    try:
        return datetime.strptime(value, TIMEFRAME_LAYOUT)
    except ValueError:
        parsed = dt_util.parse_datetime(value)
        if parsed is None:
            _LOGGER.debug("Invalid DahuaBridge timeframe %s value %r", field, raw)
            return None

    if parsed.tzinfo is None:
        parsed = with_timezone(parsed, local_time_zone(hass))
    return parsed


def datetime_for_compare(hass: HomeAssistant, value: datetime) -> datetime:
    if value.tzinfo is not None:
        return value
    return with_timezone(value, local_time_zone(hass))


def bridge_playback_datetime(value: datetime) -> str:
    if value.tzinfo is None:
        return value.strftime("%Y-%m-%d %H:%M:%S")
    return value.astimezone(timezone.utc).isoformat()


def local_time_zone(hass: HomeAssistant) -> Any:
    time_zone = getattr(getattr(hass, "config", None), "time_zone", None)
    if time_zone:
        zone = dt_util.get_time_zone(time_zone)
        if zone is not None:
            return zone
    return dt_util.DEFAULT_TIME_ZONE


def with_timezone(value: datetime, zone: Any) -> datetime:
    localize = getattr(zone, "localize", None)
    if callable(localize):
        return localize(value)
    return value.replace(tzinfo=zone)


def query_value(query: Mapping[str, str], *names: str) -> str | None:
    for name in names:
        value = query.get(name)
        if value:
            return value
    return None


def positive_int(value: Any) -> int | None:
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        return None
    return parsed if parsed > 0 else None
