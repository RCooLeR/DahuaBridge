package ha

import (
	"context"
	"fmt"
	"strings"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/dahua"
	"RCooLeR/DahuaBridge/internal/mqtt"
	"github.com/rs/zerolog"
)

type DiscoveryPublisher struct {
	cfg    config.Config
	mqtt   mqtt.Client
	logger zerolog.Logger
}

type LegacyDiscoveryCleanupResult struct {
	RemovedTopics int      `json:"removed_topics"`
	DeviceCount   int      `json:"device_count"`
	DeviceIDs     []string `json:"device_ids,omitempty"`
}

type devicePayload struct {
	Identifiers  []string `json:"identifiers"`
	Name         string   `json:"name"`
	Manufacturer string   `json:"manufacturer,omitempty"`
	Model        string   `json:"model,omitempty"`
	SWVersion    string   `json:"sw_version,omitempty"`
	ViaDevice    string   `json:"via_device,omitempty"`
}

type entityConfig struct {
	Name              string        `json:"name"`
	UniqueID          string        `json:"unique_id"`
	StateTopic        string        `json:"state_topic"`
	CommandTopic      string        `json:"command_topic,omitempty"`
	ValueTemplate     string        `json:"value_template,omitempty"`
	Min               *float64      `json:"min,omitempty"`
	Max               *float64      `json:"max,omitempty"`
	Step              *float64      `json:"step,omitempty"`
	Mode              string        `json:"mode,omitempty"`
	PayloadOn         string        `json:"payload_on,omitempty"`
	PayloadOff        string        `json:"payload_off,omitempty"`
	PayloadPress      string        `json:"payload_press,omitempty"`
	DeviceClass       string        `json:"device_class,omitempty"`
	Icon              string        `json:"icon,omitempty"`
	EntityCategory    string        `json:"entity_category,omitempty"`
	UnitOfMeasurement string        `json:"unit_of_measurement,omitempty"`
	StateClass        string        `json:"state_class,omitempty"`
	ObjectID          string        `json:"object_id,omitempty"`
	DefaultEntityID   string        `json:"default_entity_id,omitempty"`
	AvailabilityTopic string        `json:"availability_topic,omitempty"`
	AvailabilityMode  string        `json:"availability_mode,omitempty"`
	EventTypes        []string      `json:"event_types,omitempty"`
	Device            devicePayload `json:"device"`
}

type cameraConfig struct {
	Name              string        `json:"name,omitempty"`
	UniqueID          string        `json:"unique_id"`
	Topic             string        `json:"topic"`
	ObjectID          string        `json:"object_id,omitempty"`
	DefaultEntityID   string        `json:"default_entity_id,omitempty"`
	AvailabilityTopic string        `json:"availability_topic,omitempty"`
	AvailabilityMode  string        `json:"availability_mode,omitempty"`
	Device            devicePayload `json:"device"`
}

type deviceInfoState struct {
	Name      string            `json:"name"`
	Firmware  string            `json:"firmware,omitempty"`
	BuildDate string            `json:"build_date,omitempty"`
	Serial    string            `json:"serial,omitempty"`
	Model     string            `json:"model,omitempty"`
	Kind      dahua.DeviceKind  `json:"kind"`
	Info      map[string]any    `json:"info,omitempty"`
	Raw       map[string]string `json:"raw,omitempty"`
}

type deviceTriggerConfig struct {
	AutomationType string        `json:"automation_type"`
	Topic          string        `json:"topic"`
	Payload        string        `json:"payload,omitempty"`
	ValueTemplate  string        `json:"value_template,omitempty"`
	Type           string        `json:"type"`
	Subtype        string        `json:"subtype"`
	QoS            byte          `json:"qos,omitempty"`
	Platform       string        `json:"platform"`
	Device         devicePayload `json:"device"`
}

func NewDiscoveryPublisher(cfg config.Config, client mqtt.Client, logger zerolog.Logger) *DiscoveryPublisher {
	return &DiscoveryPublisher{
		cfg:    cfg,
		mqtt:   client,
		logger: logger.With().Str("component", "ha_discovery").Logger(),
	}
}

