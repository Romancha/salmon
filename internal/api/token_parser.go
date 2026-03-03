package api

import (
	"fmt"
	"strings"
)

// ParseConsumerTokens parses a comma-separated string of "name:token" pairs
// into a map of consumer name to token. The expected format is:
//
//	"app1:secret1,app2:secret2,app3:secret3"
func ParseConsumerTokens(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("no consumer tokens configured")
	}

	entries := strings.Split(raw, ",")
	result := make(map[string]string, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)

		colonIdx := strings.Index(entry, ":")
		if colonIdx < 0 {
			return nil, fmt.Errorf("invalid consumer token entry %q: expected format \"name:token\"", entry)
		}

		name := strings.TrimSpace(entry[:colonIdx])
		token := strings.TrimSpace(entry[colonIdx+1:])

		if name == "" || token == "" {
			return nil, fmt.Errorf("invalid consumer token entry %q: name and token must not be empty", entry)
		}

		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate consumer name %q", name)
		}

		result[name] = token
	}

	return result, nil
}
