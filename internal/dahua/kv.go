package dahua

import "strings"

func ParseKeyValueBody(body string) map[string]string {
	result := make(map[string]string)

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return result
}
