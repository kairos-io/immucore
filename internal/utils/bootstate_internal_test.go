package utils

import (
	"testing"

	"github.com/kairos-io/kairos-sdk/state"
)

// The statereset GRUB entry boots the recovery image with kairos.reset on the
// cmdline. kairos-sdk v0.25.0 started reporting that boot as AutoReset instead
// of Recovery for non-UKI systems, which left both label lookups falling
// through to their empty/error default. sysroot then had no device, the boot
// died in the initramfs, and the machine rebooted into the old system with the
// persistent partition untouched.
func TestBootStateToSysrootLabel(t *testing.T) {
	for _, tt := range []struct {
		boot state.Boot
		want string
	}{
		{state.Active, "COS_ACTIVE"},
		{state.Passive, "COS_PASSIVE"},
		{state.Recovery, "COS_SYSTEM"},
		{state.AutoReset, "COS_SYSTEM"},
		{state.LiveCD, ""},
		{state.Unknown, ""},
	} {
		if got := bootStateToSysrootLabel(tt.boot); got != tt.want {
			t.Errorf("bootStateToSysrootLabel(%q) = %q, want %q", tt.boot, got, tt.want)
		}
	}
}

func TestBootStateToImagesLabel(t *testing.T) {
	for _, tt := range []struct {
		boot state.Boot
		want string
	}{
		{state.Active, "COS_STATE"},
		{state.Passive, "COS_STATE"},
		{state.Recovery, "COS_RECOVERY"},
		{state.AutoReset, "COS_RECOVERY"},
		{state.LiveCD, ""},
		{state.Unknown, ""},
	} {
		if got := bootStateToImagesLabel(tt.boot); got != tt.want {
			t.Errorf("bootStateToImagesLabel(%q) = %q, want %q", tt.boot, got, tt.want)
		}
	}
}
