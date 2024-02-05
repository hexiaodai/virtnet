package types

import (
	"fmt"
)

// generateHostVethName select the first 11 characters of the containerID for the host veth.
func GenerateHostVethName(containerID string) string {
	return fmt.Sprintf("veth%s", containerID[:min(len(containerID))])
}

func min(len int) int {
	if len > 11 {
		return 11
	}
	return len
}
