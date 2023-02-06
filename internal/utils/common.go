package utils

import (
	"regexp"

	"github.com/twpayne/go-vfs"
)

func BootedFromCD(fs vfs.FS) bool {
	cdlabel := regexp.MustCompile("root=live:CDLABEL=")
	cmdLine, err := fs.ReadFile("/proc/cmdline")
	if err != nil {
		return false
	}

	if cdlabel.MatchString(string(cmdLine)) {
		return true
	}
	return false
}
