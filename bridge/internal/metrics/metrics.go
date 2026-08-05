package metrics

import (
	"net/http"
	"time"

	"RCooLeR/DahuaBridge/internal/buildinfo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	registry            *prometheus.Registry
	ProbeTotal          *prometheus.CounterVec
	ProbeDuration       *prometheus.HistogramVec
	DahuaRequestTotal   *prometheus.CounterVec
	DeviceAvailability  *prometheus.GaugeVec
	EventTotal          *prometheus.CounterVec
	EventLastSeen       *prometheus.GaugeVec
	EventStreamUp       *prometheus.GaugeVec
	EventStreamRestarts *prometheus.CounterVec
	StateStoreTotal     *prometheus.CounterVec
	MediaWorkers        prometheus.Gauge
	MediaViewers        *prometheus.GaugeVec
	MediaFramesTotal    *prometheus.CounterVec
	MediaFrameDrops     *prometheus.CounterVec
	MediaStartsTotal    *prometheus.CounterVec
}

func New(info buildinfo.BuildInfo) *Registry {
	reg := prometheus.NewRegistry()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dahuabridge_build_info",
		Help: "Static build information.",
	}, []string{"version", "commit", "build_date"})
	buildInfo.WithLabelValues(info.Version, info.Commit, info.BuildDate).Set(1)

	probeTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_probe_total",
		Help: "Total number of device probes.",
	}, []string{"device_id", "device_type", "status"})

	probeDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dahuabridge_probe_duration_seconds",
		Help:    "Duration of Dahua device probes.",
		Buckets: prometheus.DefBuckets,
	}, []string{"device_id", "device_type"})

	dahuaRequestTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_dahua_request_total",
		Help: "Total number of outbound Dahua HTTP requests.",
	}, []string{"device_id", "endpoint", "method", "status"})

	deviceAvailability := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dahuabridge_device_available",
		Help: "Current device availability, 1 for up and 0 for down.",
	}, []string{"device_id", "device_type"})

	eventTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_event_total",
		Help: "Total number of normalized Dahua events.",
	}, []string{"device_id", "device_type", "code", "action", "channel"})

	eventLastSeen := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dahuabridge_event_last_seen_timestamp_seconds",
		Help: "Unix timestamp for the last normalized event received by the bridge.",
	}, []string{"device_id", "device_type"})

	eventStreamUp := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dahuabridge_event_stream_up",
		Help: "Current event stream connection state, 1 for connected and 0 for disconnected.",
	}, []string{"device_id", "device_type"})

	eventStreamRestarts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_event_stream_restarts_total",
		Help: "Total number of event stream restarts after unexpected stream exits.",
	}, []string{"device_id", "device_type", "status"})

	stateStoreTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_state_store_total",
		Help: "Total number of probe state store operations.",
	}, []string{"operation", "status"})

	mediaWorkers := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dahuabridge_media_workers",
		Help: "Current number of active media workers.",
	})

	mediaViewers := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dahuabridge_media_viewers",
		Help: "Current number of viewers attached to a media worker.",
	}, []string{"stream_id", "profile"})

	mediaFramesTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_media_frames_total",
		Help: "Total number of MJPEG frames published by media workers.",
	}, []string{"stream_id", "profile"})

	mediaFrameDrops := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_media_frame_drops_total",
		Help: "Total number of media frames dropped because subscriber channels were full.",
	}, []string{"stream_id", "profile"})

	mediaStartsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "dahuabridge_media_starts_total",
		Help: "Total number of media worker start attempts.",
	}, []string{"stream_id", "profile", "status"})

	reg.MustRegister(
		buildInfo,
		probeTotal,
		probeDuration,
		dahuaRequestTotal,
		deviceAvailability,
		eventTotal,
		eventLastSeen,
		eventStreamUp,
		eventStreamRestarts,
		stateStoreTotal,
		mediaWorkers,
		mediaViewers,
		mediaFramesTotal,
		mediaFrameDrops,
		mediaStartsTotal,
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	return &Registry{
		registry:            reg,
		ProbeTotal:          probeTotal,
		ProbeDuration:       probeDuration,
		DahuaRequestTotal:   dahuaRequestTotal,
		DeviceAvailability:  deviceAvailability,
		EventTotal:          eventTotal,
		EventLastSeen:       eventLastSeen,
		EventStreamUp:       eventStreamUp,
		EventStreamRestarts: eventStreamRestarts,
		StateStoreTotal:     stateStoreTotal,
		MediaWorkers:        mediaWorkers,
		MediaViewers:        mediaViewers,
		MediaFramesTotal:    mediaFramesTotal,
		MediaFrameDrops:     mediaFrameDrops,
		MediaStartsTotal:    mediaStartsTotal,
	}
}

func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

func (r *Registry) ObserveProbe(deviceID string, deviceType string, started time.Time, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}

	r.ProbeTotal.WithLabelValues(deviceID, deviceType, status).Inc()
	r.ProbeDuration.WithLabelValues(deviceID, deviceType).Observe(time.Since(started).Seconds())
}

func (r *Registry) ObserveDahuaRequest(deviceID string, endpoint string, method string, status string) {
	r.DahuaRequestTotal.WithLabelValues(deviceID, endpoint, method, status).Inc()
}

func (r *Registry) ObserveEvent(deviceID string, deviceType string, code string, action string, channel string) {
	r.EventTotal.WithLabelValues(deviceID, deviceType, code, action, channel).Inc()
	r.EventLastSeen.WithLabelValues(deviceID, deviceType).Set(float64(time.Now().Unix()))
}

func (r *Registry) SetEventStreamUp(deviceID string, deviceType string, up bool) {
	value := 0.0
	if up {
		value = 1
	}
	r.EventStreamUp.WithLabelValues(deviceID, deviceType).Set(value)
}

func (r *Registry) ObserveEventStreamRestart(deviceID string, deviceType string, err error) {
	status := "closed"
	if err != nil {
		status = "error"
	}
	r.EventStreamRestarts.WithLabelValues(deviceID, deviceType, status).Inc()
}

func (r *Registry) ObserveStateStore(operation string, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	r.StateStoreTotal.WithLabelValues(operation, status).Inc()
}

func (r *Registry) SetMediaWorkers(count int) {
	r.MediaWorkers.Set(float64(count))
}

func (r *Registry) SetMediaViewers(streamID string, profile string, count int) {
	r.MediaViewers.WithLabelValues(streamID, profile).Set(float64(count))
}

func (r *Registry) ObserveMediaFrame(streamID string, profile string) {
	r.MediaFramesTotal.WithLabelValues(streamID, profile).Inc()
}

func (r *Registry) ObserveMediaFrameDrop(streamID string, profile string) {
	r.MediaFrameDrops.WithLabelValues(streamID, profile).Inc()
}

func (r *Registry) ObserveMediaStart(streamID string, profile string, err error) {
	status := "success"
	if err != nil {
		status = "error"
	}
	r.MediaStartsTotal.WithLabelValues(streamID, profile, status).Inc()
}
