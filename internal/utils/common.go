package utils

import (
	"github.com/kairos-io/kairos/sdk/state"
	"os"
	"strings"
)

func BootedFromCD() (bool, error) {
	runtime, err := state.NewRuntime()
	if err != nil {
		return false, err
	}

	return runtime.BootState == state.LiveCD, nil
}

func BootStateToLabel() string {
	runtime, err := state.NewRuntime()
	if err != nil {
		return ""
	}
	switch runtime.BootState {
	case "active_boot":
		return "COS_ACTIVE"
	case "passive_boot":
		return "COS_PASSIVE"
	case "recovery_boot":
		return "COS_RECOVERY"
	default:
		return ""
	}
}

func BootStateToImage() string {
	runtime, err := state.NewRuntime()
	if err != nil {
		return ""
	}
	switch runtime.BootState {
	case "active_boot":
		return "/cOS/active.img"
	case "passive_boot":
		return "/cOS/passive.img"
	case "recovery_boot":
		return "/cOS/recovery.img"
	default:
		return ""
	}
}

func GetRootDir() string {
	cmdline, _ := os.ReadFile("/proc/cmdline")
	switch {
	case strings.Contains(string(cmdline), "IMMUCORE_NOPIVOT"):
		return "/"
	default:
		// Default is sysroot for normal no-pivot boot
		return "/sysroot"
	}
}
