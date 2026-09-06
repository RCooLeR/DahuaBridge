"""Real HA fixtures; this suite deliberately never imports ha_stubs."""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))


@pytest.fixture(autouse=True)
def custom_integrations(enable_custom_integrations):
    yield
