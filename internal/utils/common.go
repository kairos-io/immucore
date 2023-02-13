package utils

import (
	"github.com/joho/godotenv"
	"github.com/kairos-io/kairos/sdk/state"
	"os"
	"path/filepath"
	"strings"
)

// BootedFromCD tells us if we are currently runnig of the LiveCD
func BootedFromCD() (bool, error) {
	runtime, err := state.NewRuntime()
	if err != nil {
		return false, err
	}

	return runtime.BootState == state.LiveCD, nil
}

// BootStateToLabel lets us know the label we need to mount sysroot on
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
		return "COS_SYSTEM"
	default:
		return ""
	}
}

// IsRecovery lets us know if we are in the recovery
func IsRecovery() bool {
	runtime, err := state.NewRuntime()
	if err != nil {
		return false
	}
	switch runtime.BootState {
	case "recovery_boot":
		return true
	default:
		return false
	}
}

// GetRootDir returns the proper dir to mount all the stuff
// Useful if we want to move to a no-pivot boot
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

// UniqueSlice removes duplicated entries from a slice.So dumb. Like really? Why not have a set which enforces uniqueness????
func UniqueSlice(slice []string) []string {
	keys := make(map[string]bool)
	var list []string
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

// ReadEnv will reaed an env file (key=value) and return a nice map
func ReadEnv(file string) (map[string]string, error) {
	var envMap map[string]string
	var err error

	f, err := os.Open(file)
	if err != nil {
		return envMap, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)

	envMap, err = godotenv.Parse(f)
	if err != nil {
		return envMap, err
	}

	return envMap, err
}

// CreateIfNotExists will check if a path exists and create it if needed
func CreateIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}

	return nil
}

// CleanupSlice will clean a slice of strings of empty items
// Typos can be made on writing the cos-layout.env file and that could introduce empty items
// In the lists that we need to go over, which causes bad stuff
func CleanupSlice(slice []string) []string {
	var cleanSlice []string
	for _, item := range slice {
		if strings.Trim(item, " ") == "" {
			continue
		}
		cleanSlice = append(cleanSlice, item)
	}
	return cleanSlice
}

// SetSentinelFile sets the sentinel file to identify the boot mode.
// This is used by several things to know in which state they are, for example cloud configs
func SetSentinelFile() error {
	var sentinel string

	err := CreateIfNotExists("/run/cos/")
	if err != nil {
		return err
	}
	runtime, err := state.NewRuntime()
	if err != nil {
		return err
	}

	switch runtime.BootState {
	case state.Active:
		sentinel = "active_mode"
	case state.Passive:
		sentinel = "passive_mode"
	case state.Recovery:
		sentinel = "recovery_mode"
	case state.LiveCD:
		sentinel = "live_mode"
	default:
		sentinel = string(state.Unknown)
	}
	err = os.WriteFile(filepath.Join("/run/cos/", sentinel), []byte("1"), os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}