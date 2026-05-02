package dahua

import (
	"context"
	"errors"
	"io"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
)

var (
	ErrDeviceNotFound          = errors.New("device not found")
	ErrPlaybackSessionNotFound = errors.New("playback session not found")
	ErrUnsupportedOperation    = errors.New("unsupported operation")
)

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

type NVRRecordingQuery struct {
	Channel   int
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	EventCode string
	EventOnly bool
}

type NVRRecording struct {
	ID               string   `json:"id,omitempty"`
	RecordKind       string   `json:"record_kind,omitempty"`
	Source           string   `json:"source,omitempty"`
	Status           string   `json:"status,omitempty"`
	ClipID           string   `json:"clip_id,omitempty"`
	StreamID         string   `json:"stream_id,omitempty"`
	DownloadURL      string   `json:"download_url,omitempty"`
	ExportURL        string   `json:"export_url,omitempty"`
	AssetStatus      string   `json:"asset_status,omitempty"`
	AssetClipID      string   `json:"asset_clip_id,omitempty"`
	AssetError       string   `json:"asset_error,omitempty"`
	AssetPlaybackURL string   `json:"asset_playback_url,omitempty"`
	AssetDownloadURL string   `json:"asset_download_url,omitempty"`
	AssetSelfURL     string   `json:"asset_self_url,omitempty"`
	AssetStopURL     string   `json:"asset_stop_url,omitempty"`
	Channel          int      `json:"channel"`
	StartTime        string   `json:"start_time"`
	EndTime          string   `json:"end_time"`
	FilePath         string   `json:"file_path,omitempty"`
	Type             string   `json:"type,omitempty"`
	VideoStream      string   `json:"video_stream,omitempty"`
	Disk             int      `json:"disk,omitempty"`
	Partition        int      `json:"partition,omitempty"`
	Cluster          int      `json:"cluster,omitempty"`
	LengthBytes      int64    `json:"length_bytes,omitempty"`
	CutLengthBytes   int64    `json:"cut_length_bytes,omitempty"`
	Flags            []string `json:"flags,omitempty"`
}

type NVRRecordingSearchResult struct {
	DeviceID      string         `json:"device_id"`
	Channel       int            `json:"channel"`
	StartTime     string         `json:"start_time"`
	EndTime       string         `json:"end_time"`
	Limit         int            `json:"limit"`
	ReturnedCount int            `json:"returned_count"`
	Items         []NVRRecording `json:"items"`
}

type NVREventSummaryItem struct {
	Code  string `json:"code"`
	Label string `json:"label,omitempty"`
	Count int    `json:"count"`
}

type NVREventChannelSummary struct {
	Channel    int                   `json:"channel"`
	TotalCount int                   `json:"total_count"`
	Items      []NVREventSummaryItem `json:"items"`
}

type NVREventSummary struct {
	DeviceID   string                   `json:"device_id"`
	StartTime  string                   `json:"start_time"`
	EndTime    string                   `json:"end_time"`
	TotalCount int                      `json:"total_count"`
	Items      []NVREventSummaryItem    `json:"items"`
	Channels   []NVREventChannelSummary `json:"channels"`
}

type NVRRecordingSearcher interface {
	FindRecordings(context.Context, NVRRecordingQuery) (NVRRecordingSearchResult, error)
}

type NVRRecordingDownload struct {
	Body          io.ReadCloser
	ContentType   string
	FileName      string
	ContentLength int64
}

type NVRRecordingDownloader interface {
	DownloadRecording(context.Context, string) (NVRRecordingDownload, error)
}

type NVRRecordingClipRequest struct {
	Channel     int
	StartTime   time.Time
	EndTime     time.Time
	FilePath    string
	Source      string
	Type        string
	VideoStream string
}

type NVRRecordingClipDownloader interface {
	DownloadRecordingClip(context.Context, NVRRecordingClipRequest) (NVRRecordingDownload, error)
}

type NVRRecordingIFrameDownloader interface {
	DownloadRecordingIFrame(context.Context, NVRRecordingClipRequest) (NVRRecordingDownload, error)
}

type NVRPlaybackSessionRequest struct {
	Channel     int
	StartTime   time.Time
	EndTime     time.Time
	SeekTime    time.Time
	FilePath    string
	Source      string
	Type        string
	VideoStream string
}

type NVRPlaybackProfile struct {
	Name           string `json:"name"`
	DASHURL        string `json:"dash_url,omitempty"`
	HLSURL         string `json:"hls_url,omitempty"`
	MJPEGURL       string `json:"mjpeg_url,omitempty"`
	WebRTCOfferURL string `json:"webrtc_offer_url,omitempty"`
}

