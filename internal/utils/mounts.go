package utils

import (
	"fmt"
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"os"
	"strings"
	"syscall"
)

// https://github.com/kairos-io/packages/blob/7c3581a8ba6371e5ce10c3a98bae54fde6a505af/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-mount-layout.sh#L58

// ParseMount will return a proper full disk path based on UUID or LABEL given
// input: LABEL=FOO:/mount
// output: /dev/disk...:/mount
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
func ReadCMDLineArg(arg string) []string {
	cmdLine, err := os.ReadFile("/proc/cmdline")
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
				res = append(res, "true")
			} else {
				res = append(res, dat[1])
			}
		}
	}
	return res
}

// IsMounted lets us know if the given device is currently mounted
func IsMounted(dev string) bool {
	_, err := CommandWithPath(fmt.Sprintf("findmnt %s", dev))
	return err == nil
}

// DiskFSType will return the FS type for a given disk
// Does NOT need to be mounted
// Needs full path so either /dev/sda1 or /dev/disk/by-{label,uuid}/{label,uuid}
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

// MountToFstab transforms a mount.Mount into a fstab.Mount so we can transform existing mounts into the fstab format
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
// as that will return an empty mountpoint which will break fstab mounting
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
// so we need to interpret ourselves
func Fsck(device string) error {
	if device == "tmpfs" {
		return nil
	}
	mode := ReadCMDLineArg("fsck.mode=")
	repair := ReadCMDLineArg("fsck.repair=")
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
	Log.Debug().Str("output", out).Msg("fsck output")
	if e != nil {
		Log.Warn().Str("error", e.Error()).Str("what", device).Msg("fsck")
	}
	return e
}

// MinimalMounts will set the minimal mounts needed for immucore
// For now only proc is needed to read the cmdline fully in uki mode
// in normal modes this should already be done by the initramfs process, so we can ignore errors
// Just mount dev, tmp and sys just in case
func MinimalMounts() {
	type m struct {
		source string
		target string
		t      string
		flags  int
		data   string
	}
	toMount := []m{
		//{"dev", "/dev", "devtmpfs", syscall.MS_NOSUID, "mode=755"},
		{"proc", "/proc", "proc", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME, ""},
		//{"sys", "/sys", "sysfs", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME, ""},
		//{"tmp", "/tmp", "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV, ""},
		//{"run", "/run", "tmpfs", syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME, "mode=755"},
	}
	for _, mnt := range toMount {
		_ = os.MkdirAll(mnt.target, 0755)
		if !IsMounted(mnt.target) {
			err := syscall.Mount(mnt.source, mnt.target, mnt.t, uintptr(mnt.flags), mnt.data)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}
