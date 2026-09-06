# DahuaBridge

DahuaBridge is a Dahua-focused bridge for Home Assistant.

It has three parts:

- `bridge/`: the Go service that talks to Dahua devices, owns the runtime model, exposes HTTP APIs, and serves bridge-hosted media
- `integration/`: the Home Assistant custom integration that consumes the bridge catalog and creates Home Assistant entities
- `ha-cards/`: optional custom cards for higher-level dashboard UX

Most users need:

1. the Go bridge from `bridge/`
2. the Home Assistant custom integration from `integration/custom_components/dahuabridge`

Live video can prefer the NVR or each camera directly. The bridge saves a default
preference and optional camera overrides, monitors the upstream streams, and
switches to a working alternate when the preferred source fails. Home Assistant
keeps the same bridge RTSP relay URL while the bridge selects the upstream;
the HLS, DASH, MJPEG, and WebRTC output choices remain independent. Recorded
playback continues to use the NVR. See [live media routing](bridge/docs/media-and-recording.md).

## Documentation

- System overview: [docs/README.md](docs/README.md)
- Bridge docs: [bridge/docs/README.md](bridge/docs/README.md)
- Home Assistant integration docs: [integration/docs/README.md](integration/docs/README.md)
- HA cards docs: [ha-cards/docs/README.md](ha-cards/docs/README.md)

## Recommended Path

1. Read the system docs in [docs/deployment.md](docs/deployment.md).
2. Set up the bridge with [bridge/docs/getting-started.md](bridge/docs/getting-started.md).
3. Install the Home Assistant integration with [integration/docs/install.md](integration/docs/install.md).

## CI

GitHub Actions runs on every branch push, every pull request, and manual
dispatch. The workflow checks:

- Go tests, race checks, real FFmpeg media tests, and Docker image build for `bridge/`
- Python compile, fast unit tests, and isolated real Home Assistant lifecycle tests for `integration/`
- Typecheck, lifecycle tests, browser smoke, and a size-budgeted production build for `ha-cards/`

Run the same checks locally before pushing when possible:

```bash
(cd bridge && go test ./...)
python -m compileall integration/custom_components/dahuabridge integration/tests
python -m unittest discover -s integration/tests
(cd ha-cards && npm ci && npm run lint && npm run test && npm run build)
```

The real HA suite uses Python 3.14 with
`integration/requirements-test-ha.txt` and runs separately with
`python -m pytest -c integration/pytest-ha.ini integration/tests_real_ha`.
FFmpeg integration tests run when `ffmpeg` and `ffprobe` are installed.
Run browser smoke after building with `npm run test:browser -- /path/to/chrome`
from `ha-cards/`. Deploy the complete `ha-cards/dist/` directory, including its
lazy chunks and assets.

## Repository Layout

- [bridge/README.md](bridge/README.md)
- [integration/README.md](integration/README.md)
- [ha-cards/README.md](ha-cards/README.md)

The primary deployment path is still bridge plus Home Assistant integration, but the repository also contains an optional Lovelace card workspace with its own build and install path.
