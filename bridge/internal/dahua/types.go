package dahua

import (
	"context"
	"errors"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

var ErrDeviceNotFound = errors.New("device not found")

type DeviceKind string

const (
	DeviceKindNVR        DeviceKind = "nvr"
	DeviceKindVTO        DeviceKind = "vto"
	DeviceKindIPC        DeviceKind = "ipc"
	DeviceKindNVRChannel DeviceKind = "nvr_channel"
	DeviceKindNVRDisk    DeviceKind = "nvr_disk"
	DeviceKindVTOLock    DeviceKind = "vto_lock"
	DeviceKindVTOAlarm   DeviceKind = "vto_alarm"
)

type Device struct {
	ID           string            `json:"id"`
	ParentID     string            `json:"parent_id,omitempty"`
	Name         string            `json:"name"`
	Manufacturer string            `json:"manufacturer"`
	Model        string            `json:"model,omitempty"`
	Serial       string            `json:"serial,omitempty"`
	Firmware     string            `json:"firmware,omitempty"`
	BuildDate    string            `json:"build_date,omitempty"`
	BaseURL      string            `json:"base_url,omitempty"`
	Kind         DeviceKind        `json:"kind"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}

type ProbeResult struct {
	Root     Device                 `json:"root"`
	Children []Device               `json:"children,omitempty"`
	States   map[string]DeviceState `json:"states,omitempty"`
	Raw      map[string]string      `json:"raw,omitempty"`
}

type DeviceState struct {
	Available bool           `json:"available"`
	Info      map[string]any `json:"info,omitempty"`
}

type EventAction string

const (
	EventActionStart EventAction = "start"
	EventActionStop  EventAction = "stop"
	EventActionPulse EventAction = "pulse"
	EventActionState EventAction = "state"
)

type Event struct {
	DeviceID   string            `json:"device_id"`
	DeviceKind DeviceKind        `json:"device_kind"`
	ChildID    string            `json:"child_id,omitempty"`
	Code       string            `json:"code"`
	Action     EventAction       `json:"action"`
	Index      int               `json:"index,omitempty"`
	Channel    int               `json:"channel,omitempty"`
	OccurredAt time.Time         `json:"occurred_at"`
	Data       map[string]string `json:"data,omitempty"`
}

type ProbeActionResult struct {
	DeviceID   string       `json:"device_id"`
	DeviceKind DeviceKind   `json:"device_kind"`
	Result     *ProbeResult `json:"result,omitempty"`
	Error      string       `json:"error,omitempty"`
}

type DeviceConfigUpdate struct {
	BaseURL         *string `json:"base_url,omitempty"`
	Username        *string `json:"username,omitempty"`
	Password        *string `json:"password,omitempty"`
	OnvifEnabled    *bool   `json:"onvif_enabled,omitempty"`
	OnvifUsername   *string `json:"onvif_username,omitempty"`
	OnvifPassword   *string `json:"onvif_password,omitempty"`
	OnvifServiceURL *string `json:"onvif_service_url,omitempty"`
	InsecureSkipTLS *bool   `json:"insecure_skip_tls,omitempty"`
}

type Driver interface {
	ID() string
	Kind() DeviceKind
	PollInterval() time.Duration
	Probe(context.Context) (*ProbeResult, error)
}

type SnapshotProvider interface {
	Snapshot(context.Context, int) ([]byte, string, error)
}

type EventSource interface {
	StreamEvents(context.Context, chan<- Event) error
}

type VTOLockController interface {
	Unlock(context.Context, int) error
}

type VTOCallController interface {
	AnswerCall(context.Context) error
	HangupCall(context.Context) error
}

type NVRInventoryRefresher interface {
	InvalidateInventoryCache()
}

type ConfigurableDriver interface {
	UpdateConfig(config.DeviceConfig) error
}