func (p *DiscoveryPublisher) PublishProbe(ctx context.Context, result *dahua.ProbeResult) error {
	if !p.cfg.HomeAssistant.Enabled || result == nil {
		return nil
	}

	devices := append([]dahua.Device{result.Root}, result.Children...)
	for _, device := range devices {
		state := result.States[device.ID]
		availabilityTopic := p.deviceTopic(device.ID, "availability")
		infoTopic := p.deviceTopic(device.ID, "info")

		onlineConfig := entityConfig{
			Name:              "Online",
			UniqueID:          fmt.Sprintf("%s_%s_online", p.cfg.HomeAssistant.NodeID, device.ID),
			StateTopic:        availabilityTopic,
			PayloadOn:         "online",
			PayloadOff:        "offline",
			DeviceClass:       "connectivity",
			EntityCategory:    "diagnostic",
			ObjectID:          fmt.Sprintf("%s_online", device.ID),
			AvailabilityTopic: availabilityTopic,
			AvailabilityMode:  "latest",
			Device:            toDevicePayload(device),
		}

		metadataConfig := entityConfig{
			Name:              "Firmware",
			UniqueID:          fmt.Sprintf("%s_%s_firmware", p.cfg.HomeAssistant.NodeID, device.ID),
			StateTopic:        infoTopic,
			ValueTemplate:     "{{ value_json.firmware }}",
			EntityCategory:    "diagnostic",
			ObjectID:          fmt.Sprintf("%s_firmware", device.ID),
			AvailabilityTopic: availabilityTopic,
			AvailabilityMode:  "latest",
			Device:            toDevicePayload(device),
		}

		if p.haDiscoveryEnabled() {
			if err := p.publishDiscovery(ctx, "binary_sensor", device.ID, "online", onlineConfig); err != nil {
				return err
			}
			if err := p.publishDiscovery(ctx, "sensor", device.ID, "firmware", metadataConfig); err != nil {
				return err
			}
			for _, extra := range p.extraEntityConfigs(device, state, availabilityTopic, infoTopic) {
				if err := p.publishDiscovery(ctx, extra.component, device.ID, extra.objectID, extra.config); err != nil {
					return err
				}
			}
			for _, trigger := range p.extraTriggerConfigs(device) {
				if err := p.publishDeviceTrigger(ctx, device.ID, trigger.objectID, trigger.config); err != nil {
					return err
				}
			}
		}

		availability := "offline"
		if state.Available || device.ID == result.Root.ID {
			availability = "online"
		}
		if err := p.mqtt.Publish(ctx, availabilityTopic, p.cfg.MQTT.QoS, true, []byte(availability)); err != nil {
			return err
		}

		if err := p.mqtt.PublishJSON(ctx, infoTopic, p.cfg.MQTT.QoS, true, deviceInfoState{
			Name:      device.Name,
			Firmware:  device.Firmware,
			BuildDate: device.BuildDate,
			Serial:    device.Serial,
			Model:     device.Model,
			Kind:      device.Kind,
			Info:      state.Info,
			Raw:       result.Raw,
		}); err != nil {
			return err
		}
		if err := p.publishEventDerivedStates(ctx, device, state); err != nil {
			return err
		}
	}

	if p.haDiscoveryEnabled() {
		for _, camera := range p.cameraConfigs(result) {
			if err := p.publishCameraDiscovery(ctx, camera.deviceID, camera.objectID, camera.config); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *DiscoveryPublisher) PublishUnavailable(ctx context.Context, deviceID string) error {
	return p.mqtt.Publish(ctx, p.deviceTopic(deviceID, "availability"), p.cfg.MQTT.QoS, true, []byte("offline"))
}

func (p *DiscoveryPublisher) PublishBinaryState(ctx context.Context, deviceID string, field string, value bool) error {
	payload := []byte("OFF")
	if value {
		payload = []byte("ON")
	}
	return p.mqtt.Publish(ctx, p.stateTopic(deviceID, field), p.cfg.MQTT.QoS, false, payload)
}

func (p *DiscoveryPublisher) PublishState(ctx context.Context, deviceID string, field string, value string, retain bool) error {
	return p.mqtt.Publish(ctx, p.stateTopic(deviceID, field), p.cfg.MQTT.QoS, retain, []byte(value))
}

func (p *DiscoveryPublisher) PublishEvent(ctx context.Context, deviceID string, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = make(map[string]any)
	}
	payload["event_type"] = eventType
	return p.mqtt.PublishJSON(ctx, p.eventTopic(deviceID, "activity"), p.cfg.MQTT.QoS, false, payload)
}

func (p *DiscoveryPublisher) PublishCameraSnapshot(ctx context.Context, deviceID string, payload []byte) error {
	if len(payload) == 0 {
		return nil
	}
	return p.mqtt.Publish(ctx, p.cameraTopic(deviceID), p.cfg.MQTT.QoS, false, payload)
}

func (p *DiscoveryPublisher) RemoveProbeDiscovery(ctx context.Context, result *dahua.ProbeResult) (LegacyDiscoveryCleanupResult, error) {
	if result == nil {
		return LegacyDiscoveryCleanupResult{}, nil
	}

	devices := append([]dahua.Device{result.Root}, result.Children...)
	deviceIDs := make([]string, 0, len(devices))
	removedTopics := 0

	for _, device := range devices {
		deviceIDs = append(deviceIDs, device.ID)

		if err := p.clearDiscoveryTopic(ctx, p.discoveryTopic("binary_sensor", device.ID, "online")); err != nil {
			return LegacyDiscoveryCleanupResult{}, err
		}
		removedTopics++

		if err := p.clearDiscoveryTopic(ctx, p.discoveryTopic("sensor", device.ID, "firmware")); err != nil {
			return LegacyDiscoveryCleanupResult{}, err
		}
		removedTopics++

		availabilityTopic := p.deviceTopic(device.ID, "availability")
		infoTopic := p.deviceTopic(device.ID, "info")
		state := result.States[device.ID]
		for _, extra := range p.extraEntityConfigs(device, state, availabilityTopic, infoTopic) {
			if err := p.clearDiscoveryTopic(ctx, p.discoveryTopic(extra.component, device.ID, extra.objectID)); err != nil {
				return LegacyDiscoveryCleanupResult{}, err
			}
			removedTopics++
		}
		for _, trigger := range p.extraTriggerConfigs(device) {
			if err := p.clearDiscoveryTopic(ctx, p.deviceTriggerTopic(device.ID, trigger.objectID)); err != nil {
				return LegacyDiscoveryCleanupResult{}, err
			}
			removedTopics++
		}
	}

	for _, camera := range p.cameraConfigs(result) {
		if err := p.clearDiscoveryTopic(ctx, p.discoveryTopic("camera", camera.deviceID, camera.objectID)); err != nil {
			return LegacyDiscoveryCleanupResult{}, err
		}
		removedTopics++
	}

	return LegacyDiscoveryCleanupResult{
		RemovedTopics: removedTopics,
		DeviceCount:   len(devices),
		DeviceIDs:     deviceIDs,
	}, nil
}

func (p *DiscoveryPublisher) publishDiscovery(ctx context.Context, component string, deviceID string, objectID string, payload any) error {
	topic := p.discoveryTopic(component, deviceID, objectID)
	p.logger.Debug().Str("topic", topic).Msg("publishing home assistant discovery")
	payload = applyDefaultEntityID(component, deviceID, objectID, payload)
	return p.mqtt.PublishJSON(ctx, topic, p.cfg.MQTT.QoS, p.cfg.MQTT.Retain, payload)
}

func (p *DiscoveryPublisher) publishCameraDiscovery(ctx context.Context, deviceID string, objectID string, payload any) error {
	topic := p.discoveryTopic("camera", deviceID, objectID)
	p.logger.Debug().Str("topic", topic).Msg("publishing home assistant camera discovery")
	payload = applyDefaultEntityID("camera", deviceID, objectID, payload)
	return p.mqtt.PublishJSON(ctx, topic, p.cfg.MQTT.QoS, p.cfg.MQTT.Retain, payload)
}

func (p *DiscoveryPublisher) deviceTopic(deviceID string, suffix string) string {
	return fmt.Sprintf("%s/devices/%s/%s", p.cfg.MQTT.TopicPrefix, deviceID, suffix)
}

func (p *DiscoveryPublisher) discoveryTopic(component string, deviceID string, objectID string) string {
	return fmt.Sprintf(
		"%s/%s/%s/%s/config",
		p.cfg.MQTT.DiscoveryPrefix,
		component,
		p.cfg.HomeAssistant.NodeID,
		fmt.Sprintf("%s_%s", deviceID, objectID),
	)
}

func (p *DiscoveryPublisher) deviceTriggerTopic(deviceID string, objectID string) string {
	return fmt.Sprintf(
		"%s/device_automation/%s/%s_%s/config",
		p.cfg.MQTT.DiscoveryPrefix,
		p.cfg.HomeAssistant.NodeID,
		deviceID,
		objectID,
	)
}

func (p *DiscoveryPublisher) clearDiscoveryTopic(ctx context.Context, topic string) error {
	p.logger.Debug().Str("topic", topic).Msg("clearing home assistant discovery topic")
	return p.mqtt.Publish(ctx, topic, p.cfg.MQTT.QoS, true, []byte{})
}

func (p *DiscoveryPublisher) haDiscoveryEnabled() bool {
	return p.cfg.HomeAssistant.Enabled && !p.cfg.HomeAssistant.NativeEntityMode()
}

func (p *DiscoveryPublisher) stateTopic(deviceID string, field string) string {
	return fmt.Sprintf("%s/devices/%s/state/%s", p.cfg.MQTT.TopicPrefix, deviceID, field)
}

func (p *DiscoveryPublisher) eventTopic(deviceID string, field string) string {
	return fmt.Sprintf("%s/devices/%s/event/%s", p.cfg.MQTT.TopicPrefix, deviceID, field)
}

func (p *DiscoveryPublisher) cameraTopic(deviceID string) string {
	return fmt.Sprintf("%s/devices/%s/camera/snapshot", p.cfg.MQTT.TopicPrefix, deviceID)
}

func (p *DiscoveryPublisher) commandTopic(deviceID string, field string) string {
	return fmt.Sprintf("%s/devices/%s/command/%s", p.cfg.MQTT.TopicPrefix, deviceID, field)
}

func toDevicePayload(device dahua.Device) devicePayload {
	payload := devicePayload{
		Identifiers:  []string{device.ID},
		Name:         device.Name,
		Manufacturer: device.Manufacturer,
		Model:        device.Model,
		SWVersion:    device.Firmware,
	}
	if device.ParentID != "" {
		payload.ViaDevice = device.ParentID
	}
	return payload
}

func (p *DiscoveryPublisher) publishEventDerivedStates(ctx context.Context, device dahua.Device, state dahua.DeviceState) error {
	if device.Kind != dahua.DeviceKindVTO || len(state.Info) == 0 {
		return nil
	}

	fields := []string{
		"call_state",
		"last_ring_at",
		"last_call_started_at",
		"last_call_ended_at",
		"last_call_duration_seconds",
		"last_call_source",
	}

	for _, field := range fields {
		value, ok := stateTopicValue(state.Info[field])
		if !ok {
			continue
		}
		if err := p.PublishState(ctx, device.ID, field, value, true); err != nil {
			return err
		}
	}

	return nil
}

func stateTopicValue(value any) (string, bool) {
	switch typed := value.(type) {
	case nil:
		return "", false
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return "", false
		}
		return trimmed, true
	default:
		return strings.TrimSpace(fmt.Sprint(typed)), true
	}
}

