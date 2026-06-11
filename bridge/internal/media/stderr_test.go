package media

import "testing"

func TestTailBufferKeepsOnlyNewestBytes(t *testing.T) {
	buffer := newTailBuffer(5)
	if _, err := buffer.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := buffer.Write([]byte("defgh")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if got := buffer.String(); got != "defgh" {
		t.Fatalf("unexpected tail %q", got)
	}
}

func TestTailBufferRedactsFFmpegCredentials(t *testing.T) {
	buffer := newTailBuffer(512)
	_, _ = buffer.Write([]byte("rtsp://assistant:secret@192.0.2.10/live?auth_token=abc failed"))

	got := buffer.String()
	if got != "rtsp://[redacted]@192.0.2.10/live?auth_token=[redacted] failed" {
		t.Fatalf("unexpected redacted stderr %q", got)
	}
}
