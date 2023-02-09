package utils

import (
	"fmt"
	"github.com/containerd/containerd/mount"
	"github.com/deniswernert/go-fstab"
	"github.com/kairos-io/kairos/pkg/utils"
	"os"
	"os/exec"
	"strings"
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
			res = append(res, dat[1])
		}
	}
	return res
}

// IsMountedByLabel lets us know if the given label is currently mounted
func IsMountedByLabel(label string) bool {
	_, err := utils.SH(fmt.Sprintf("findmnt /dev/disk/by-label/%s", label))
	return err == nil
}

// DiskFSType will return the FS type for a given disk
// Needs to be mounted
// Needs full path so either /dev/sda1 or /dev/disk/by-{label,uuid}/{label,uuid}
func DiskFSType(s string) string {
	out, _ := utils.SH(fmt.Sprintf("findmnt -rno FSTYPE %s", s))
	return out
}

// SyncState will rsync source into destination. Useful for Bind mounts.
func SyncState(src, dst string) error {
	return exec.Command("rsync", "-aqAX", src, dst).Run()
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
