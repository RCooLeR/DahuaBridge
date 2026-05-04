# Integration Docs

Read these pages in order when you need to understand or operate the Home Assistant integration:

1. [How it works](architecture.md)
2. [Install](install.md)
3. [Configuration](configuration.md)
4. [Entities and controls](entities-and-controls.md)
5. [Camera recording and archive access](camera-recording.md)
6. [Feature summary](features.md)

The short version: Home Assistant configures one bridge URL, polls the bridge-native catalog, and creates entities from records in that catalog. All live stream, snapshot, recording, playback, and control actions go back through bridge URLs advertised by the catalog.

## Related Docs

- [Project docs](../../docs/README.md)
- [Bridge docs](../../bridge/docs/README.md)
