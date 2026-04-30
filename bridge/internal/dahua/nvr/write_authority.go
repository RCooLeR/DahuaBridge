package nvr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/dahua"
)

const nvrConfigWriteCacheTTL = 15 * time.Minute

type channelWriteStatus struct {
	Checked time.Time
	Allowed bool
	Reason  string
}

func (d *Driver) nvrConfigWriteStatus(ctx context.Context) (bool, string) {
	d.configWriteMu.RLock()
	if d.configWriteKnown && time.Since(d.configWriteChecked) < nvrConfigWriteCacheTTL {
		allowed := d.configWriteAllowed
		reason := d.configWriteReason
		d.configWriteMu.RUnlock()
		return allowed, reason
	}
	d.configWriteMu.RUnlock()

	allowed, reason := d.probeNVRConfigWriteStatus(ctx)

	d.configWriteMu.Lock()
	d.configWriteChecked = time.Now()
	d.configWriteKnown = true
	d.configWriteAllowed = allowed
	d.configWriteReason = reason
	d.configWriteMu.Unlock()
	return allowed, reason
}

func (d *Driver) probeNVRConfigWriteStatus(ctx context.Context) (bool, string) {
	if d.client == nil {
		return false, "client_unavailable"
	}
	recordModes, err := d.loadRecordModes(ctx)
	if err != nil {
		return false, "probe_failed"
	}

	cfg := d.currentConfig()
	for zeroBasedChannel := range recordModes {
		channelNumber := zeroBasedChannel + 1
		if !cfg.AllowsChannel(channelNumber) {
			continue
		}
		if !cfg.AllowConfigWrites {
			return false, "disabled_by_config"
		}
		return true, "enabled_by_config"
	}

	return false, "record_mode_unavailable"
}

func isAuthorityDeniedConfigError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "403 forbidden") ||
		strings.Contains(message, "authority:check failure") ||
		strings.Contains(message, "permission denied")
}

func (d *Driver) resetConfigWriteStatus() {
	d.configWriteMu.Lock()
	d.configWriteChecked = time.Time{}
	d.configWriteKnown = false
	d.configWriteAllowed = false
	d.configWriteReason = ""
	d.configWriteMu.Unlock()

	d.audioWriteMu.Lock()
	d.audioWriteStatus = nil
	d.audioWriteMu.Unlock()
}

func (d *Driver) requireNVRConfigWrite(ctx context.Context, channel int, operation string) error {
	allowed, reason := d.nvrConfigWriteStatus(ctx)
	if allowed {
		return nil
	}
	switch strings.TrimSpace(reason) {
	case "permission_denied":
		return fmt.Errorf("%w: %s requires nvr config-write permission on channel %d", dahua.ErrUnsupportedOperation, operation, channel)
	case "disabled_by_config":
		return fmt.Errorf("%w: %s requires allow_config_writes=true on channel %d", dahua.ErrUnsupportedOperation, operation, channel)
	default:
		return fmt.Errorf("%w: %s requires an nvr config-write surface on channel %d", dahua.ErrUnsupportedOperation, operation, channel)
	}
}