type discoveredEntity struct {
	component string
	objectID  string
	config    entityConfig
}

func removeDiscoveredEntity(entities []discoveredEntity, objectID string) []discoveredEntity {
	filtered := entities[:0]
	for _, entity := range entities {
		if entity.objectID == objectID {
			continue
		}
		filtered = append(filtered, entity)
	}
	return filtered
}

type discoveredTrigger struct {
	objectID string
	config   deviceTriggerConfig
}

type discoveredCamera struct {
	deviceID string
	objectID string
	config   cameraConfig
}

func (p *DiscoveryPublisher) cameraConfigs(result *dahua.ProbeResult) []discoveredCamera {
	if result == nil {
		return nil
	}

	switch result.Root.Kind {
	case dahua.DeviceKindNVR:
		cameras := make([]discoveredCamera, 0, len(result.Children))
		for _, child := range result.Children {
			if child.Kind != dahua.DeviceKindNVRChannel {
				continue
			}
			cameras = append(cameras, p.cameraConfigForDevice(child))
		}
		return cameras
	case dahua.DeviceKindVTO, dahua.DeviceKindIPC:
		return []discoveredCamera{p.cameraConfigForDevice(result.Root)}
	default:
		return nil
	}
}

func (p *DiscoveryPublisher) cameraConfigForDevice(device dahua.Device) discoveredCamera {
	return discoveredCamera{
		deviceID: device.ID,
		objectID: "camera",
		config: cameraConfig{
			Name:              "Camera",
			UniqueID:          fmt.Sprintf("%s_%s_camera", p.cfg.HomeAssistant.NodeID, device.ID),
			Topic:             p.cameraTopic(device.ID),
			ObjectID:          fmt.Sprintf("%s_camera", device.ID),
			AvailabilityTopic: p.deviceTopic(device.ID, "availability"),
			AvailabilityMode:  "latest",
			Device:            toDevicePayload(device),
		},
	}
}

func applyDefaultEntityID(component string, deviceID string, objectID string, payload any) any {
	defaultEntityID := defaultEntityIDFor(component, deviceID, objectID)

	switch typed := payload.(type) {
	case entityConfig:
		if typed.DefaultEntityID == "" {
			typed.DefaultEntityID = defaultEntityID
		}
		return typed
	case cameraConfig:
		if typed.DefaultEntityID == "" {
			typed.DefaultEntityID = defaultEntityID
		}
		return typed
	default:
		return payload
	}
}

func defaultEntityIDFor(component string, deviceID string, objectID string) string {
	return fmt.Sprintf("%s.%s", component, sanitizeEntityID(fmt.Sprintf("%s_%s", deviceID, objectID)))
}

func sanitizeEntityID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "dahuabridge"
	}

	var builder strings.Builder
	builder.Grow(len(value))
	lastUnderscore := false

	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				builder.WriteByte('_')
				lastUnderscore = true
			}
		}
	}

	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "dahuabridge"
	}
	return result
}

