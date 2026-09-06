package app

import (
	"RCooLeR/DahuaBridge/internal/media"
	"RCooLeR/DahuaBridge/internal/metrics"
	"RCooLeR/DahuaBridge/internal/rtsprelay"
)

func attachRelayDiagnostics(manager *media.Manager, registry *metrics.Registry, relay *rtsprelay.Server) {
	manager.AttachRelayStatuses(func() []media.WorkerStatus {
		result := make([]media.WorkerStatus, 0)
		for _, status := range relay.Statuses() {
			state := "starting"
			if status.Ready {
				state = "running"
			}
			result = append(result, media.WorkerStatus{
				Key: status.Key + ":rtsp", Format: "rtsp", State: state,
				StreamID: status.StreamID, Profile: status.Profile, Channel: status.Channel,
				LiveSource: status.Source, SourceURL: status.SourceURL, SharedInput: true,
				Warm: status.Warm, PreconnectMode: status.PreconnectMode,
				Viewers: status.Viewers, StartedAt: status.StartedAt, LastFrameAt: status.LastPacketAt,
				SourceVideoCodec: status.VideoCodec, SourceAudioCodec: status.AudioCodec,
				OutputVideoCodec: status.VideoCodec, OutputAudioCodec: status.AudioCodec, OutputVideoEncoder: "copy",
				BytesReceived: status.BytesReceived, Bitrate: status.Bitrate, Reconnects: status.Reconnects,
			})
		}
		return result
	})
	registry.RegisterRelayMetrics(func() []metrics.RelayStatus {
		result := make([]metrics.RelayStatus, 0)
		for _, status := range relay.Statuses() {
			result = append(result, metrics.RelayStatus{StreamID: status.StreamID, Profile: status.Profile,
				Source: status.Source, Viewers: status.Viewers, BytesReceived: status.BytesReceived,
				Bitrate: status.Bitrate, Reconnects: status.Reconnects, Ready: status.Ready})
		}
		return result
	})
}
