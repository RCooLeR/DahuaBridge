from __future__ import annotations

from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True, slots=True)
class ButtonSpec:
    key: str
    name: str
    url: str
    icon: str


@dataclass(frozen=True, slots=True)
class SwitchSpec:
    key: str
    name: str
    url: str
    icon: str
    value_key: str
    payload_key: str | None = None
    value_source: str = "intercom"
    payload_on: dict[str, Any] | None = None
    payload_off: dict[str, Any] | None = None
