//go:build linux

package media

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

var processClockTicks = sync.OnceValue(func() float64 {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "getconf", "CLK_TCK").Output()
	if err != nil {
		return 0
	}
	ticks, _ := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	return ticks
})

func readProcessUsage(pid int) (processSample, uint64, bool) {
	body, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return processSample{}, 0, false
	}
	return parseProcessUsage(string(body), processClockTicks(), uint64(os.Getpagesize()), time.Now())
}

func parseProcessUsage(body string, ticks float64, pageSize uint64, now time.Time) (processSample, uint64, bool) {
	// The process name in parentheses may itself contain spaces or parentheses.
	end := strings.LastIndexByte(body, ')')
	if end < 0 || ticks <= 0 {
		return processSample{}, 0, false
	}
	fields := strings.Fields(body[end+1:])
	// fields[0] is proc stat field 3 (state): utime/stime are 14/15,
	// starttime is 22, and RSS is 24. RSS is in pages; CPU times are ticks.
	if len(fields) < 22 {
		return processSample{}, 0, false
	}
	user, err1 := strconv.ParseFloat(fields[11], 64)
	kernel, err2 := strconv.ParseFloat(fields[12], 64)
	resident, err3 := strconv.ParseUint(fields[21], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return processSample{}, 0, false
	}
	return processSample{cpuSeconds: (user + kernel) / ticks, started: fields[19], at: now}, resident * pageSize, true
}
