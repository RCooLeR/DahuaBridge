package media

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"RCooLeR/DahuaBridge/internal/config"
	"RCooLeR/DahuaBridge/internal/streams"

	"github.com/rs/zerolog"
)

func safeHLSDirectoryPrefix(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "stream"
	}
	replacer := strings.NewReplacer(":", "_", "/", "_", "\\", "_", " ", "_")
	safe := replacer.Replace(trimmed)
	if len(safe) > 96 {
		safe = safe[:96]
	}
	return safe
}

func hasIncompleteMJPEGFrame(buffer []byte) bool {
	start := bytes.Index(buffer, []byte{0xFF, 0xD8})
	if start < 0 {
		return false
	}
	end := bytes.Index(buffer[start+2:], []byte{0xFF, 0xD9})
	return end < 0
}

func removeDirectoryAfter(logger zerolog.Logger, dir string, delay time.Duration) {
	if strings.TrimSpace(dir) == "" {
		return
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	<-timer.C
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn().Err(err).Str("dir", dir).Msg("failed to remove expired hls cache directory")
	}
}

func redactURLUserinfo(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func resolvedScaleWidth(requested int, configured int) int {
	if configured <= 0 {
		return 0
	}
	if requested > 0 {
		return requested
	}
	return configured
}

func buildFFmpegStartAttempts(cfg config.MediaConfig) []ffmpegStartAttempt {
	attempts := make([]ffmpegStartAttempt, 0, 3)
	if hardwareAccelEnabled(cfg.HWAccelArgs) {
		attempts = append(attempts, ffmpegStartAttempt{
			useHWAccel:  true,
			inputPreset: cfg.InputPreset,
		})
	}
	attempts = append(attempts, ffmpegStartAttempt{
		useHWAccel:  false,
		inputPreset: cfg.InputPreset,
	})
	if !strings.EqualFold(strings.TrimSpace(cfg.InputPreset), "stable") {
		attempts = append(attempts, ffmpegStartAttempt{
			useHWAccel:  false,
			inputPreset: "stable",
		})
	}
	return attempts
}

func buildRTSPInputArgs(profile streams.Profile, inputPreset string) []string {
	return buildRTSPInputArgsWithWallclock(profile, inputPreset, profile.UseWallclockAsTimestamps)
}

func buildRTSPInputArgsWithWallclock(profile streams.Profile, inputPreset string, useWallclockTimestamps bool) []string {
	args := []string{
		"-rtsp_transport", firstNonEmpty(profile.RTSPTransport, "tcp"),
	}
	fflags := []string{"discardcorrupt", "genpts"}
	switch strings.ToLower(strings.TrimSpace(inputPreset)) {
	case "stable":
	default:
		fflags = append(fflags, "nobuffer")
		args = append(args, "-flags", "low_delay")
	}
	args = append(args, "-fflags", "+"+strings.Join(fflags, "+"))
	if useWallclockTimestamps {
		args = append(args, "-use_wallclock_as_timestamps", "1")
	}
	args = append(args, "-i", profile.StreamURL)
	return args
}

func buildFilterChain(frameRate int, scaleWidth int, sourceWidth int, sourceHeight int) []string {
	filters := []string{"fps=" + strconv.Itoa(frameRate)}
	if scaleWidth <= 0 {
		return filters
	}
	if targetWidth, targetHeight, ok := computeScaledDimensions(sourceWidth, sourceHeight, scaleWidth); ok {
		return append(filters, fmt.Sprintf("scale=%d:%d", targetWidth, targetHeight))
	}
	if sourceWidth <= 0 || sourceHeight <= 0 {
		widthExpr := normalizeScaleWidth(scaleWidth)
		if widthExpr > 0 {
			return append(filters, fmt.Sprintf("scale='min(%d,iw)':-2", widthExpr))
		}
	}
	return filters
}

func buildQSVFilterChain(frameRate int, scaleWidth int, sourceWidth int, sourceHeight int) []string {
	options := []string{fmt.Sprintf("framerate=%d", frameRate)}
	if targetWidth, targetHeight, ok := computeScaledDimensions(sourceWidth, sourceHeight, scaleWidth); ok {
		options = append(options,
			fmt.Sprintf("w=%d", targetWidth),
			fmt.Sprintf("h=%d", targetHeight),
		)
	}
	options = append(options, "format=nv12")
	return []string{"vpp_qsv=" + strings.Join(options, ":")}
}

func computeScaledDimensions(sourceWidth int, sourceHeight int, targetWidth int) (int, int, bool) {
	if sourceWidth <= 0 || sourceHeight <= 0 || targetWidth <= 0 || sourceWidth <= targetWidth {
		return 0, 0, false
	}

	targetWidth = normalizeScaleWidth(targetWidth)
	if targetWidth <= 0 {
		return 0, 0, false
	}

	targetHeight := (sourceHeight * targetWidth) / sourceWidth
	if targetHeight <= 0 {
		return 0, 0, false
	}
	if targetHeight%2 != 0 {
		targetHeight--
	}
	if targetHeight <= 0 {
		return 0, 0, false
	}
	return targetWidth, targetHeight, true
}

func normalizeScaleWidth(width int) int {
	if width <= 0 {
		return 0
	}
	if width%2 != 0 {
		width--
	}
	return maxInt(width, 0)
}

func validateHLSFileName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("invalid hls asset name")
	}
	if name != filepath.Base(name) || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) {
		return errors.New("invalid hls asset name")
	}
	return nil
}

