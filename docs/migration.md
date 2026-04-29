# Native Integration Note

This repo now treats the Go bridge plus the native Home Assistant custom integration as the supported path.

Use these defaults:

```yaml
mqtt:
  enabled: false

home_assistant:
  entity_mode: native
```

If you are coming from an older mixed setup, remove duplicate legacy Home Assistant devices manually from Home Assistant after the native integration is working. The bridge no longer exposes dedicated migration helper endpoints for that cleanup.
