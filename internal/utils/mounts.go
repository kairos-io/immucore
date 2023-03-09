package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/kairos/sdk/state"
)

// https://github.com/kairos-io/packages/blob/7c3581a8ba6371e5ce10c3a98bae54fde6a505af/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L58

// ParseMount will return a proper full disk path based on UUID or LABEL given
// input: LABEL=FOO:/mount
// output: /dev/disk...:/mount .
func ParseMount(s string) string {
	switch {
	case strings.Contains(s, "UUID="):
		dat := strings.Split(s, "UUID=")
		return fmt.Sprintf("/dev/disk/by-uuid/%s", dat[1])
	case strings.Contains(s, "LABEL="):
		dat := strings.Split(s, "LABEL=")
		return fmt.Sprintf("/dev/disk/by-label/%s", dat[1])
	default:
		return s
	}
}

// ReadCMDLineArg will return the pair of arg=value for a given arg if it was passed on the cmdline
// TODO: Split this into GetBool and GetValue to return decent defaults.
func ReadCMDLineArg(arg string) []string {
	cmdLine, err := os.ReadFile(GetHostProcCmdline())
	if err != nil {
		return []string{}
	}
	res := []string{}
	fields := strings.Fields(string(cmdLine))
	for _, f := range fields {
		if strings.HasPrefix(f, arg) {
			dat := strings.Split(f, arg)
			// For stanzas that have no value, we should return something better than an empty value
			// Otherwise anything can easily clean the value
			if dat[1] == "" {
				res = append(res, "")
			} else {
				res = append(res, dat[1])
			}
		}
	}
	return res
}

// IsMounted lets us know if the given device is currently mounted.
func IsMounted(dev string) bool {
	_, err := CommandWithPath(fmt.Sprintf("findmnt %s", dev))
	return err == nil
}

// DiskFSType will return the FS type for a given disk
// Does NOT need to be mounted
// Needs full path so either /dev/sda1 or /dev/disk/by-{label,uuid}/{label,uuid} .
func DiskFSType(s string) string {
	out, e := CommandWithPath(fmt.Sprintf("blkid %s -s TYPE -o value", s))
	if e != nil {
		Log.Err(e).Msg("blkid")
	}
	out = strings.Trim(strings.Trim(out, " "), "\n")
	Log.Debug().Str("what", s).Str("type", out).Msg("Partition FS type")
	return out
}

// SyncState will rsync source into destination. Useful for Bind mounts.
func SyncState(src, dst string) error {
	_, err := CommandWithPath(fmt.Sprintf("rsync -aqAX %s %s", src, dst))
	return err
}

// AppendSlash it's in the name. Appends a slash.
func AppendSlash(path string) string {
	if !strings.HasSuffix(path, "/") {
		return fmt.Sprintf("%s/", path)
	}
	return path
}

// MountToFstab transforms a mount.Mount into a fstab.Mount so we can transform existing mounts into the fstab format.
func MountToFstab(m mount.Mount) *fstab.Mount {
	opts := map[string]string{}
	for _, o := range m.Options {
		if strings.Contains(o, "=") {
			dat := strings.Split(o, "=")
			key := dat[0]
			value := dat[1]
			opts[key] = value
		} else {
			opts[o] = ""
		}
	}
	return &fstab.Mount{
		Spec:    m.Source,
		VfsType: m.Type,
		MntOps:  opts,
		Freq:    0,
		PassNo:  0,
	}
}

// CleanSysrootForFstab will clean up the pesky sysroot dir from entries to make them
// suitable to be written in the fstab
// As we mount on /sysroot during initramfs but the fstab file is for the real init process, we need to remove
// Any mentions to /sysroot from the fstab lines, otherwise they won't work
// Special care for the root (/sysroot) path as we can't just simple remove that path and call it a day
// as that will return an empty mountpoint which will break fstab mounting.
func CleanSysrootForFstab(path string) string {
	if IsUKI() {
		return path
	}
	cleaned := strings.ReplaceAll(path, "/sysroot", "")
	if cleaned == "" {
		cleaned = "/"
	}
	return cleaned
}

