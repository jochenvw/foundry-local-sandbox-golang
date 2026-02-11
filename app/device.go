package app

import (
	"fmt"
	"strings"

	"github.com/microsoft/foundry-local/sdk/go/foundrylocal"
)

// ParseDeviceChoice converts a string into a foundry device type.
func ParseDeviceChoice(choice string) (foundrylocal.DeviceType, bool, error) {
	switch strings.ToLower(strings.TrimSpace(choice)) {
	case "", "auto":
		return "", false, nil
	case "cpu":
		return foundrylocal.CPU, true, nil
	case "gpu":
		return foundrylocal.GPU, true, nil
	case "npu":
		return foundrylocal.NPU, true, nil
	default:
		return "", false, fmt.Errorf("invalid device %q (expected auto|cpu|gpu|npu)", choice)
	}
}