type NVRPlaybackSession struct {
	ID                 string                        `json:"id"`
	StreamID           string                        `json:"stream_id"`
	DeviceID           string                        `json:"device_id"`
	SourceStreamID     string                        `json:"source_stream_id,omitempty"`
	Name               string                        `json:"name"`
	Channel            int                           `json:"channel"`
	StartTime          string                        `json:"start_time"`
	EndTime            string                        `json:"end_time"`
	SeekTime           string                        `json:"seek_time"`
	RecommendedProfile string                        `json:"recommended_profile"`
	SnapshotURL        string                        `json:"snapshot_url,omitempty"`
	CreatedAt          string                        `json:"created_at"`
	ExpiresAt          string                        `json:"expires_at"`
	Profiles           map[string]NVRPlaybackProfile `json:"profiles"`
}

type NVRChannelControlCapabilities struct {
	DeviceID  string                      `json:"device_id"`
	Channel   int                         `json:"channel"`
	PTZ       NVRPTZCapabilities          `json:"ptz"`
	Aux       NVRAuxCapabilities          `json:"aux"`
	Audio     NVRChannelAudioCapabilities `json:"audio"`
	Recording NVRRecordingCapabilities    `json:"recording"`
}

type NVRChannelAudioCapabilities struct {
	Supported              bool                                `json:"supported"`
	Mute                   bool                                `json:"mute"`
	Volume                 bool                                `json:"volume"`
	VolumePermissionDenied bool                                `json:"volume_permission_denied,omitempty"`
	Muted                  bool                                `json:"muted,omitempty"`
	StreamEnabled          bool                                `json:"stream_enabled,omitempty"`
	Playback               NVRChannelAudioPlaybackCapabilities `json:"playback"`
}

type NVRChannelAudioPlaybackCapabilities struct {
	Supported  bool                  `json:"supported"`
	Siren      bool                  `json:"siren"`
	QuickReply bool                  `json:"quick_reply"`
	Formats    []string              `json:"formats,omitempty"`
	FileCount  int                   `json:"file_count,omitempty"`
	Files      []NVRChannelAudioFile `json:"files,omitempty"`
}

