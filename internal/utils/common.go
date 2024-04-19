package utils

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/avast/retry-go"
	"github.com/google/go-tpm/tpm2"
	"github.com/google/go-tpm/tpm2/transport"
	"github.com/joho/godotenv"
	"github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/kairos-sdk/state"
)

// BootStateToLabelDevice lets us know the device we need to mount sysroot on based on labels.
func BootStateToLabelDevice() string {
	runtime, err := state.NewRuntimeWithLogger(Log)
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
// Useful if we want to move to a no-pivot boot.
func GetRootDir() string {
	cmdline, _ := os.ReadFile(GetHostProcCmdline())
	switch {
	case state.DetectUKIboot(string(cmdline)):
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

// ReadEnv will read an env file (key=value) and return a nice map.
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

// CreateIfNotExists will check if a path exists and create it if needed.
func CreateIfNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}

	return nil
}

// CleanupSlice will clean a slice of strings of empty items
// Typos can be made on writing the cos-layout.env file and that could introduce empty items
// In the lists that we need to go over, which causes bad stuff.
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

// GetTarget gets the target image and device to mount in /sysroot.
func GetTarget(dryRun bool) (string, string, error) {
	if IsUKI() {
		return "", "", nil
	}

	label := BootStateToLabelDevice()

	// If dry run, or we are disabled return whatever values, we won't go much further
	if dryRun || DisableImmucore() {
		return "fake", label, nil
	}

	imgs := CleanupSlice(ReadCMDLineArg("cos-img/filename="))

	// If no image just panic here, we cannot longer continue
	if len(imgs) == 0 {
		msg := "could not get the image name from cmdline (i.e. cos-img/filename=/cOS/active.img)"
		Log.Error().Msg(msg)
		return "", "", errors.New(msg)
	}

	Log.Debug().Str("what", imgs[0]).Msg("Target device")
	Log.Debug().Str("what", label).Msg("Target label")
	return imgs[0], label, nil
}

// DisableImmucore identifies if we need to be disabled
// We disable if we boot from CD, netboot, squashfs recovery or have the rd.cos.disable stanza in cmdline.
func DisableImmucore() bool {
	cmdline, _ := os.ReadFile(GetHostProcCmdline())
	cmdlineS := string(cmdline)

	return strings.Contains(cmdlineS, "live:LABEL") || strings.Contains(cmdlineS, "live:CDLABEL") ||
		strings.Contains(cmdlineS, "netboot") || strings.Contains(cmdlineS, "rd.cos.disable") ||
		strings.Contains(cmdlineS, "rd.immucore.disable")
}

// RootRW tells us if the mode to mount root.
func RootRW() string {
	if len(ReadCMDLineArg("rd.cos.debugrw")) > 0 || len(ReadCMDLineArg("rd.immucore.debugrw")) > 0 {
		Log.Warn().Msg("Mounting root as RW")
		return "rw"
	}
	return "ro"
}

// GetState returns the disk-by-label of the state partition to mount
// This is only valid for either active/passive or normal recovery.
func GetState() string {
	var label string

	err := retry.Do(
		func() error {
			r, err := state.NewRuntimeWithLogger(Log)
			if err != nil {
				return err
			}
			switch r.BootState {
			case state.Active, state.Passive:
				label = "COS_STATE"
			case state.Recovery:
				label = "COS_RECOVERY"
			default:
				return errors.New("could not get label")
			}
			return nil
		},
		retry.Delay(1*time.Second),
		retry.Attempts(10),
		retry.DelayType(retry.FixedDelay),
		retry.OnRetry(func(n uint, _ error) {
			Log.Debug().Uint("try", n).Msg("Cannot get state label, retrying")
		}),
	)
	if err != nil {
		Log.Panic().Err(err).Msg("Could not get state label")
	}

	Log.Debug().Str("what", label).Msg("Get state label")
	return filepath.Join("/dev/disk/by-label/", label)
}

func IsUKI() bool {
	cmdline, err := os.ReadFile(GetHostProcCmdline())
	if err != nil {
		Log.Warn().Err(err).Msg("Error reading /proc/cmdline file " + err.Error())
		return false
	}

	return state.DetectUKIboot(string(cmdline))
}

// CommandWithPath runs a command adding the usual PATH to environment
// Useful under UKI as there is nothing setting the PATH.
func CommandWithPath(c string) (string, error) {
	cmd := exec.Command("/bin/sh", "-c", c)
	cmd.Env = os.Environ()
	pathAppend := "/usr/bin:/usr/sbin:/bin:/sbin"
	// try to extract any existing path from the environment
	for _, env := range cmd.Env {
		splitted := strings.Split(env, "=")
		if splitted[0] == "PATH" {
			pathAppend = fmt.Sprintf("%s:%s", pathAppend, splitted[1])
		}
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("PATH=%s", pathAppend))
	o, err := cmd.CombinedOutput()
	return string(o), err
}

// PrepareCommandWithPath prepares a cmd with the proper env
// For running under yip.
func PrepareCommandWithPath(c string) *exec.Cmd {
	cmd := exec.Command("/bin/sh", "-c", c)
	cmd.Env = os.Environ()
	pathAppend := constants.PathAppend
	// try to extract any existing path from the environment
	for _, env := range cmd.Env {
		splitted := strings.Split(env, "=")
		if splitted[0] == constants.PATH {
			pathAppend = fmt.Sprintf("%s:%s", pathAppend, splitted[1])
		}
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", constants.PATH, pathAppend))
	return cmd
}

// GetHostProcCmdline returns the path to /proc/cmdline
// Mainly used to override the cmdline during testing.
func GetHostProcCmdline() string {
	proc := os.Getenv("HOST_PROC_CMDLINE")
	if proc == "" {
		return "/proc/cmdline"
	}
	return proc
}

func DropToEmergencyShell() {
	env := os.Environ()
	// try to extract any existing path from the environment
	pathAppend := constants.PathAppend
	for _, e := range env {
		splitted := strings.Split(e, "=")
		if splitted[0] == constants.PATH {
			pathAppend = fmt.Sprintf("%s:%s", pathAppend, splitted[1])
		}
	}
	env = append(env, fmt.Sprintf("%s=%s", constants.PATH, pathAppend))
	if err := syscall.Exec("/bin/bash", []string{"/bin/bash"}, env); err != nil {
		if err := syscall.Exec("/bin/sh", []string{"/bin/sh"}, env); err != nil {
			if err := syscall.Exec("/sysroot/bin/bash", []string{"/sysroot/bin/bash"}, env); err != nil {
				if err := syscall.Exec("/sysroot/bin/sh", []string{"/sysroot/bin/sh"}, env); err != nil {
					Log.Fatal().Msg("Could not drop to emergency shell")
				}
			}
		}
	}
}

// PCRExtend extends the given pcr with the give data.
func PCRExtend(pcr int, data []byte) error {
	t, err := transport.OpenTPM()
	if err != nil {
		return err
	}
	defer func(t transport.TPMCloser) {
		_ = t.Close()
	}(t)
	digest := sha256.Sum256(data)
	pcrHandle := tpm2.PCRExtend{
		PCRHandle: tpm2.AuthHandle{
			Handle: tpm2.TPMHandle(pcr),
			Auth:   tpm2.PasswordAuth(nil),
		},
		Digests: tpm2.TPMLDigestValues{
			Digests: []tpm2.TPMTHA{
				{
					HashAlg: tpm2.TPMAlgSHA256,
					Digest:  digest[:],
				},
			},
		},
	}

	if _, err = pcrHandle.Execute(t); err != nil {
		return err
	}

	return nil
}
