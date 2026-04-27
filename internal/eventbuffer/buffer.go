package eventbuffer

import (
	"sort"
	"strings"
	"sync"

	"RCooLeR/DahuaBridge/internal/dahua"
)

const DefaultCapacity = 512

type Buffer struct {
	mu       sync.RWMutex
	events   []dahua.Event
	next     int
	count    int
	capacity int
}

type Filter struct {
	DeviceID   string
	ChildID    string
	DeviceKind dahua.DeviceKind
	Code       string
	Action     string
	Limit      int
}

func New(capacity int) *Buffer {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}

	return &Buffer{
		events:   make([]dahua.Event, capacity),
		capacity: capacity,
	}
}

func (b *Buffer) Add(event dahua.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.events[b.next] = cloneEvent(event)
	b.next = (b.next + 1) % b.capacity
	if b.count < b.capacity {
		b.count++
	}
}

func (b *Buffer) List(filter Filter) []dahua.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return nil
	}

	limit := filter.Limit
	if limit <= 0 || limit > b.count {
		limit = b.count
	}

	result := make([]dahua.Event, 0, limit)
	deviceID := strings.TrimSpace(filter.DeviceID)
	childID := strings.TrimSpace(filter.ChildID)
	deviceKind := dahua.DeviceKind(strings.TrimSpace(string(filter.DeviceKind)))
	code := strings.ToLower(strings.TrimSpace(filter.Code))
	action := strings.ToLower(strings.TrimSpace(filter.Action))

	for i := 0; i < b.count; i++ {
		index := (b.next - 1 - i + b.capacity) % b.capacity
		event := b.events[index]
		if deviceID != "" && event.DeviceID != deviceID {
			continue
		}
		if childID != "" && event.ChildID != childID {
			continue
		}
		if deviceKind != "" && event.DeviceKind != deviceKind {
			continue
		}
		if code != "" && strings.ToLower(strings.TrimSpace(event.Code)) != code {
			continue
		}
		if action != "" && strings.ToLower(strings.TrimSpace(string(event.Action))) != action {
			continue
		}

		result = append(result, cloneEvent(event))
		if len(result) >= limit {
			break
		}
	}

	return result
}

func (b *Buffer) ListEvents(deviceID string, childID string, deviceKind dahua.DeviceKind, code string, action string, limit int) []dahua.Event {
	return b.List(Filter{
		DeviceID:   deviceID,
		ChildID:    childID,
		DeviceKind: deviceKind,
		Code:       code,
		Action:     action,
		Limit:      limit,
	})
}

func (b *Buffer) Clear() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	removed := b.count
	b.events = make([]dahua.Event, b.capacity)
	b.next = 0
	b.count = 0
	return removed
}

func (b *Buffer) Stats() map[string]any {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return map[string]any{
		"capacity": b.capacity,
		"count":    b.count,
	}
}

func (b *Buffer) EventStats() map[string]any {
	return b.Stats()
}

func (b *Buffer) ClearEvents() int {
	return b.Clear()
}

func cloneEvent(input dahua.Event) dahua.Event {
	output := input
	if len(input.Data) > 0 {
		output.Data = make(map[string]string, len(input.Data))
		keys := make([]string, 0, len(input.Data))
		for key := range input.Data {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			output.Data[key] = input.Data[key]
		}
	}
	return output
}
