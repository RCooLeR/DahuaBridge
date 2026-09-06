//go:build linux

package media

import (
	"strings"
	"testing"
	"time"
)

func TestProcessUsageParsesNamesWithSpacesAndParentheses(t *testing.T) {
	fields := strings.Fields("S 1 2 3 4 5 6 7 8 9 10 250 50 0 0 0 0 0 0 12345 0 12")
	sample, memory, ok := parseProcessUsage("99 (ffmpeg (worker)) "+strings.Join(fields, " "), 100, 4096, time.Now())
	if !ok || sample.cpuSeconds != 3 || sample.started != "12345" || memory != 12*4096 {
		t.Fatalf("wrong usage: %+v %d %v", sample, memory, ok)
	}
	if _, _, ok := parseProcessUsage("malformed", 100, 4096, time.Now()); ok {
		t.Fatal("accepted malformed process stat")
	}
}
