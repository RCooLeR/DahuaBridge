# Home Assistant compatibility tests

This suite runs against real Home Assistant, aiohttp, Voluptuous, config flows,
entity platforms, and registries. It never imports the lightweight `ha_stubs`
used by `integration/tests`.

On Linux with Python 3.14, from the repository root:

```sh
python -m pip install -r integration/requirements-test-ha.txt
python -m pytest -c integration/pytest-ha.ini integration/tests_real_ha
```

The pinned test package selects Home Assistant 2026.9.1. Camera and stream
requirements are pinned separately because HA normally installs platform
dependencies dynamically, while the test fixtures disable that installation.

Tests cover setup and unload of every integration platform, recorder/camera
parent registration, real options schema defaults, failure preservation, old
bridge compatibility, protected setup validation, and authenticated HTTP request
chains. HTTP servers in the tests listen locally and use synthetic credentials.
