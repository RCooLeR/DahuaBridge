package metrics

import "github.com/prometheus/client_golang/prometheus"

type RelayStatus struct {
	StreamID, Profile, Source          string
	Viewers                            int
	BytesReceived, Bitrate, Reconnects uint64
	Ready                              bool
}

type relayCollector struct {
	read                                        func() []RelayStatus
	active, viewers, bytes, bitrate, reconnects *prometheus.Desc
}

func (r *Registry) RegisterRelayMetrics(read func() []RelayStatus) {
	labels := []string{"stream_id", "profile", "source"}
	r.registry.MustRegister(&relayCollector{
		read:       read,
		active:     prometheus.NewDesc("dahuabridge_rtsp_upstream_active", "Active shared RTSP input, 1 after PLAY succeeds.", labels, nil),
		viewers:    prometheus.NewDesc("dahuabridge_rtsp_viewers", "Playing readers of a shared RTSP input, including local FFmpeg consumers.", labels, nil),
		bytes:      prometheus.NewDesc("dahuabridge_rtsp_session_bytes_received", "RTP bytes received during this upstream session.", labels, nil),
		bitrate:    prometheus.NewDesc("dahuabridge_rtsp_bitrate_bps", "Measured upstream RTP bitrate in bits per second.", labels, nil),
		reconnects: prometheus.NewDesc("dahuabridge_rtsp_reopens", "Stream reopen count in bounded recent history, including idle reopen and failover.", labels, nil),
	})
}

func (c *relayCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range []*prometheus.Desc{c.active, c.viewers, c.bytes, c.bitrate, c.reconnects} {
		ch <- desc
	}
}

func (c *relayCollector) Collect(ch chan<- prometheus.Metric) {
	for _, status := range c.read() {
		labels := []string{status.StreamID, status.Profile, status.Source}
		active := 0.0
		if status.Ready {
			active = 1
		}
		values := []float64{active, float64(status.Viewers), float64(status.BytesReceived), float64(status.Bitrate), float64(status.Reconnects)}
		// Session bytes reset on reconnect and reopen history can be evicted.
		// Both are gauges, not monotonic process-lifetime counters.
		for index, desc := range []*prometheus.Desc{c.active, c.viewers, c.bytes, c.bitrate, c.reconnects} {
			ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, values[index], labels...)
		}
	}
}
