//go:build !linux

package media

func readProcessUsage(_ int) (processSample, uint64, bool) {
	return processSample{}, 0, false
}