func (p *DiscoveryPublisher) extraEntityConfigs(device dahua.Device, state dahua.DeviceState, availabilityTopic string, infoTopic string) []discoveredEntity {
	switch device.Kind {
	case dahua.DeviceKindNVR:
		return []discoveredEntity{
			{
				component: "sensor",
				objectID:  "channel_count",
				config: entityConfig{
					Name:              "Channel Count",
					UniqueID:          fmt.Sprintf("%s_%s_channel_count", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.channel_count }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_channel_count", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:cctv",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "disk_count",
				config: entityConfig{
					Name:              "Disk Count",
					UniqueID:          fmt.Sprintf("%s_%s_disk_count", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.disk_count }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_disk_count", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:harddisk",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "disk_fault",
				config: entityConfig{
					Name:              "Disk Fault",
					UniqueID:          fmt.Sprintf("%s_%s_disk_fault", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.disk_fault else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					DeviceClass:       "problem",
					ObjectID:          fmt.Sprintf("%s_disk_fault", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:harddisk-remove",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "disk_error_count",
				config: entityConfig{
					Name:              "Disk Error Count",
					UniqueID:          fmt.Sprintf("%s_%s_disk_error_count", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.disk_error_count }}",
					EntityCategory:    "diagnostic",
					StateClass:        "measurement",
					ObjectID:          fmt.Sprintf("%s_disk_error_count", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:alert-circle",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "total_bytes",
				config: entityConfig{
					Name:              "Total Storage Bytes",
					UniqueID:          fmt.Sprintf("%s_%s_total_bytes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.total_bytes }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "B",
					StateClass:        "measurement",
					ObjectID:          fmt.Sprintf("%s_total_bytes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "data_size",
					Icon:              "mdi:database",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "used_bytes",
				config: entityConfig{
					Name:              "Used Storage Bytes",
					UniqueID:          fmt.Sprintf("%s_%s_used_bytes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.used_bytes }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "B",
					StateClass:        "measurement",
					ObjectID:          fmt.Sprintf("%s_used_bytes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "data_size",
					Icon:              "mdi:database-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "free_bytes",
				config: entityConfig{
					Name:              "Free Storage Bytes",
					UniqueID:          fmt.Sprintf("%s_%s_free_bytes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.free_bytes }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "B",
					StateClass:        "measurement",
					ObjectID:          fmt.Sprintf("%s_free_bytes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "data_size",
					Icon:              "mdi:database-arrow-right",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "used_percent",
				config: entityConfig{
					Name:              "Storage Used Percent",
					UniqueID:          fmt.Sprintf("%s_%s_used_percent", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.used_percent }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "%",
					StateClass:        "measurement",
					ObjectID:          fmt.Sprintf("%s_used_percent", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:chart-donut",
					Device:            toDevicePayload(device),
				},
			},
		}
	case dahua.DeviceKindNVRChannel:
		entities := []discoveredEntity{
			{
				component: "sensor",
				objectID:  "channel_number",
				config: entityConfig{
					Name:              "Channel Number",
					UniqueID:          fmt.Sprintf("%s_%s_channel_number", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.channel }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_channel_number", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:numeric",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "main_resolution",
				config: entityConfig{
					Name:              "Main Resolution",
					UniqueID:          fmt.Sprintf("%s_%s_main_resolution", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.main_resolution }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_main_resolution", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:video-high-definition",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "sub_resolution",
				config: entityConfig{
					Name:              "Sub Resolution",
					UniqueID:          fmt.Sprintf("%s_%s_sub_resolution", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.sub_resolution }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_sub_resolution", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:video-input-component",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "motion",
				config: entityConfig{
					Name:              "Motion",
					UniqueID:          fmt.Sprintf("%s_%s_motion", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "motion"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					DeviceClass:       "motion",
					ObjectID:          fmt.Sprintf("%s_motion", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "human",
				config: entityConfig{
					Name:              "Human",
					UniqueID:          fmt.Sprintf("%s_%s_human", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "human"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_human", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:account",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "vehicle",
				config: entityConfig{
					Name:              "Vehicle",
					UniqueID:          fmt.Sprintf("%s_%s_vehicle", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "vehicle"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_vehicle", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:car",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "tripwire",
				config: entityConfig{
					Name:              "Tripwire",
					UniqueID:          fmt.Sprintf("%s_%s_tripwire", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "tripwire"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_tripwire", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:vector-line",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "intrusion",
				config: entityConfig{
					Name:              "Intrusion",
					UniqueID:          fmt.Sprintf("%s_%s_intrusion", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "intrusion"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_intrusion", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:shield-alert",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "tamper",
				config: entityConfig{
					Name:              "Tamper",
					UniqueID:          fmt.Sprintf("%s_%s_tamper", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "tamper"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_tamper", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "tamper",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "event",
				objectID:  "activity",
				config: entityConfig{
					Name:              "Activity",
					UniqueID:          fmt.Sprintf("%s_%s_activity", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.eventTopic(device.ID, "activity"),
					ObjectID:          fmt.Sprintf("%s_activity", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					EventTypes: []string{
						"videomotion_start",
						"videomotion_stop",
						"alarmlocal_start",
						"alarmlocal_stop",
						"smartmotionhuman_start",
						"smartmotionhuman_stop",
						"smartmotionvehicle_start",
						"smartmotionvehicle_stop",
						"crosslinedetection_start",
						"crosslinedetection_stop",
						"crossregiondetection_start",
						"crossregiondetection_stop",
					},
					Icon:   "mdi:cctv",
					Device: toDevicePayload(device),
				},
			},
		}
		if hasStateFeature(state.Info, "control_aux_features", "siren") {
			entities = append(entities, discoveredEntity{
				component: "button",
				objectID:  "siren",
				config: entityConfig{
					Name:              "Siren",
					UniqueID:          fmt.Sprintf("%s_%s_siren", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "siren"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_siren", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:bullhorn",
					Device:            toDevicePayload(device),
				},
			})
		}
		if hasStateFeature(state.Info, "control_aux_features", "warning_light") {
			entities = append(entities, discoveredEntity{
				component: "button",
				objectID:  "warning_light",
				config: entityConfig{
					Name:              "Warning Light",
					UniqueID:          fmt.Sprintf("%s_%s_warning_light", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "warning_light"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_warning_light", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:alarm-light",
					Device:            toDevicePayload(device),
				},
			})
		}
		if hasStateFeature(state.Info, "control_aux_features", "wiper") {
			entities = append(entities, discoveredEntity{
				component: "button",
				objectID:  "wiper",
				config: entityConfig{
					Name:              "Wiper",
					UniqueID:          fmt.Sprintf("%s_%s_wiper", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "wiper"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_wiper", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:wiper",
					Device:            toDevicePayload(device),
				},
			})
		}
		if _, ok := state.Info["control_audio_supported"]; ok {
			entities = append(entities,
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "audio_control_supported",
					config: entityConfig{
						Name:              "Audio Control Supported",
						UniqueID:          fmt.Sprintf("%s_%s_audio_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_supported else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_control_supported", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:volume-source",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "audio_mute_control_supported",
					config: entityConfig{
						Name:              "Audio Mute Supported",
						UniqueID:          fmt.Sprintf("%s_%s_audio_mute_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_mute_supported else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_mute_control_supported", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:volume-mute",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "audio_volume_control_supported",
					config: entityConfig{
						Name:              "Audio Volume Supported",
						UniqueID:          fmt.Sprintf("%s_%s_audio_volume_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_volume_supported else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_volume_control_supported", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:volume-high",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "audio_playback_supported",
					config: entityConfig{
						Name:              "Audio Playback Supported",
						UniqueID:          fmt.Sprintf("%s_%s_audio_playback_supported", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_playback_supported else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_playback_supported", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:play-box-multiple",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "audio_playback_siren_supported",
					config: entityConfig{
						Name:              "Audio Siren Supported",
						UniqueID:          fmt.Sprintf("%s_%s_audio_playback_siren_supported", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_playback_siren else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_playback_siren_supported", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:bullhorn",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "sensor",
					objectID:  "audio_playback_clip_count",
					config: entityConfig{
						Name:              "Audio Playback Clip Count",
						UniqueID:          fmt.Sprintf("%s_%s_audio_playback_clip_count", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ value_json.info.control_audio_playback_file_count }}",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_audio_playback_clip_count", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:playlist-music",
						Device:            toDevicePayload(device),
					},
				},
			)
		}
		if stateInfoBool(state.Info, "control_recording_supported") {
			entities = append(entities,
				discoveredEntity{
					component: "binary_sensor",
					objectID:  "recording_active",
					config: entityConfig{
						Name:              "Recording Active",
						UniqueID:          fmt.Sprintf("%s_%s_recording_active", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ 'ON' if value_json.info.control_recording_active else 'OFF' }}",
						PayloadOn:         "ON",
						PayloadOff:        "OFF",
						ObjectID:          fmt.Sprintf("%s_recording_active", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:record-rec",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "sensor",
					objectID:  "recording_mode",
					config: entityConfig{
						Name:              "Recording Mode",
						UniqueID:          fmt.Sprintf("%s_%s_recording_mode", p.cfg.HomeAssistant.NodeID, device.ID),
						StateTopic:        infoTopic,
						ValueTemplate:     "{{ value_json.info.control_recording_mode }}",
						EntityCategory:    "diagnostic",
						ObjectID:          fmt.Sprintf("%s_recording_mode", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:video-wireless",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "button",
					objectID:  "recording_start",
					config: entityConfig{
						Name:              "Start Recording",
						UniqueID:          fmt.Sprintf("%s_%s_recording_start", p.cfg.HomeAssistant.NodeID, device.ID),
						CommandTopic:      p.commandTopic(device.ID, "recording_start"),
						PayloadPress:      "PRESS",
						ObjectID:          fmt.Sprintf("%s_recording_start", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:record-rec",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "button",
					objectID:  "recording_stop",
					config: entityConfig{
						Name:              "Stop Recording",
						UniqueID:          fmt.Sprintf("%s_%s_recording_stop", p.cfg.HomeAssistant.NodeID, device.ID),
						CommandTopic:      p.commandTopic(device.ID, "recording_stop"),
						PayloadPress:      "PRESS",
						ObjectID:          fmt.Sprintf("%s_recording_stop", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:stop-circle-outline",
						Device:            toDevicePayload(device),
					},
				},
			)
		}
		entities = append(entities, discoveredEntity{
			component: "sensor",
			objectID:  "validation_notes",
			config: entityConfig{
				Name:              "Validation Notes",
				UniqueID:          fmt.Sprintf("%s_%s_validation_notes", p.cfg.HomeAssistant.NodeID, device.ID),
				StateTopic:        infoTopic,
				ValueTemplate:     "{{ value_json.info.validation_notes_text }}",
				EntityCategory:    "diagnostic",
				ObjectID:          fmt.Sprintf("%s_validation_notes", device.ID),
				AvailabilityTopic: availabilityTopic,
				AvailabilityMode:  "latest",
				Icon:              "mdi:note-text-outline",
				Device:            toDevicePayload(device),
			},
		})
		return entities
	case dahua.DeviceKindNVRDisk:
		return []discoveredEntity{
			{
				component: "sensor",
				objectID:  "state",
				config: entityConfig{
					Name:              "Disk State",
					UniqueID:          fmt.Sprintf("%s_%s_state", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.state }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_state", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:harddisk",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "total_bytes",
				config: entityConfig{
					Name:              "Total Bytes",
					UniqueID:          fmt.Sprintf("%s_%s_total_bytes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.total_bytes }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_total_bytes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:database",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "used_bytes",
				config: entityConfig{
					Name:              "Used Bytes",
					UniqueID:          fmt.Sprintf("%s_%s_used_bytes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.used_bytes }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_used_bytes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:database-outline",
					Device:            toDevicePayload(device),
				},
			},
		}
	case dahua.DeviceKindVTO:
		entities := []discoveredEntity{
			{
				component: "sensor",
				objectID:  "current_profile",
				config: entityConfig{
					Name:              "Current Profile",
					UniqueID:          fmt.Sprintf("%s_%s_current_profile", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.current_profile }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_current_profile", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:account-voice",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "main_resolution",
				config: entityConfig{
					Name:              "Main Resolution",
					UniqueID:          fmt.Sprintf("%s_%s_main_resolution", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.main_resolution }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_main_resolution", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:video-high-definition",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "audio_codec",
				config: entityConfig{
					Name:              "Audio Codec",
					UniqueID:          fmt.Sprintf("%s_%s_audio_codec", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.audio_codec }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_audio_codec", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:waveform",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "alarm_input_count",
				config: entityConfig{
					Name:              "Alarm Input Count",
					UniqueID:          fmt.Sprintf("%s_%s_alarm_input_count", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.alarm_input_count }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_alarm_input_count", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:alarm-light",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "audio_output_volume_control_supported",
				config: entityConfig{
					Name:              "Output Volume Control Supported",
					UniqueID:          fmt.Sprintf("%s_%s_audio_output_volume_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_output_volume_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_audio_output_volume_control_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:volume-high",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "audio_input_volume_control_supported",
				config: entityConfig{
					Name:              "Mic Volume Control Supported",
					UniqueID:          fmt.Sprintf("%s_%s_audio_input_volume_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_input_volume_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_audio_input_volume_control_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:microphone",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "mute_control_supported",
				config: entityConfig{
					Name:              "Mute Control Supported",
					UniqueID:          fmt.Sprintf("%s_%s_mute_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_mute_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_mute_control_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:volume-mute",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "recording_control_supported",
				config: entityConfig{
					Name:              "Recording Control Supported",
					UniqueID:          fmt.Sprintf("%s_%s_recording_control_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_recording_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_recording_control_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:record-rec",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "direct_talkback_supported",
				config: entityConfig{
					Name:              "Direct Talkback Supported",
					UniqueID:          fmt.Sprintf("%s_%s_direct_talkback_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_direct_talkback_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_direct_talkback_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:microphone-message",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "full_call_acceptance_supported",
				config: entityConfig{
					Name:              "Full Call Acceptance Supported",
					UniqueID:          fmt.Sprintf("%s_%s_full_call_acceptance_supported", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_full_call_acceptance_supported else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_full_call_acceptance_supported", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:phone-check",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "event_snapshot_local_storage",
				config: entityConfig{
					Name:              "Event Snapshot Local Storage",
					UniqueID:          fmt.Sprintf("%s_%s_event_snapshot_local_storage", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.record_storage_event_snapshot_local else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_event_snapshot_local_storage", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:content-save-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "output_volume",
				config: entityConfig{
					Name:              "Output Volume",
					UniqueID:          fmt.Sprintf("%s_%s_output_volume", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.control_audio_output_volume }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "%",
					ObjectID:          fmt.Sprintf("%s_output_volume", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:volume-high",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "input_volume",
				config: entityConfig{
					Name:              "Mic Volume",
					UniqueID:          fmt.Sprintf("%s_%s_input_volume", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.control_audio_input_volume }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "%",
					ObjectID:          fmt.Sprintf("%s_input_volume", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:microphone",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "muted",
				config: entityConfig{
					Name:              "Muted",
					UniqueID:          fmt.Sprintf("%s_%s_muted", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_muted else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_muted", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:volume-off",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "auto_record_enabled",
				config: entityConfig{
					Name:              "Auto Record Enabled",
					UniqueID:          fmt.Sprintf("%s_%s_auto_record_enabled", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_recording_auto_enabled else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_auto_record_enabled", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:record-rec",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "auto_record_time_seconds",
				config: entityConfig{
					Name:              "Auto Record Time",
					UniqueID:          fmt.Sprintf("%s_%s_auto_record_time_seconds", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.control_recording_auto_time_seconds }}",
					EntityCategory:    "diagnostic",
					UnitOfMeasurement: "s",
					ObjectID:          fmt.Sprintf("%s_auto_record_time_seconds", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:timer-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "stream_audio_enabled",
				config: entityConfig{
					Name:              "Stream Audio Enabled",
					UniqueID:          fmt.Sprintf("%s_%s_stream_audio_enabled", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ 'ON' if value_json.info.control_stream_audio_enabled else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_stream_audio_enabled", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:speaker-wireless",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "doorbell",
				config: entityConfig{
					Name:              "Doorbell",
					UniqueID:          fmt.Sprintf("%s_%s_doorbell", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "doorbell"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_doorbell", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:doorbell-video",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "call",
				config: entityConfig{
					Name:              "Call Active",
					UniqueID:          fmt.Sprintf("%s_%s_call", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "call"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_call", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:phone",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "call_state",
				config: entityConfig{
					Name:              "Call State",
					UniqueID:          fmt.Sprintf("%s_%s_call_state", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "call_state"),
					ObjectID:          fmt.Sprintf("%s_call_state", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:phone-ring",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "last_ring_at",
				config: entityConfig{
					Name:              "Last Ring At",
					UniqueID:          fmt.Sprintf("%s_%s_last_ring_at", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "last_ring_at"),
					ObjectID:          fmt.Sprintf("%s_last_ring_at", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "timestamp",
					Icon:              "mdi:doorbell-video",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "last_call_started_at",
				config: entityConfig{
					Name:              "Last Call Started At",
					UniqueID:          fmt.Sprintf("%s_%s_last_call_started_at", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "last_call_started_at"),
					ObjectID:          fmt.Sprintf("%s_last_call_started_at", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "timestamp",
					Icon:              "mdi:phone-in-talk",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "last_call_ended_at",
				config: entityConfig{
					Name:              "Last Call Ended At",
					UniqueID:          fmt.Sprintf("%s_%s_last_call_ended_at", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "last_call_ended_at"),
					ObjectID:          fmt.Sprintf("%s_last_call_ended_at", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "timestamp",
					Icon:              "mdi:phone-hangup",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "last_call_duration_seconds",
				config: entityConfig{
					Name:              "Last Call Duration",
					UniqueID:          fmt.Sprintf("%s_%s_last_call_duration_seconds", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "last_call_duration_seconds"),
					ObjectID:          fmt.Sprintf("%s_last_call_duration_seconds", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					UnitOfMeasurement: "s",
					Icon:              "mdi:timer-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "last_call_source",
				config: entityConfig{
					Name:              "Last Call Source",
					UniqueID:          fmt.Sprintf("%s_%s_last_call_source", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "last_call_source"),
					ObjectID:          fmt.Sprintf("%s_last_call_source", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:account-voice",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "button",
				objectID:  "answer",
				config: entityConfig{
					Name:              "Answer Call",
					UniqueID:          fmt.Sprintf("%s_%s_answer", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "answer"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_answer", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:phone",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "button",
				objectID:  "hangup",
				config: entityConfig{
					Name:              "Hang Up Call",
					UniqueID:          fmt.Sprintf("%s_%s_hangup", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "hangup"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_hangup", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:phone-hangup",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "number",
				objectID:  "output_volume_control",
				config: entityConfig{
					Name:              "Output Volume Control",
					UniqueID:          fmt.Sprintf("%s_%s_output_volume_control", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					CommandTopic:      p.commandTopic(device.ID, "output_volume"),
					ValueTemplate:     "{{ value_json.info.control_audio_output_volume }}",
					Min:               float64Ptr(0),
					Max:               float64Ptr(100),
					Step:              float64Ptr(1),
					Mode:              "slider",
					ObjectID:          fmt.Sprintf("%s_output_volume_control", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					UnitOfMeasurement: "%",
					Icon:              "mdi:volume-high",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "number",
				objectID:  "input_volume_control",
				config: entityConfig{
					Name:              "Input Volume Control",
					UniqueID:          fmt.Sprintf("%s_%s_input_volume_control", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					CommandTopic:      p.commandTopic(device.ID, "input_volume"),
					ValueTemplate:     "{{ value_json.info.control_audio_input_volume }}",
					Min:               float64Ptr(0),
					Max:               float64Ptr(100),
					Step:              float64Ptr(1),
					Mode:              "slider",
					ObjectID:          fmt.Sprintf("%s_input_volume_control", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					UnitOfMeasurement: "%",
					Icon:              "mdi:microphone",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "switch",
				objectID:  "mute",
				config: entityConfig{
					Name:              "Mute",
					UniqueID:          fmt.Sprintf("%s_%s_mute", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					CommandTopic:      p.commandTopic(device.ID, "mute"),
					ValueTemplate:     "{{ 'ON' if value_json.info.control_audio_muted else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_mute", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:volume-mute",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "switch",
				objectID:  "auto_record",
				config: entityConfig{
					Name:              "Auto Record",
					UniqueID:          fmt.Sprintf("%s_%s_auto_record", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					CommandTopic:      p.commandTopic(device.ID, "auto_record"),
					ValueTemplate:     "{{ 'ON' if value_json.info.control_recording_auto_enabled else 'OFF' }}",
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_auto_record", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:record-rec",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "tamper",
				config: entityConfig{
					Name:              "Tamper",
					UniqueID:          fmt.Sprintf("%s_%s_tamper", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "tamper"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_tamper", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					DeviceClass:       "tamper",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "access",
				config: entityConfig{
					Name:              "Access Active",
					UniqueID:          fmt.Sprintf("%s_%s_access", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "access"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_access", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:door-open",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "validation_notes",
				config: entityConfig{
					Name:              "Validation Notes",
					UniqueID:          fmt.Sprintf("%s_%s_validation_notes", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.validation_notes_text }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_validation_notes", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:note-text-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "event",
				objectID:  "activity",
				config: entityConfig{
					Name:              "Activity",
					UniqueID:          fmt.Sprintf("%s_%s_activity", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.eventTopic(device.ID, "activity"),
					ObjectID:          fmt.Sprintf("%s_activity", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					EventTypes: []string{
						"alarmlocal_start",
						"alarmlocal_stop",
						"doorbell_start",
						"doorbell_stop",
						"tamper_start",
						"tamper_stop",
						"call_start",
						"call_stop",
						"accesscontrol_start",
						"accesscontrol_stop",
					},
					Icon:   "mdi:doorbell-video",
					Device: toDevicePayload(device),
				},
			},
		}
		if !stateInfoBool(state.Info, "control_audio_output_volume_supported") {
			entities = removeDiscoveredEntity(entities, "output_volume_control")
		}
		if !stateInfoBool(state.Info, "control_audio_input_volume_supported") {
			entities = removeDiscoveredEntity(entities, "input_volume_control")
		}
		if !stateInfoBool(state.Info, "control_audio_mute_supported") {
			entities = removeDiscoveredEntity(entities, "mute")
		}
		if !stateInfoBool(state.Info, "control_recording_supported") {
			entities = removeDiscoveredEntity(entities, "auto_record")
		}
		if p.cfg.Media.Enabled {
			entities = append(entities, discoveredEntity{
				component: "button",
				objectID:  "intercom_reset",
				config: entityConfig{
					Name:              "Reset Bridge Session",
					UniqueID:          fmt.Sprintf("%s_%s_intercom_reset", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "intercom_reset"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_intercom_reset", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:restart",
					Device:            toDevicePayload(device),
				},
			})
		}
		if len(p.cfg.Media.WebRTCUplinkTargets) > 0 {
			entities = append(entities,
				discoveredEntity{
					component: "button",
					objectID:  "uplink_enable",
					config: entityConfig{
						Name:              "Enable RTP Export",
						UniqueID:          fmt.Sprintf("%s_%s_uplink_enable", p.cfg.HomeAssistant.NodeID, device.ID),
						CommandTopic:      p.commandTopic(device.ID, "uplink_enable"),
						PayloadPress:      "PRESS",
						ObjectID:          fmt.Sprintf("%s_uplink_enable", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:upload-network",
						Device:            toDevicePayload(device),
					},
				},
				discoveredEntity{
					component: "button",
					objectID:  "uplink_disable",
					config: entityConfig{
						Name:              "Disable RTP Export",
						UniqueID:          fmt.Sprintf("%s_%s_uplink_disable", p.cfg.HomeAssistant.NodeID, device.ID),
						CommandTopic:      p.commandTopic(device.ID, "uplink_disable"),
						PayloadPress:      "PRESS",
						ObjectID:          fmt.Sprintf("%s_uplink_disable", device.ID),
						AvailabilityTopic: availabilityTopic,
						AvailabilityMode:  "latest",
						Icon:              "mdi:upload-off",
						Device:            toDevicePayload(device),
					},
				},
			)
		}
		return entities
	case dahua.DeviceKindIPC:
		return []discoveredEntity{
			{
				component: "sensor",
				objectID:  "device_type",
				config: entityConfig{
					Name:              "Device Type",
					UniqueID:          fmt.Sprintf("%s_%s_device_type", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.device_type }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_device_type", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:cctv",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "hardware_version",
				config: entityConfig{
					Name:              "Hardware Version",
					UniqueID:          fmt.Sprintf("%s_%s_hardware_version", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.hardware_version }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_hardware_version", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:chip",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "device_class",
				config: entityConfig{
					Name:              "Device Class",
					UniqueID:          fmt.Sprintf("%s_%s_device_class", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.device_class }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_device_class", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:shape-outline",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "process_info",
				config: entityConfig{
					Name:              "Process Info",
					UniqueID:          fmt.Sprintf("%s_%s_process_info", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.process_info }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_process_info", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:cpu-64-bit",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "motion",
				config: entityConfig{
					Name:              "Motion",
					UniqueID:          fmt.Sprintf("%s_%s_motion", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "motion"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					DeviceClass:       "motion",
					ObjectID:          fmt.Sprintf("%s_motion", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "human",
				config: entityConfig{
					Name:              "Human",
					UniqueID:          fmt.Sprintf("%s_%s_human", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "human"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_human", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:account",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "vehicle",
				config: entityConfig{
					Name:              "Vehicle",
					UniqueID:          fmt.Sprintf("%s_%s_vehicle", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "vehicle"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_vehicle", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:car",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "tripwire",
				config: entityConfig{
					Name:              "Tripwire",
					UniqueID:          fmt.Sprintf("%s_%s_tripwire", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "tripwire"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_tripwire", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:vector-line",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "intrusion",
				config: entityConfig{
					Name:              "Intrusion",
					UniqueID:          fmt.Sprintf("%s_%s_intrusion", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "intrusion"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_intrusion", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:shield-alert",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "event",
				objectID:  "activity",
				config: entityConfig{
					Name:              "Activity",
					UniqueID:          fmt.Sprintf("%s_%s_activity", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.eventTopic(device.ID, "activity"),
					ObjectID:          fmt.Sprintf("%s_activity", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					EventTypes: []string{
						"videomotion_start",
						"videomotion_stop",
						"alarmlocal_start",
						"alarmlocal_stop",
						"smartmotionhuman_start",
						"smartmotionhuman_stop",
						"smartmotionvehicle_start",
						"smartmotionvehicle_stop",
						"crosslinedetection_start",
						"crosslinedetection_stop",
						"crossregiondetection_start",
						"crossregiondetection_stop",
					},
					Icon:   "mdi:cctv",
					Device: toDevicePayload(device),
				},
			},
		}
	case dahua.DeviceKindVTOLock:
		return []discoveredEntity{
			{
				component: "button",
				objectID:  "open",
				config: entityConfig{
					Name:              "Open Door",
					UniqueID:          fmt.Sprintf("%s_%s_open", p.cfg.HomeAssistant.NodeID, device.ID),
					CommandTopic:      p.commandTopic(device.ID, "press"),
					PayloadPress:      "PRESS",
					ObjectID:          fmt.Sprintf("%s_open", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:door-open",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "state",
				config: entityConfig{
					Name:              "Lock State",
					UniqueID:          fmt.Sprintf("%s_%s_state", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.state }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_state", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:lock",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "sensor",
				objectID:  "sensor_enabled",
				config: entityConfig{
					Name:              "Door Sensor Enabled",
					UniqueID:          fmt.Sprintf("%s_%s_sensor_enabled", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.sensor_enabled }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_sensor_enabled", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:door",
					Device:            toDevicePayload(device),
				},
			},
		}
	case dahua.DeviceKindVTOAlarm:
		return []discoveredEntity{
			{
				component: "sensor",
				objectID:  "sense_method",
				config: entityConfig{
					Name:              "Sense Method",
					UniqueID:          fmt.Sprintf("%s_%s_sense_method", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        infoTopic,
					ValueTemplate:     "{{ value_json.info.sense_method }}",
					EntityCategory:    "diagnostic",
					ObjectID:          fmt.Sprintf("%s_sense_method", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:alarm-panel",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "binary_sensor",
				objectID:  "active",
				config: entityConfig{
					Name:              "Active",
					UniqueID:          fmt.Sprintf("%s_%s_active", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.stateTopic(device.ID, "active"),
					PayloadOn:         "ON",
					PayloadOff:        "OFF",
					ObjectID:          fmt.Sprintf("%s_active", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					Icon:              "mdi:alarm-light",
					Device:            toDevicePayload(device),
				},
			},
			{
				component: "event",
				objectID:  "activity",
				config: entityConfig{
					Name:              "Activity",
					UniqueID:          fmt.Sprintf("%s_%s_activity", p.cfg.HomeAssistant.NodeID, device.ID),
					StateTopic:        p.eventTopic(device.ID, "activity"),
					ObjectID:          fmt.Sprintf("%s_activity", device.ID),
					AvailabilityTopic: availabilityTopic,
					AvailabilityMode:  "latest",
					EventTypes: []string{
						"alarmlocal_start",
						"alarmlocal_stop",
						"alarmlocal_pulse",
					},
					Icon:   "mdi:alarm-light",
					Device: toDevicePayload(device),
				},
			},
		}
	default:
		return nil
	}
}

func hasStateFeature(values map[string]any, key string, want string) bool {
	for _, value := range stateInfoStringSlice(values, key) {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func stateInfoBool(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	switch typed := values[key].(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func float64Ptr(value float64) *float64 {
	return &value
}

func stateInfoStringSlice(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return nil
	}

	switch typed := raw.(type) {
	case []string:
		result := make([]string, 0, len(typed))
		for _, value := range typed {
			value = strings.TrimSpace(value)
			if value != "" {
				result = append(result, value)
			}
		}
		return result
	case []any:
		result := make([]string, 0, len(typed))
		for _, value := range typed {
			text, ok := value.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func (p *DiscoveryPublisher) extraTriggerConfigs(device dahua.Device) []discoveredTrigger {
	if device.Kind == dahua.DeviceKindIPC {
		eventTopic := p.eventTopic(device.ID, "activity")
		triggers := []struct {
			objectID  string
			triggerID string
			typeName  string
			subtype   string
		}{
			{objectID: "motion_start", triggerID: "videomotion_start", typeName: "motion_detected", subtype: "motion"},
			{objectID: "human_start", triggerID: "smartmotionhuman_start", typeName: "human_detected", subtype: "human"},
			{objectID: "vehicle_start", triggerID: "smartmotionvehicle_start", typeName: "vehicle_detected", subtype: "vehicle"},
			{objectID: "tripwire_start", triggerID: "crosslinedetection_start", typeName: "tripwire_detected", subtype: "tripwire"},
			{objectID: "intrusion_start", triggerID: "crossregiondetection_start", typeName: "intrusion_detected", subtype: "intrusion"},
		}

		result := make([]discoveredTrigger, 0, len(triggers))
		for _, trigger := range triggers {
			result = append(result, discoveredTrigger{
				objectID: trigger.objectID,
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        trigger.triggerID,
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           trigger.typeName,
					Subtype:        trigger.subtype,
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			})
		}

		return result
	}

	if device.Kind == dahua.DeviceKindVTO {
		eventTopic := p.eventTopic(device.ID, "activity")
		return []discoveredTrigger{
			{
				objectID: "doorbell_start",
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        "doorbell_start",
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           "doorbell_pressed",
					Subtype:        "doorbell",
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			},
			{
				objectID: "call_start",
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        "call_start",
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           "call_started",
					Subtype:        "call",
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			},
			{
				objectID: "call_stop",
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        "call_stop",
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           "call_ended",
					Subtype:        "call",
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			},
			{
				objectID: "accesscontrol_start",
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        "accesscontrol_start",
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           "access_granted",
					Subtype:        "door_access",
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			},
			{
				objectID: "tamper_start",
				config: deviceTriggerConfig{
					AutomationType: "trigger",
					Topic:          eventTopic,
					Payload:        "tamper_start",
					ValueTemplate:  "{{ value_json.event_type }}",
					Type:           "tamper_detected",
					Subtype:        "tamper",
					QoS:            p.cfg.MQTT.QoS,
					Platform:       "device_automation",
					Device:         toDevicePayload(device),
				},
			},
		}
	}

	if device.Kind != dahua.DeviceKindNVRChannel {
		if device.Kind == dahua.DeviceKindVTOAlarm {
			eventTopic := p.eventTopic(device.ID, "activity")
			return []discoveredTrigger{
				{
					objectID: "active_start",
					config: deviceTriggerConfig{
						AutomationType: "trigger",
						Topic:          eventTopic,
						Payload:        "alarmlocal_start",
						ValueTemplate:  "{{ value_json.event_type }}",
						Type:           "alarm_detected",
						Subtype:        "alarm_input",
						QoS:            p.cfg.MQTT.QoS,
						Platform:       "device_automation",
						Device:         toDevicePayload(device),
					},
				},
			}
		}
		return nil
	}

	eventTopic := p.eventTopic(device.ID, "activity")
	triggers := []struct {
		objectID  string
		triggerID string
		typeName  string
		subtype   string
	}{
		{objectID: "motion_start", triggerID: "videomotion_start", typeName: "motion_detected", subtype: "motion"},
		{objectID: "human_start", triggerID: "smartmotionhuman_start", typeName: "human_detected", subtype: "human"},
		{objectID: "vehicle_start", triggerID: "smartmotionvehicle_start", typeName: "vehicle_detected", subtype: "vehicle"},
		{objectID: "tripwire_start", triggerID: "crosslinedetection_start", typeName: "tripwire_detected", subtype: "tripwire"},
		{objectID: "intrusion_start", triggerID: "crossregiondetection_start", typeName: "intrusion_detected", subtype: "intrusion"},
	}

	result := make([]discoveredTrigger, 0, len(triggers))
	for _, trigger := range triggers {
		result = append(result, discoveredTrigger{
			objectID: trigger.objectID,
			config: deviceTriggerConfig{
				AutomationType: "trigger",
				Topic:          eventTopic,
				Payload:        trigger.triggerID,
				ValueTemplate:  "{{ value_json.event_type }}",
				Type:           trigger.typeName,
				Subtype:        trigger.subtype,
				QoS:            p.cfg.MQTT.QoS,
				Platform:       "device_automation",
				Device:         toDevicePayload(device),
			},
		})
	}

	return result
}

func (p *DiscoveryPublisher) publishDeviceTrigger(ctx context.Context, deviceID string, objectID string, payload any) error {
	topic := p.deviceTriggerTopic(deviceID, objectID)
	p.logger.Debug().Str("topic", topic).Msg("publishing home assistant device trigger discovery")
	return p.mqtt.PublishJSON(ctx, topic, p.cfg.MQTT.QoS, p.cfg.MQTT.Retain, payload)
}
