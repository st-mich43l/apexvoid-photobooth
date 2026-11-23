package main

import (
	"strconv"
	"strings"
)

// validRuntimeVersion follows Microsoft's documented nonzero pv check.
func validRuntimeVersion(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 4 {
		return false
	}
	nonzero := false
	for _, part := range parts {
		if part == "" {
			return false
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return false
		}
		if n != 0 {
			nonzero = true
		}
	}
	return nonzero
}
