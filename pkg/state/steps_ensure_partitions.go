package state

import (
	"context"
	"errors"
	"fmt"
	"time"

	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/spectrocloud-labs/herd"
)

// EnsurePartitionsDagStep gates every downstream mount step in the in-RAM
// workflow. On first boot the workstation's disk may not yet carry the
// COS_OEM and COS_PERSISTENT partitions immucore needs; this step either
// creates them (when kairos.ram.create_partitions is set) or halts the
// boot with an actionable message pointing the operator at the exact cmdline
// tokens they need to add. Once it succeeds the rest of the DAG behaves as
// if the disk had been installed normally — MountOemDagStep, custom mounts
// and cloud-init all see present labels.
//
// The step is idempotent: on any boot where both partitions already exist it
// is a no-op and returns immediately.
//
// It intentionally does NOT drop to the emergency shell on the "flag missing"
// path — that is a configuration error, not a boot failure, and needs the
// operator to change the PXE cmdline and reboot rather than hand-fix state in
// a shell. Returning an error from the DAG lets the normal failure summary
// print, which now carries our own explicit message ahead of it.
func (s *State) EnsurePartitionsDagStep(g *herd.Graph, deps ...string) error {
	return g.Add(cnst.OpEnsurePartitions,
		herd.WithDeps(deps...),
		TimedCallback(cnst.OpEnsurePartitions, func(ctx context.Context) error {
			oemFound, persistentFound, err := internalUtils.KairosPartitionsPresent()
			if err != nil {
				return fmt.Errorf("scanning disks for kairos partitions: %w", err)
			}
			if oemFound && persistentFound {
				internalUtils.KLog.Logger.Info().Msg("COS_OEM and COS_PERSISTENT present; ensure-partitions is a no-op")
				return nil
			}

			explicit, autoSet := internalUtils.ParseAutoCreateDisk()
			if !autoSet {
				candidates := internalUtils.CandidateDisks()
				internalUtils.KLog.Logger.Error().
					Bool("oem_found", oemFound).
					Bool("persistent_found", persistentFound).
					Strs("candidate_disks", candidates).
					Msg("missing required partitions and kairos.ram.create_partitions not set")
				internalUtils.HaltWithBanner(
					internalUtils.RenderMissingPartitionsMessage(oemFound, persistentFound, candidates),
					"missing kairos partitions and kairos.ram.create_partitions not set",
					errors.New("missing kairos partitions"),
				)
				// Reached only on non-systemd hosts (Alpine/openrc), where
				// HaltWithBanner paints the screen and returns so we can fail
				// the step normally.
				return errors.New("missing kairos partitions; see console for details")
			}

			target, err := selectTargetDisk(explicit)
			if err != nil {
				return err
			}

			// If we are creating BOTH partitions we may need to init a fresh
			// GPT. When one label is already present, the disk already has a
			// partition table we want to preserve — append only.
			bothMissing := !oemFound && !persistentFound
			initDisk := bothMissing
			if bothMissing && internalUtils.DiskHasPartitions(target) {
				if !internalUtils.AutoCreateWipeEnabled() {
					internalUtils.HaltWithBanner(
						internalUtils.RenderWipeRequiredMessage(target),
						fmt.Sprintf("target disk %s has existing partitions; add %s to overwrite",
							target, cnst.CmdlineAutoCreatePartitionsWipe),
						errors.New("refusing to overwrite non-empty disk without wipe flag"),
					)
					// Reached only on non-systemd hosts (Alpine/openrc), where
					// HaltWithBanner paints the screen and returns so we can
					// fail the step normally.
					return errors.New("refusing to overwrite non-empty disk; see console for details")
				}
				// Wipe explicitly requested — proceed with initDisk=true.
			}

			oemSize := internalUtils.ParseAutoCreateSize(cnst.CmdlineAutoCreateOemSize, cnst.DefaultOemSizeMiB)
			persistentSize := internalUtils.ParseAutoCreateSize(cnst.CmdlineAutoCreatePersistentSize, cnst.DefaultPersistentSizeMiB)

			internalUtils.KLog.Logger.Info().
				Str("target", target).
				Bool("init_disk", initDisk).
				Bool("oem_present", oemFound).
				Bool("persistent_present", persistentFound).
				Uint64("oem_size_mib", oemSize).
				Uint64("persistent_size_mib", persistentSize).
				Msg("creating missing kairos partitions")

			yamlStage := internalUtils.BuildEnsurePartitionsStage(target, oemFound, persistentFound, oemSize, persistentSize, initDisk)
			if err := internalUtils.RunYipStageInline("ensure-partitions", yamlStage); err != nil {
				return fmt.Errorf("yip layout stage failed: %w", err)
			}

			// yip's layout plugin already triggers a partition table reread,
			// but udev needs a beat to populate ID_FS_LABEL. Poll until both
			// labels are visible, or fail loudly.
			waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := internalUtils.WaitForKairosPartitions(waitCtx, 30*time.Second); err != nil {
				return fmt.Errorf("waiting for new partitions to become visible: %w", err)
			}
			internalUtils.KLog.Logger.Info().Msg("kairos partitions ready")
			return nil
		}))
}

// selectTargetDisk resolves the effective target disk. explicit wins when
// non-empty (operator gave us a path); otherwise we scan candidate disks and
// require exactly one, refusing to guess when more than one is available.
// Both "no candidates" and "multiple candidates" halt boot via HaltWithBanner
// — same reasoning as the missing-flag branch: returning an error would let
// systemd proceed into a broken userland.
func selectTargetDisk(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	candidates := internalUtils.CandidateDisks()
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	banner := internalUtils.RenderAmbiguousDiskMessage(candidates)
	if len(candidates) == 0 {
		internalUtils.HaltWithBanner(
			banner,
			"auto-create requested but no candidate disks visible",
			errors.New("no candidate disks"),
		)
	} else {
		internalUtils.HaltWithBanner(
			banner,
			fmt.Sprintf("auto-create requested but %d candidate disks visible; set an explicit path via %s=/dev/xxx",
				len(candidates), cnst.CmdlineAutoCreatePartitions),
			errors.New("ambiguous disk selection"),
		)
	}
	// Reached only on non-systemd hosts (Alpine/openrc), where HaltWithBanner
	// paints the screen and returns so we can fail the step normally.
	return "", errors.New("could not resolve a target disk; see console for details")
}
