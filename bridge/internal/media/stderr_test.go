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
