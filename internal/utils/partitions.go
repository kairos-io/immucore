package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaypipes/ghw"
	"github.com/kairos-io/immucore/internal/constants"
	sdkConstants "github.com/kairos-io/kairos-sdk/constants"
)

// KairosPartitionsPresent scans block devices via ghw and reports whether the
// COS_OEM and COS_PERSISTENT filesystem labels are present on an eligible
// disk. The scan applies the same candidate criteria as auto-selection
// (non-removable, non-virtual): a label sitting on a USB stick or loop device
// must not suppress auto-creation, nor should RAM mode end up treating
// removable media as its persistent storage. It never talks to /dev directly
// — a partition is considered present only when udev has populated its
// ID_FS_LABEL.
func KairosPartitionsPresent() (oem, persistent bool, err error) {
	block, berr := ghw.Block()
	if berr != nil {
		return false, false, fmt.Errorf("reading block info: %w", berr)
	}
	for _, disk := range block.Disks {
		if !isCandidateDisk(disk.Name, disk.IsRemovable) {
			continue
		}
		for _, part := range disk.Partitions {
			switch part.FilesystemLabel {
			case sdkConstants.OEMLabel:
				oem = true
			case sdkConstants.PersistentLabel:
				persistent = true
			}
		}
	}
	return oem, persistent, nil
}

// CandidateDisks returns the list of disks that are eligible to receive the
// auto-created COS_OEM / COS_PERSISTENT partitions. Removable, loop, ram, cd
// and device-mapper disks are excluded — those are the boot medium or
// synthetic devices, never a workstation's persistent storage.
//
// The result is sorted by size, largest first (ties broken by device path),
// because auto-selection picks the first entry: the largest disk is the same
// rule kairos-agent uses for `device: auto` at install time, so RAM mode and
// a regular install land on the same disk.
func CandidateDisks() []string {
	block, err := ghw.Block()
	if err != nil {
		return nil
	}
	type disk struct {
		path string
		size uint64
	}
	var disks []disk
	for _, d := range block.Disks {
		if !isCandidateDisk(d.Name, d.IsRemovable) {
			continue
		}
		disks = append(disks, disk{path: "/dev/" + d.Name, size: d.SizeBytes})
	}
	sort.Slice(disks, func(i, j int) bool {
		if disks[i].size != disks[j].size {
			return disks[i].size > disks[j].size
		}
		return disks[i].path < disks[j].path
	})
	out := make([]string, 0, len(disks))
	for _, d := range disks {
		out = append(out, d.path)
	}
	return out
}