// Fsck will run fsck over the device
// options are set on cmdline, but they are for systemd-fsck,
// so we need to interpret ourselves.
func Fsck(device string) error {
	if device == "tmpfs" {
		return nil
	}
	mode := CleanupSlice(ReadCMDLineArg("fsck.mode="))
	repair := CleanupSlice(ReadCMDLineArg("fsck.repair="))
	// Be safe with defaults
	if len(mode) == 0 {
		mode = []string{"auto"}
	}
	if len(repair) == 0 {
		repair = []string{"preen"}
	}
	args := []string{"fsck", device}
	// Check the mode
	// skip means just skip the fsck
	// force means force even if fs is deemed clean
	// auto or others means normal fsck call
	switch mode[0] {
	case "skip":
		return nil
	case "force":
		args = append(args, "-f")
	}

	// Check repair type
	// preen means try to fix automatically
	// yes means say yes to everything (potentially destructive)
	// no means say no to everything
	switch repair[0] {
	case "preen":
		args = append(args, "-a")
	case "yes":
		args = append(args, "-y")
	case "no":
		args = append(args, "-n")
	}
	cmd := strings.Join(args, " ")
	Log.Debug().Str("cmd", cmd).Msg("fsck command")
	out, e := CommandWithPath(cmd)
	if e != nil {
		Log.Debug().Err(e).Str("out", out).Str("what", device).Msg("fsck")
	}
	return e
}

// MountProc will mount /proc
// For now proc is needed to read the cmdline fully in uki mode
// in normal modes this should already be done by the initramfs process, so we can skip this.
func MountProc() {
	_ = os.MkdirAll("/proc", 0755)
	if !IsMounted("/proc") {
		_ = syscall.Mount("proc", "/proc", "proc", syscall.MS_NOSUID|syscall.MS_NODEV|syscall.MS_NOEXEC|syscall.MS_RELATIME, "")
	}

}

// GetOemTimeout parses the cmdline to get the oem timeout to use. Defaults to 5 (converted into seconds afterwards).
func GetOemTimeout() int {
	var time []string

	// Pick both stanzas until we deprecate the cos ones
	timeCos := CleanupSlice(ReadCMDLineArg("rd.cos.oemtimeout="))
	timeImmucore := CleanupSlice(ReadCMDLineArg("rd.immucore.oemtimeout="))

	if len(timeCos) != 0 {
		time = timeCos
	}
	if len(timeImmucore) != 0 {
		time = timeImmucore
	}

	if len(time) == 0 {
		return 5
	}
	converted, err := strconv.Atoi(time[0])
	if err != nil {
		return 5
	}
	return converted
}

// GetOverlayBase parses the cdmline and gets the overlay config
// Format is rd.cos.overlay=tmpfs:20% or rd.cos.overlay=LABEL=$LABEL or rd.cos.overlay=UUID=$UUID
// Notice that this can be later override by the config coming from cos-layout.env .
func GetOverlayBase() string {
	var overlayConfig []string

	// Pick both stanzas until we deprecate the cos ones
	// Clean up the slice in case the values are empty
	overlayConfigCos := CleanupSlice(ReadCMDLineArg("rd.cos.overlay="))
	overlayConfigImmucore := CleanupSlice(ReadCMDLineArg("rd.immucore.overlay="))

	if len(overlayConfigCos) != 0 {
		overlayConfig = overlayConfigCos
	}
	if len(overlayConfigImmucore) != 0 {
		overlayConfig = overlayConfigImmucore
	}

	if len(overlayConfig) == 0 {
		return "tmpfs:20%"
	}

	return overlayConfig[0]

}

// GetOemLabel will ge the oem label to mount, first from the cmdline and if that fails, from the runtime
// This way users can override the oem label.
func GetOemLabel() string {
	var oemLabel string
	// Pick both stanzas until we deprecate the cos ones
	oemLabelCos := CleanupSlice(ReadCMDLineArg("rd.cos.oemlabel="))
	oemLabelImmucore := CleanupSlice(ReadCMDLineArg("rd.immucore.oemlabel="))
	if len(oemLabelCos) != 0 {
		oemLabel = oemLabelCos[0]
	}
	if len(oemLabelImmucore) != 0 {
		oemLabel = oemLabelImmucore[0]
	}

	if oemLabel != "" {
		return oemLabel
	}
	// We could not get it from the cmdline so get it from the runtime
	runtime, err := state.NewRuntime()
	if err != nil {
		Log.Debug().Err(err).Msg("runtime")
		return ""
	}
	return runtime.OEM.Label
}
