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
)

// KairosPartitionsPresent scans block devices via ghw and reports whether the
// COS_OEM and COS_PERSISTENT filesystem labels are present anywhere on the
// system. It never talks to /dev directly — a partition is considered present
// only when udev has populated its ID_FS_LABEL.
func KairosPartitionsPresent() (oem, persistent bool, err error) {
	block, berr := ghw.Block()
	if berr != nil {
		return false, false, fmt.Errorf("reading block info: %w", berr)
	}
	for _, disk := range block.Disks {
		for _, part := range disk.Partitions {
			switch part.FilesystemLabel {
			case constants.OemLabel:
				oem = true
			case constants.PersistentLabel:
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
// The result is sorted by device path for deterministic behavior.
func CandidateDisks() []string {
	block, err := ghw.Block()
	if err != nil {
		return nil
	}
	var out []string
	for _, d := range block.Disks {
		if !isCandidateDisk(d.Name, d.IsRemovable) {
			continue
		}
		out = append(out, "/dev/"+d.Name)
	}
	sort.Strings(out)
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
func DiskHasPartitions(devPath string) bool {
	base := filepath.Base(devPath)
	block, err := ghw.Block()
	if err != nil {
		return false
	}
	for _, d := range block.Disks {
		if d.Name == base {
			return len(d.Partitions) > 0
		}
	}
	return false
}

// ParseAutoCreateDisk reads kairos.ram.auto_create_partitions from the kernel
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
// bare kairos.ram.auto_create_partitions flag.
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
			constants.OemLabel, oemSizeMiB)
	}
	if !persistentFound {
		fmt.Fprintf(&partsYAML,
			"          - fsLabel: %s\n            pLabel: persistent\n            size: %d\n            filesystem: ext4\n",
			constants.PersistentLabel, persistentSizeMiB)
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

// RenderMissingPartitionsMessage returns the operator-facing message printed
// to stderr when required partitions are missing and no auto-create flag was
// set. It lists exactly which partitions are missing plus every candidate
// disk currently visible, so an admin can copy-paste the right cmdline back
// into their PXE server / grub config.
func RenderMissingPartitionsMessage(oemFound, persistentFound bool, candidates []string) string {
	var b strings.Builder
	b.WriteString("========================================\n")
	b.WriteString("    IMMUCORE: MISSING PARTITIONS\n")
	b.WriteString("========================================\n")
	b.WriteString("Required partitions not found:\n")
	if !oemFound {
		fmt.Fprintf(&b, "  - %s (missing)\n", constants.OemLabel)
	}
	if !persistentFound {
		fmt.Fprintf(&b, "  - %s (missing)\n", constants.PersistentLabel)
	}
	b.WriteString("\nTo auto-create partitions on first boot, add to kernel cmdline:\n")
	fmt.Fprintf(&b, "  %s               (auto-select single candidate disk)\n", constants.CmdlineAutoCreatePartitions)
	fmt.Fprintf(&b, "  %s=/dev/vda      (explicit disk)\n", constants.CmdlineAutoCreatePartitions)
	b.WriteString("\nAdd this flag to overwrite existing partition tables (DESTROYS DATA):\n")
	fmt.Fprintf(&b, "  %s\n", constants.CmdlineAutoCreatePartitionsWipe)
	b.WriteString("\nSize overrides (MiB, optional):\n")
	fmt.Fprintf(&b, "  %s64      (default %d)\n", constants.CmdlineAutoCreateOemSize, constants.DefaultOemSizeMiB)
	fmt.Fprintf(&b, "  %s0       (default %d = fill disk)\n", constants.CmdlineAutoCreatePersistentSize, constants.DefaultPersistentSizeMiB)
	b.WriteString("\nCandidate disks visible on this host:\n")
	if len(candidates) == 0 {
		b.WriteString("  (none — check that the workstation has a non-removable disk attached)\n")
	} else {
		for _, d := range candidates {
			fmt.Fprintf(&b, "  %s\n", d)
		}
	}
	b.WriteString("========================================\n")
	return b.String()
}

// RenderAmbiguousDiskMessage explains why auto-selection refused to guess when
// more than one candidate disk is present. Better a clear halt than silent
// data loss on the wrong drive.
func RenderAmbiguousDiskMessage(candidates []string) string {
	var b strings.Builder
	b.WriteString("========================================\n")
	b.WriteString("    IMMUCORE: AMBIGUOUS DISK SELECTION\n")
	b.WriteString("========================================\n")
	fmt.Fprintf(&b, "Found %d candidate disks; refusing to auto-select.\n\n", len(candidates))
	b.WriteString("Set an explicit disk in the kernel cmdline:\n")
	for _, d := range candidates {
		fmt.Fprintf(&b, "  %s=%s\n", constants.CmdlineAutoCreatePartitions, d)
	}
	b.WriteString("========================================\n")
	return b.String()
}

// RenderWipeRequiredMessage explains that the target disk already has a
// partition table and the wipe flag is not set. Same reasoning as
// RenderAmbiguousDiskMessage — halt loudly over overwriting data.
func RenderWipeRequiredMessage(disk string) string {
	var b strings.Builder
	b.WriteString("========================================\n")
	b.WriteString("    IMMUCORE: DISK NOT EMPTY\n")
	b.WriteString("========================================\n")
	fmt.Fprintf(&b, "Target disk %s already has partitions; refusing to overwrite.\n\n", disk)
	b.WriteString("To wipe existing partitions and start fresh, ADD to the kernel cmdline:\n")
	fmt.Fprintf(&b, "  %s\n", constants.CmdlineAutoCreatePartitionsWipe)
	b.WriteString("\nWARNING: this DESTROYS all data on the disk.\n")
	b.WriteString("========================================\n")
	return b.String()
}