// isCandidateDisk factors the "is this a real, non-removable workstation disk"
// heuristic so tests can exercise the name/removable filter without mocking
// the whole ghw tree. Removable media (USB, SD cards) and synthetic devices
// (loop, ram, zram, nbd, dm-*, sr* for CD-ROM) are rejected.
func isCandidateDisk(name string, removable bool) bool {
	if removable {
		return false
	}
	for _, prefix := range []string{"loop", "ram", "zram", "nbd", "dm-", "sr", "fd", "md"} {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// DiskHasPartitions reports whether the block device at devPath already has
// any partition table entries visible to ghw. Used to gate the wipe flag: we
// refuse to write a fresh GPT over a disk with existing partitions unless the
// caller opted in with kairos.ram.wipe.
//
// Fails CLOSED: when block info cannot be read (or the device is not in the
// list) the disk is reported as having partitions. This is a safety gate —
// answering "empty" on unknown state would let init_disk=true write a fresh
// GPT without wipe consent.
func DiskHasPartitions(devPath string) bool {
	base := filepath.Base(devPath)
	block, err := ghw.Block()
	if err != nil {
		return true
	}
	for _, d := range block.Disks {
		if d.Name == base {
			return len(d.Partitions) > 0
		}
	}
	return true
}

// DiskHasKairosPartitions reports whether the block device at devPath carries
// a COS_OEM or COS_PERSISTENT partition. A disk holding one of our labels is
// "ours" — appending the missing sibling next to it needs no wipe consent,
// unlike touching a disk whose partitions all belong to someone else.
func DiskHasKairosPartitions(devPath string) bool {
	base := filepath.Base(devPath)
	block, err := ghw.Block()
	if err != nil {
		return false
	}
	for _, d := range block.Disks {
		if d.Name != base {
			continue
		}
		for _, p := range d.Partitions {
			if p.FilesystemLabel == sdkConstants.OEMLabel || p.FilesystemLabel == sdkConstants.PersistentLabel {
				return true
			}
		}
	}
	return false
}

// ParseAutoCreateDisk reads kairos.ram.create_partitions from the kernel
// cmdline. The stanza has three shapes:
//   - absent            → set=false, no action taken
//   - bare token        → set=true, explicitDisk="" (caller must auto-select)
//   - =path             → set=true, explicitDisk=path (caller uses it verbatim)
//
// Uses exact-token matching (not ReadCMDLineArg's HasPrefix) so we don't
// confuse it with the sibling .wipe / .oem / .persistent stanzas.
func ParseAutoCreateDisk() (explicitDisk string, set bool) {
	cmdline, err := os.ReadFile(GetHostProcCmdline())
	if err != nil {
		return "", false
	}
	key := constants.CmdlineAutoCreatePartitions
	for _, tok := range strings.Fields(string(cmdline)) {
		if tok == key {
			set = true
			continue
		}
		if strings.HasPrefix(tok, key+"=") {
			set = true
			v := strings.TrimPrefix(tok, key+"=")
			if v != "" {
				return v, true
			}
		}
	}
	return "", set
}

// AutoCreateWipeEnabled reports whether kairos.ram.wipe
// is present on the cmdline. Exact-token match to avoid overlapping with the
// bare kairos.ram.create_partitions flag.
func AutoCreateWipeEnabled() bool {
	cmdline, err := os.ReadFile(GetHostProcCmdline())
	if err != nil {
		return false
	}
	for _, tok := range strings.Fields(string(cmdline)) {
		if tok == constants.CmdlineAutoCreatePartitionsWipe {
			return true
		}
	}
	return false
}

// ParseAutoCreateSize reads a size override from the cmdline (in MiB) or
// returns fallback. Accepts a plain integer number of MiB; the yip layout
// plugin uses uint64 MiB natively so we do not translate units here.
// Malformed values fall back silently — cmdline is not the place to fail
// boot on typo.
func ParseAutoCreateSize(cmdlineKey string, fallback uint64) uint64 {
	vals := ReadCMDLineArg(cmdlineKey)
	if len(vals) == 0 {
		return fallback
	}
	for _, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err == nil {
			return n
		}
		KLog.Logger.Warn().Str("key", cmdlineKey).Str("value", v).Msg("unparseable size override; using default")
	}
	return fallback
}

// BuildEnsurePartitionsStage renders the yip cloud-init YAML that creates the
// missing Kairos partitions on targetDisk. The layout plugin decides whether
// to init a fresh GPT (via initDisk) or append to an existing one, based on
// what's already present. Sizes are in MiB; a persistentSize of 0 tells yip
// to expand the persistent partition to the end of the disk.
//
// The output is the full YAML for a single yip stage document — callers hand
// it directly to the yip executor with an ad-hoc stage name.
func BuildEnsurePartitionsStage(targetDisk string, oemFound, persistentFound bool, oemSizeMiB, persistentSizeMiB uint64, initDisk bool) string {
	var partsYAML strings.Builder
	if !oemFound {
		fmt.Fprintf(&partsYAML,
			"          - fsLabel: %s\n            pLabel: oem\n            size: %d\n            filesystem: ext4\n",
			sdkConstants.OEMLabel, oemSizeMiB)
	}
	if !persistentFound {
		fmt.Fprintf(&partsYAML,
			"          - fsLabel: %s\n            pLabel: persistent\n            size: %d\n            filesystem: ext4\n",
			sdkConstants.PersistentLabel, persistentSizeMiB)
	}
	return fmt.Sprintf(`stages:
  ensure-partitions:
    - name: "Create missing Kairos partitions"
      layout:
        device:
          path: %s
          init_disk: %t
        add_partitions:
%s`, targetDisk, initDisk, partsYAML.String())
}

// WaitForKairosPartitions polls until both COS_OEM and COS_PERSISTENT labels
// appear via ghw (i.e. udev has finished processing the new partitions), or
// the timeout elapses. Kernel usually needs a beat after partx / partprobe
// before /dev/disk/by-label/... is populated.
func WaitForKairosPartitions(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		oem, persistent, err := KairosPartitionsPresent()
		if err == nil && oem && persistent {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for COS_OEM and COS_PERSISTENT to appear after partitioning")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// ramModeIntro is the shared intro paragraph for every RAM-mode failure
// screen: names the feature the user enabled and where in the flow it broke.
func ramModeIntro() string {
	return "You booted with " + Emphasize("kairos.ram") + " (in-RAM workflow). Kairos loaded the operating\n" +
		"system into memory and then tried to bring the local OEM and PERSISTENT\n" +
		"partitions online — but hit a problem before your cloud-config or user\n" +
		"data could be reached. Details below."
}

// RenderMissingPartitionsMessage returns the operator-facing screen shown
// when required partitions are missing and no auto-create flag was set.
// Lists exactly which partitions are missing plus every candidate disk
// currently visible, so an admin can copy-paste the right cmdline back into
// their PXE server / grub config.
func RenderMissingPartitionsMessage(oemFound, persistentFound bool, candidates []string) string {
	var wrong strings.Builder
	wrong.WriteString("The in-RAM workflow needs both partitions on a local disk:\n")
	state := func(found bool) string {
		if found {
			return "present"
		}
		return "MISSING"
	}
	fmt.Fprintf(&wrong, "  - %s              %s\n", sdkConstants.OEMLabel, state(oemFound))
	fmt.Fprintf(&wrong, "  - %s       %s\n", sdkConstants.PersistentLabel, state(persistentFound))

	var fix strings.Builder
	fmt.Fprintf(&fix, "  %s               auto-create partitions on the single candidate disk\n", constants.CmdlineAutoCreatePartitions)
	fmt.Fprintf(&fix, "  %s=/dev/vda      auto-create on an explicit disk\n", constants.CmdlineAutoCreatePartitions)

	var extra strings.Builder
	fmt.Fprintf(&extra, "  %s          overwrite an existing partition table (DESTROYS DATA)\n", constants.CmdlineAutoCreatePartitionsWipe)
	fmt.Fprintf(&extra, "  %s64            override OEM size in MiB (default %d)\n", constants.CmdlineAutoCreateOemSize, sdkConstants.OEMSize)
	fmt.Fprintf(&extra, "  %s0      override PERSISTENT size in MiB (default %d = fill disk)\n", constants.CmdlineAutoCreatePersistentSize, sdkConstants.PersistentSize)

	var disks strings.Builder
	if len(candidates) == 0 {
		disks.WriteString("  (none — check that a non-removable disk is attached to this machine)\n")
	} else {
		for _, d := range candidates {
			fmt.Fprintf(&disks, "  %s\n", d)
		}
	}

	return RenderFailureScreen(
		"RAM mode: required disk partitions are missing",
		ramModeIntro(),
		FailureSection{Title: "What went wrong", Body: wrong.String()},
		FailureSection{Title: "How to fix (add to your kernel cmdline)", Body: fix.String()},
		FailureSection{Title: "Optional extra flags", Body: extra.String()},
		FailureSection{Title: "Candidate disks visible on this host", Body: disks.String()},
	)
}

// RenderNoDisksMessage explains that auto-create was requested but no
// eligible disk is attached to the machine at all (only removable / virtual
// devices were found).
func RenderNoDisksMessage() string {
	var wrong strings.Builder
	wrong.WriteString("Partition auto-creation was requested but no eligible disk was found.\n")
	wrong.WriteString("Removable media (USB, SD), CD-ROM and virtual devices (loop, ram,\n")
	wrong.WriteString("device-mapper) are never used for the OEM and PERSISTENT partitions.\n")

	var fix strings.Builder
	fix.WriteString("  - Attach a non-removable disk to this machine, or\n")
	fmt.Fprintf(&fix, "  - Point auto-create at a specific device: %s=/dev/xxx\n", constants.CmdlineAutoCreatePartitions)

	return RenderFailureScreen(
		"RAM mode: no disk available for the OEM and PERSISTENT partitions",
		ramModeIntro(),
		FailureSection{Title: "What went wrong", Body: wrong.String()},
		FailureSection{Title: "How to fix", Body: fix.String()},
	)
}

// RenderWipeRequiredMessage explains that the target disk already has a
// partition table and the wipe flag is not set. Same reasoning as
// RenderAmbiguousDiskMessage — halt loudly over overwriting data.
func RenderWipeRequiredMessage(disk string) string {
	var wrong strings.Builder
	fmt.Fprintf(&wrong, "Target disk %s already carries a partition table. Auto-create\n", disk)
	wrong.WriteString("will not overwrite existing partitions without an explicit opt-in.\n")

	var fix strings.Builder
	fmt.Fprintf(&fix, "  %s      DESTROYS all data on %s\n", constants.CmdlineAutoCreatePartitionsWipe, disk)

	return RenderFailureScreen(
		"RAM mode: target disk not empty",
		ramModeIntro(),
		FailureSection{Title: "What went wrong", Body: wrong.String()},
		FailureSection{Title: "How to fix (add to your kernel cmdline)", Body: fix.String()},
	)
}