type NVRChannelAudioFile struct {
	Name      string `json:"name"`
	Path      string `json:"path,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type NVRPTZCapabilities struct {
	Supported          bool     `json:"supported"`
	Pan                bool     `json:"pan"`
	Tilt               bool     `json:"tilt"`
	Zoom               bool     `json:"zoom"`
	Focus              bool     `json:"focus"`
	MoveRelatively     bool     `json:"move_relatively"`
	AutoScan           bool     `json:"auto_scan"`
	Preset             bool     `json:"preset"`
	Pattern            bool     `json:"pattern"`
	Tour               bool     `json:"tour"`
	Aux                bool     `json:"aux"`
	AuxFunctions       []string `json:"aux_functions,omitempty"`
	PanSpeedMin        int      `json:"pan_speed_min,omitempty"`
	PanSpeedMax        int      `json:"pan_speed_max,omitempty"`
	TiltSpeedMin       int      `json:"tilt_speed_min,omitempty"`
	TiltSpeedMax       int      `json:"tilt_speed_max,omitempty"`
	PresetMin          int      `json:"preset_min,omitempty"`
	PresetMax          int      `json:"preset_max,omitempty"`
	HorizontalAngleMin int      `json:"horizontal_angle_min,omitempty"`
	HorizontalAngleMax int      `json:"horizontal_angle_max,omitempty"`
	VerticalAngleMin   int      `json:"vertical_angle_min,omitempty"`
	VerticalAngleMax   int      `json:"vertical_angle_max,omitempty"`
	Commands           []string `json:"commands,omitempty"`
}

type NVRRecordingCapabilities struct {
	Supported bool   `json:"supported"`
	Active    bool   `json:"active"`
	Mode      string `json:"mode,omitempty"`
}

type NVRAuxCapabilities struct {
	Supported bool     `json:"supported"`
	Outputs   []string `json:"outputs,omitempty"`
	Features  []string `json:"features,omitempty"`
}

type NVRPTZAction string

const (
	NVRPTZActionStart NVRPTZAction = "start"
	NVRPTZActionStop  NVRPTZAction = "stop"
	NVRPTZActionPulse NVRPTZAction = "pulse"
)

type NVRPTZCommand string

const (
	NVRPTZCommandUp        NVRPTZCommand = "up"
	NVRPTZCommandDown      NVRPTZCommand = "down"
	NVRPTZCommandLeft      NVRPTZCommand = "left"
	NVRPTZCommandRight     NVRPTZCommand = "right"
	NVRPTZCommandLeftUp    NVRPTZCommand = "left_up"
	NVRPTZCommandRightUp   NVRPTZCommand = "right_up"
	NVRPTZCommandLeftDown  NVRPTZCommand = "left_down"
	NVRPTZCommandRightDown NVRPTZCommand = "right_down"
	NVRPTZCommandZoomIn    NVRPTZCommand = "zoom_in"
	NVRPTZCommandZoomOut   NVRPTZCommand = "zoom_out"
	NVRPTZCommandFocusNear NVRPTZCommand = "focus_near"
	NVRPTZCommandFocusFar  NVRPTZCommand = "focus_far"
)

type NVRPTZRequest struct {
	Channel  int
	Action   NVRPTZAction
	Command  NVRPTZCommand
	Speed    int
	Duration time.Duration
}

type NVRAuxAction string

const (
	NVRAuxActionStart NVRAuxAction = "start"
	NVRAuxActionStop  NVRAuxAction = "stop"
	NVRAuxActionPulse NVRAuxAction = "pulse"
)

type NVRAuxRequest struct {
	Channel  int
	Action   NVRAuxAction
	Output   string
	Duration time.Duration
}

type NVRAudioRequest struct {
	Channel int
	Muted   bool
}

type NVRRecordingAction string

const (
	NVRRecordingActionStart NVRRecordingAction = "start"
	NVRRecordingActionStop  NVRRecordingAction = "stop"
	NVRRecordingActionAuto  NVRRecordingAction = "auto"
)

type NVRRecordingRequest struct {
	Channel int
	Action  NVRRecordingAction
}

type NVRDiagnosticActionRequest struct {
	Channel  int
	Method   string
	Action   string
	Duration time.Duration
}

type NVRDiagnosticActionResult struct {
	Status      string   `json:"status"`
	DeviceID    string   `json:"device_id"`
	Channel     int      `json:"channel"`
	Method      string   `json:"method"`
	Action      string   `json:"action"`
	DurationMS  int64    `json:"duration_ms,omitempty"`
	Endpoint    string   `json:"endpoint,omitempty"`
	Description string   `json:"description,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

type NVRChannelControlReader interface {
	ChannelControlCapabilities(context.Context, int) (NVRChannelControlCapabilities, error)
}

type NVRPTZController interface {
	PTZ(context.Context, NVRPTZRequest) error
}

type NVRAuxController interface {
	Aux(context.Context, NVRAuxRequest) error
}

type NVRAudioController interface {
	SetAudioMute(context.Context, NVRAudioRequest) error
}

type NVRRecordingController interface {
	Recording(context.Context, NVRRecordingRequest) error
}

type NVRDiagnosticController interface {
	DiagnosticAction(context.Context, NVRDiagnosticActionRequest) (NVRDiagnosticActionResult, error)
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

type VTOControlCapabilities struct {
	DeviceID                    string                   `json:"device_id"`
	Call                        VTOCallCapabilities      `json:"call"`
	Locks                       VTOLockCapabilities      `json:"locks"`
	Audio                       VTOAudioCapabilities     `json:"audio"`
	Recording                   VTORecordingCapabilities `json:"recording"`
	DirectTalkbackSupported     bool                     `json:"direct_talkback_supported"`
	FullCallAcceptanceSupported bool                     `json:"full_call_acceptance_supported"`
	ValidationNotes             []string                 `json:"validation_notes,omitempty"`
}

type VTOCallCapabilities struct {
	Answer bool   `json:"answer"`
	Hangup bool   `json:"hangup"`
	State  string `json:"state,omitempty"`
}

type VTOLockCapabilities struct {
	Supported bool  `json:"supported"`
	Count     int   `json:"count,omitempty"`
	Indexes   []int `json:"indexes,omitempty"`
}

type VTOAudioCapabilities struct {
	OutputVolume       bool   `json:"output_volume"`
	InputVolume        bool   `json:"input_volume"`
	Mute               bool   `json:"mute"`
	Codec              string `json:"codec,omitempty"`
	OutputVolumeLevel  int    `json:"output_volume_level,omitempty"`
	OutputVolumeLevels []int  `json:"output_volume_levels,omitempty"`
	InputVolumeLevel   int    `json:"input_volume_level,omitempty"`
	InputVolumeLevels  []int  `json:"input_volume_levels,omitempty"`
	Muted              bool   `json:"muted,omitempty"`
	StreamAudioEnabled bool   `json:"stream_audio_enabled,omitempty"`
}

type VTORecordingCapabilities struct {
	Supported             bool `json:"supported"`
	EventSnapshotLocal    bool `json:"event_snapshot_local"`
	AutoRecordEnabled     bool `json:"auto_record_enabled,omitempty"`
	AutoRecordTimeSeconds int  `json:"auto_record_time_seconds,omitempty"`
}

type VTOControlReader interface {
	ControlCapabilities(context.Context) (VTOControlCapabilities, error)
}

type VTOAudioController interface {
	SetAudioOutputVolume(context.Context, int, int) error
	SetAudioInputVolume(context.Context, int, int) error
	SetAudioMute(context.Context, bool) error
}

type VTORecordingController interface {
	SetRecordingEnabled(context.Context, bool) error
}

type NVRInventoryRefresher interface {
	InvalidateInventoryCache()
}

type ConfigurableDriver interface {
	UpdateConfig(config.DeviceConfig) error
}