func contentTypeForHLSFile(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	default:
		return "application/octet-stream"
	}
}

func formatFFmpegSeconds(duration time.Duration) string {
	seconds := duration.Seconds()
	formatted := strconv.FormatFloat(seconds, 'f', 3, 64)
	formatted = strings.TrimRight(formatted, "0")
	formatted = strings.TrimRight(formatted, ".")
	if formatted == "" {
		return "1"
	}
	return formatted
}

func playbackDurationFromStreamURL(raw string) (time.Duration, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}

	query := parsed.Query()
	startRaw := strings.TrimSpace(query.Get("starttime"))
	endRaw := strings.TrimSpace(query.Get("endtime"))
	if startRaw == "" || endRaw == "" {
		return 0, false
	}

	startTime, err := time.Parse("2006_01_02_15_04_05", startRaw)
	if err != nil {
		return 0, false
	}
	endTime, err := time.Parse("2006_01_02_15_04_05", endRaw)
	if err != nil || !endTime.After(startTime) {
		return 0, false
	}
	return endTime.Sub(startTime), true
}

func ffmpegLogLevel(cfg config.MediaConfig) string {
	level := strings.ToLower(strings.TrimSpace(cfg.FFmpegLogLevel))
	switch level {
	case "quiet", "panic", "fatal", "error", "warning", "info", "verbose", "debug", "trace":
		return level
	default:
		return "error"
	}
}

func hardwareAccelEnabled(args []string) bool {
	return len(args) > 0
}

func appendInputHWAccelArgs(args []string, cfg config.MediaConfig, useHWAccel bool) []string {
	if !useHWAccel {
		return args
	}

	args = append(args, cfg.HWAccelArgs...)
	if qsvHWAccelConfigured(cfg.HWAccelArgs) && !containsArg(cfg.HWAccelArgs, "-hwaccel_output_format") {
		args = append(args, "-hwaccel_output_format", "qsv")
	}
	return args
}

func qsvHWAccelConfigured(args []string) bool {
	for _, arg := range args {
		if strings.Contains(strings.ToLower(strings.TrimSpace(arg)), "qsv") {
			return true
		}
	}
	return false
}

func containsArg(args []string, target string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), target) {
			return true
		}
	}
	return false
}

func useQSVVideoEncoder(cfg config.MediaConfig, useHWAccel bool) bool {
	return useHWAccel &&
		hardwareAccelEnabled(cfg.HWAccelArgs) &&
		strings.EqualFold(strings.TrimSpace(cfg.VideoEncoder), "qsv")
}

func appendVideoFilterArgs(args []string, cfg config.MediaConfig, scaleWidth int, profile streams.Profile, useHWAccel bool, frameRate int) []string {
	var filterChain []string
	if useQSVVideoEncoder(cfg, useHWAccel) {
		filterChain = buildQSVFilterChain(frameRate, scaleWidth, profile.SourceWidth, profile.SourceHeight)
	} else {
		filterChain = buildFilterChain(frameRate, scaleWidth, profile.SourceWidth, profile.SourceHeight)
	}
	if len(filterChain) == 0 {
		return args
	}
	return append(args, "-vf", strings.Join(filterChain, ","))
}

func appendVideoEncoderArgs(args []string, cfg config.MediaConfig, useHWAccel bool, gopSize int, softwarePreset string) []string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return append(args,
			"-c:v", "h264_qsv",
			"-pix_fmt", "nv12",
			"-profile:v", "baseline",
			"-g", strconv.Itoa(gopSize),
			"-keyint_min", strconv.Itoa(gopSize),
			"-sc_threshold", "0",
			"-bf", "0",
		)
	}

	return append(args,
		"-c:v", "libx264",
		"-preset", softwarePreset,
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-profile:v", "baseline",
		"-g", strconv.Itoa(gopSize),
		"-keyint_min", strconv.Itoa(gopSize),
		"-sc_threshold", "0",
		"-bf", "0",
	)
}

func appendMJPEGEncoderArgs(args []string, cfg config.MediaConfig, useHWAccel bool) []string {
	if useQSVVideoEncoder(cfg, useHWAccel) {
		return append(args,
			"-c:v", "mjpeg_qsv",
			"-global_quality", strconv.Itoa(mapSoftwareJPEGQualityToQSV(cfg.JPEGQuality)),
		)
	}

	return append(args,
		"-q:v", strconv.Itoa(cfg.JPEGQuality),
	)
}

func mapSoftwareJPEGQualityToQSV(jpegQuality int) int {
	if jpegQuality <= 0 {
		return 80
	}
	quality := 100 - ((jpegQuality - 1) * 5)
	if quality < 1 {
		return 1
	}
	if quality > 100 {
		return 100
	}
	return quality
}

func isHardwareAccelFailure(stderrText string) bool {
	text := strings.ToLower(strings.TrimSpace(stderrText))
	if text == "" {
		return false
	}
	return strings.Contains(text, "device creation failed") ||
		strings.Contains(text, "hardware device setup failed") ||
		strings.Contains(text, "no device available for decoder") ||
		strings.Contains(text, "hevc_qsv") ||
		strings.Contains(text, "h264_qsv")
}

func isOptionalAudioOutputFailure(stderrText string) bool {
	text := strings.ToLower(strings.TrimSpace(stderrText))
	if text == "" {
		return false
	}
	return strings.Contains(text, "stream map '?' matches no streams; ignoring") &&
		strings.Contains(text, "output file does not contain any stream")
}
