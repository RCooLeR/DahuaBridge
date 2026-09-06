package media

import "time"

type processSample struct {
	cpuSeconds float64
	started    string
	at         time.Time
}

func (m *Manager) enrichProcessStatuses(statuses []WorkerStatus) {
	m.processMu.Lock()
	defer m.processMu.Unlock()
	next := make(map[int]processSample)
	for index := range statuses {
		status := &statuses[index]
		if status.ProcessID == 0 || status.State == "retained" {
			continue
		}
		sample, memory, ok := readProcessUsage(status.ProcessID)
		if !ok {
			status.ProcessID = 0
			continue
		}
		status.ResidentMemoryBytes = memory
		// A reused PID starts a new sample series. Leave the first CPU value
		// unavailable rather than reporting a misleading zero or lifetime average.
		if previous, exists := m.processSamples[status.ProcessID]; exists && sample.started == previous.started && sample.cpuSeconds >= previous.cpuSeconds {
			if elapsed := sample.at.Sub(previous.at).Seconds(); elapsed > 0 {
				cpu := 100 * (sample.cpuSeconds - previous.cpuSeconds) / elapsed
				status.CPUPercent = &cpu
			}
		}
		next[status.ProcessID] = sample
	}
	m.processSamples = next
}
