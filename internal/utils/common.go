package utils

import (
	"github.com/joho/godotenv"
	"github.com/kairos-io/kairos/sdk/state"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"strings"
)

// BootStateToLabelDevice lets us know the device we need to mount sysroot on based on labels
func BootStateToLabelDevice() string {
	runtime, err := state.NewRuntime()
	if err != nil {
		return ""
	}
	switch runtime.BootState {
	case state.Active:
		return filepath.Join("/dev/disk/by-label", "COS_ACTIVE")
	case state.Passive:
		return filepath.Join("/dev/disk/by-label", "COS_PASSIVE")
	case state.Recovery:
		return filepath.Join("/dev/disk/by-label", "COS_SYSTEM")
	default:
		return ""
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

// GetTarget gets the target image and device to mount in /sysroot
func GetTarget(dryRun bool) (string, string) {
	var img, label string

	label = BootStateToLabelDevice()

	// If dry run, or we are disabled return whatever values, we won't go much further
	if dryRun || DisableImmucore() {
		return "fake", label
	}

	img = ReadCMDLineArg("cos-img/filename=")[0]

	// If no image just panic here, we cannot longer continue
	if img == "" {
		log.Logger.Fatal().Msg("Could not get the image name from cmdline (i.e. cos-img/filename=/cOS/active.img)")
	}

	log.Debug().Str("what", img).Msg("Target device")
	log.Debug().Str("what", label).Msg("Target label")
	return img, label
}

// DisableImmucore identifies if we need to be disabled
// We disable if we boot from CD, netboot, squashfs recovery or have the rd.cos.disable stanza in cmdline
func DisableImmucore() bool {
	cmdline, _ := os.ReadFile("/proc/cmdline")
	cmdlineS := string(cmdline)

	return strings.Contains(cmdlineS, "live:LABEL") || strings.Contains(cmdlineS, "live:CDLABEL") || strings.Contains(cmdlineS, "netboot") || strings.Contains(cmdlineS, "rd.cos.disable")
}

// RootRW tells us if the mode to mount root
func RootRW() string {
	if len(ReadCMDLineArg("rd.cos.debugrw")) > 0 {
		log.Logger.Warn().Msg("Mounting root as RW")
		return "rw"
	}
	return "ro"
}

// GetState returns the disk-by-label of the state partition to mount
// This is only valid for either active/passive or normal recovery
func GetState() string {
	var label string
	runtime, err := state.NewRuntime()
	if err != nil {
		return label
	}
	switch runtime.BootState {
	case state.Active, state.Passive:
		label = filepath.Join("/dev/disk/by-label/", runtime.State.Label)
	case state.Recovery:
		label = filepath.Join("/dev/disk/by-label/", runtime.Recovery.Label)
	}
	log.Logger.Debug().Str("what", label).Msg("Get state label")
	return label
}
