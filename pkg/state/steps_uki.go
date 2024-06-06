package state

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/foxboron/go-uefi/efi"
	"github.com/hashicorp/go-multierror"
	cnst "github.com/kairos-io/immucore/internal/constants"
	internalUtils "github.com/kairos-io/immucore/internal/utils"
	"github.com/kairos-io/immucore/pkg/op"
	"github.com/kairos-io/immucore/pkg/schema"
	"github.com/kairos-io/kairos-sdk/signatures"
	"github.com/kairos-io/kairos-sdk/state"
	kcrypt "github.com/kairos-io/kcrypt/pkg/lib"
	"github.com/mudler/go-kdetect"
	"github.com/spectrocloud-labs/herd"
)

// UKIExtendPCR extends the PCR with the given extension in a graceful way.
func UKIExtendPCR(extension string) error {
	return internalUtils.PCRExtend(cnst.DefaultPCR, []byte(extension))

}

// UKIMountBaseSystem mounts the base system for the UKI boot system
// as when booting in UKI mode we have a blank slate and we need to mount everything
// Make sure we set the directories as MS_SHARED
// This is important afterwards when running containers and they get unshared and so on
// And can lead to rootfs out of boundaries issues for them
// also it doesnt help when mounting the final rootfs as we want to broke the mounts into it and any submounts.
func (s *State) UKIMountBaseSystem(g *herd.Graph) error {
	type mount struct {
		where string
		what  string
		fs    string
		flags uintptr
		data  string
	}

	return g.Add(
		cnst.OpUkiBaseMounts,
		herd.WithCallback(
			func(_ context.Context) error {
				var err error
				// Mount base mounts
				mounts := []mount{
					{
						"/sys",
						"sysfs",
						"sysfs",
						syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
						"",
					},
					{
						"/sys",
						"",
						"",
						syscall.MS_SHARED,
						"",
					},
					{
						"/sys/kernel/security",
						"securityfs",
						"securityfs",
						0,
						"",
					},
					{
						"/sys/kernel/debug",
						"debugfs",
						"debugfs",
						0,
						"",
					},
					{
						"/sys/firmware/efi/efivars",
						"efivarfs",
						"efivarfs",
						syscall.MS_NOSUID | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_RELATIME,
						"",
					},
					{
						"/dev",
						"devtmpfs",
						"devtmpfs",
						syscall.MS_NOSUID,
						"mode=755",
					},
					{
						"/dev",
						"",
						"",
						syscall.MS_SHARED,
						"",
					},
					{
						"/dev/pts",
						"devpts",
						"devpts",
						syscall.MS_NOSUID | syscall.MS_NOEXEC,
						"ptmxmode=000,gid=5,mode=620",
					},
					{
						"/dev/shm",
						"tmpfs",
						"tmpfs",
						0,
						"",
					},
					{
						"/tmp",
						"tmpfs",
						"tmpfs",
						syscall.MS_NOSUID | syscall.MS_NODEV,
						"",
					},
					{
						"/tmp",
						"",
						"",
						syscall.MS_SHARED,
						"",
					},
				}

				for dir, perm := range map[string]os.FileMode{
					"/proc":    0o555,
					"/dev":     0o777,
					"/dev/pts": 0o777,
					"/dev/shm": 0o777,
					"/sys":     0o555,
				} {
					e := os.MkdirAll(dir, perm)
					if e != nil {
						internalUtils.Log.Err(e).Str("dir", dir).Interface("permissions", perm).Msg("Creating dir")
					}
				}
				for _, m := range mounts {
					e := os.MkdirAll(m.where, 0755)
					if e != nil {
						err = multierror.Append(err, e)
						internalUtils.Log.Err(e).Msg("Creating dir")
					}

					e = internalUtils.Mount(m.what, m.where, m.fs, m.flags, m.data)
					if e != nil {
						err = multierror.Append(err, e)
						internalUtils.Log.Err(e).Str("what", m.what).Str("where", m.where).Str("type", m.fs).Msg("Mounting")
					}
				}

				// Now that we have all the mounts, check if we got secureboot enabled
				if !efi.GetSecureBoot() && len(internalUtils.ReadCMDLineArg("rd.immucore.securebootdisabled")) == 0 {
					internalUtils.Log.Error().Msg("Secure boot is not enabled. Aborting boot.")
					// Sleep forever.
					// We dont want to exit and print panics or kernel panic, so we print our message and wait for the user to ctrl+alt+del
					select {}
				}
				return err
			},
		),
	)
}

// UkiPivotToSysroot moves the rootfs to the sysroot and chroots into it
// Making the /sysroot the new rootfs with a tmpfs fs
// And moving all the mounts into it and all the files as well.
func (s *State) UkiPivotToSysroot(g *herd.Graph) error {
	return g.Add(cnst.OpUkiPivotToSysroot,
		herd.WithDeps(cnst.OpUkiBaseMounts),
		herd.WithCallback(func(_ context.Context) error {
			var err error
			// Create the new sysroot and move to it
			// We need the sysroot to NOT be of type rootfs, otherwise kubernetes stuff doesnt really work
			internalUtils.Log.Debug().Str("what", s.path(cnst.UkiSysrootDir)).Msg("Creating sysroot dir")
			err = os.MkdirAll(s.path(cnst.UkiSysrootDir), 0755) // #nosec G301 -- Sysroot needs to be 755 to be world readable
			if err != nil {
				internalUtils.Log.Err(err).Msg("creating sysroot dir")
				internalUtils.DropToEmergencyShell()
			}

			// Mount a tmpfs under sysroot
			internalUtils.Log.Debug().Msg("Mounting tmpfs on sysroot")
			err = internalUtils.Mount("tmpfs", s.path(cnst.UkiSysrootDir), "tmpfs", 0, "")
			if err != nil {
				internalUtils.Log.Err(err).Msg("mounting tmpfs on sysroot")
				internalUtils.DropToEmergencyShell()
			}

			// Move all the dirs in root FS that are not a mountpoint to the new root via cp -R
			rootDirs, err := os.ReadDir(s.Rootdir)
			if err != nil {
				internalUtils.Log.Err(err).Msg("reading rootdir content")
			}

			var mountPoints []string
			for _, file := range rootDirs {
				if file.Name() == cnst.UkiSysrootDir {
					continue
				}
				if file.IsDir() {
					path := file.Name()
					fileInfo, err := os.Stat(s.path(path))
					if err != nil {
						return err
					}
					parentPath := filepath.Dir(s.path(path))
					parentInfo, err := os.Stat(parentPath)
					if err != nil {
						return err
					}
					// If the directory has the same device as its parent, it's not a mount point.
					if fileInfo.Sys().(*syscall.Stat_t).Dev == parentInfo.Sys().(*syscall.Stat_t).Dev {
						internalUtils.Log.Debug().Str("what", path).Msg("simple directory")
						err = os.MkdirAll(filepath.Join(s.path(cnst.UkiSysrootDir), path), fileInfo.Mode())
						if err != nil {
							internalUtils.Log.Err(err).Str("what", filepath.Join(s.path(cnst.UkiSysrootDir), path)).Msg("mkdir")
							return err
						}

						// Copy it over
						out, err := internalUtils.CommandWithPath(fmt.Sprintf("cp -a %s %s", s.path(path), s.path(cnst.UkiSysrootDir)))
						if err != nil {
							internalUtils.Log.Err(err).Str("out", out).Str("what", s.path(path)).Str("where", s.path(cnst.UkiSysrootDir)).Msg("copying dir into sysroot")
						}
						continue
					}

					internalUtils.Log.Debug().Str("what", path).Msg("mount point")
					mountPoints = append(mountPoints, s.path(path))

					continue
				}

				info, _ := file.Info()
				fileInfo, _ := os.Lstat(file.Name())

				// Symlink
				if fileInfo.Mode()&os.ModeSymlink != 0 {
					target, err := os.Readlink(file.Name())
					if err != nil {
						return fmt.Errorf("failed to read symlink: %w", err)
					}
					symlinkPath := s.path(filepath.Join(cnst.UkiSysrootDir, file.Name()))
					err = os.Symlink(target, symlinkPath)
					if err != nil {
						internalUtils.Log.Err(err).Str("from", target).Str("to", symlinkPath).Msg("Symlink")
						internalUtils.DropToEmergencyShell()
					}
					internalUtils.Log.Debug().Str("from", target).Str("to", symlinkPath).Msg("Symlinked file")
				} else {
					// If its a file in the root dir just copy it over
					content, _ := os.ReadFile(s.path(file.Name()))
					newFilePath := s.path(filepath.Join(cnst.UkiSysrootDir, file.Name()))
					_ = os.WriteFile(newFilePath, content, info.Mode())
					internalUtils.Log.Debug().Str("from", s.path(file.Name())).Str("to", newFilePath).Msg("Copied file")
				}
			}

			// Now move the system mounts into the new dir
			for _, d := range mountPoints {
				newDir := filepath.Join(s.path(cnst.UkiSysrootDir), d)
				if _, err := os.Stat(newDir); err != nil {
					err = os.MkdirAll(newDir, 0755)
					if err != nil {
						internalUtils.Log.Err(err).Str("what", newDir).Msg("mkdir")
					}
				}

				err = internalUtils.Mount(d, newDir, "", syscall.MS_MOVE, "")
				if err != nil {
					internalUtils.Log.Err(err).Str("what", d).Str("where", newDir).Msg("move mount")
					continue
				}
				internalUtils.Log.Debug().Str("from", d).Str("to", newDir).Msg("Mount moved")
			}

			internalUtils.Log.Debug().Str("to", s.path(cnst.UkiSysrootDir)).Msg("Changing dir")
			if err = syscall.Chdir(s.path(cnst.UkiSysrootDir)); err != nil {
				internalUtils.Log.Err(err).Msg("chdir")
				internalUtils.DropToEmergencyShell()
			}

			internalUtils.Log.Debug().Str("what", s.path(cnst.UkiSysrootDir)).Str("where", "/").Msg("Moving mount")
			if err = internalUtils.Mount(s.path(cnst.UkiSysrootDir), "/", "", syscall.MS_MOVE, ""); err != nil {
				internalUtils.Log.Err(err).Msg("mount move")
				internalUtils.DropToEmergencyShell()
			}

			internalUtils.Log.Debug().Str("to", ".").Msg("Chrooting")
			if err = syscall.Chroot("."); err != nil {
				internalUtils.Log.Err(err).Msg("chroot")
				internalUtils.DropToEmergencyShell()
			}

			ext := "enter-initrd"
			pcrErr := UKIExtendPCR(ext)
			if pcrErr != nil {
				internalUtils.Log.Err(pcrErr).Str("ext", ext).Msg("extend-pcr")
			}

			pcrErr = os.MkdirAll("/run/systemd", 0755) // #nosec G301 -- Original dir has this permissions
			if pcrErr != nil {
				internalUtils.Log.Err(pcrErr).Msg("Creating /run/systemd dir")
			}
			// This dir is created by systemd-stub and passed to the kernel as a cpio archive
			// that gets mounted in the initial ramdisk where we run immucore from
			// It contains the tpm public key and signatures of the current uki
			out, pcrErr := internalUtils.CommandWithPath("cp /.extra/* /run/systemd/")
			if pcrErr != nil {
				internalUtils.Log.Err(pcrErr).Str("out", out).Msg("Copying extra files")
			}
			return err
		}))
}

// UKIUdevDaemon launches the udevd daemon and triggers+settles in order to discover devices
// Needed if we expect to find devices by label...
func (s *State) UKIUdevDaemon(g *herd.Graph) error {
	return g.Add(cnst.OpUkiUdev,
		herd.WithDeps(cnst.OpUkiBaseMounts, cnst.OpUkiPivotToSysroot, cnst.OpUkiKernelModules),
		herd.WithCallback(func(_ context.Context) error {
			// Should probably figure out other udevd binaries....
			var udevBin string
			if _, err := os.Stat("/usr/lib/systemd/systemd-udevd"); !os.IsNotExist(err) {
				udevBin = "/usr/lib/systemd/systemd-udevd"
			}
			cmd := fmt.Sprintf("%s --daemon", udevBin)
			out, err := internalUtils.CommandWithPath(cmd)
			internalUtils.Log.Debug().Str("out", out).Str("cmd", cmd).Msg("Udev daemon")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev daemon")
				return err
			}
			out, err = internalUtils.CommandWithPath("udevadm trigger")
			internalUtils.Log.Debug().Str("out", out).Msg("Udev trigger")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev trigger")
				return err
			}

			out, err = internalUtils.CommandWithPath("udevadm settle")
			internalUtils.Log.Debug().Str("out", out).Msg("Udev settle")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Udev settle")
				return err
			}
			return nil
		}),
	)
}

// UKILoadKernelModules loads kernel modules needed during uki boot to load the disks for.
// Mainly block devices and net devices
// probably others down the line.
func (s *State) UKILoadKernelModules(g *herd.Graph) error {
	return g.Add(cnst.OpUkiKernelModules,
		herd.WithDeps(cnst.OpUkiBaseMounts, cnst.OpUkiPivotToSysroot),
		herd.WithCallback(func(_ context.Context) error {
			drivers, err := kdetect.ProbeKernelModules("")
			if err != nil {
				internalUtils.Log.Err(err).Msg("Detecting needed modules")
			}
			drivers = append(drivers, cnst.GenericKernelDrivers()...)
			internalUtils.Log.Debug().Strs("drivers", drivers).Msg("Detecting needed modules")
			for _, driver := range drivers {
				cmd := fmt.Sprintf("modprobe %s", driver)
				out, err := internalUtils.CommandWithPath(cmd)
				if err != nil {
					internalUtils.Log.Debug().Err(err).Str("out", out).Msg("modprobe")
				}
			}
			return nil
		}),
	)
}

// UKIUnlock tries to unlock the disks with the TPM policy.
func (s *State) UKIUnlock(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpUkiKcrypt, append(opts, herd.WithCallback(func(_ context.Context) error {
		// Set full path on uki to get all the binaries
		if !state.EfiBootFromInstall(internalUtils.Log) {
			internalUtils.Log.Debug().Msg("Not unlocking disks as we think we are booting from removable media")
			return nil
		}
		_ = os.Setenv("PATH", "/usr/bin:/usr/sbin:/bin:/sbin")
		internalUtils.Log.Debug().Msg("Will now try to unlock partitions")
		return kcrypt.UnlockAllWithLogger(true, internalUtils.Log)
	}))...)
}

// UKIMountLiveCd tries to mount the livecd if we are booting from one into /run/initramfs/live
// to mimic the same behavior as the livecd on non-uki boot.
func (s *State) UKIMountLiveCd(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpUkiMountLivecd, append(opts, herd.WithCallback(func(_ context.Context) error {
		// If we are booting from Install Media
		if state.EfiBootFromInstall(internalUtils.Log) {
			internalUtils.Log.Debug().Msg("Not mounting livecd as we think we are booting from removable media")
			return nil
		}

		err := os.MkdirAll(s.path(cnst.UkiLivecdMountPoint), 0755)
		if err != nil {
			internalUtils.Log.Err(err).Msg(fmt.Sprintf("Creating %s", cnst.UkiLivecdMountPoint))
			return err
		}
		err = os.MkdirAll(s.path(cnst.UkiIsoBaseTree), 0755)
		if err != nil {
			internalUtils.Log.Err(err).Msg(fmt.Sprintf("Creating %s", cnst.UkiIsoBaseTree))
			return nil
		}

		// Select the correct device to mount
		// Try to find the CDROM device by label /dev/disk/by-label/UKI_ISO_INSTALL
		// try a couple of times as the udev daemon can take a bit of time to populate the devices
		var cdrom string

		for i := 0; i < 5; i++ {
			_, err = os.Stat(cnst.UkiLivecdPath)
			// if found, set it
			if err == nil {
				cdrom = cnst.UkiLivecdPath
				break
			}

			internalUtils.Log.Debug().Msg(fmt.Sprintf("No media with label found at %s", cnst.UkiLivecdPath))
			out, _ := internalUtils.CommandWithPath("ls -ltra /dev/disk/by-label/")
			internalUtils.Log.Debug().Str("out", out).Msg("contents of /dev/disk/by-label/")
			time.Sleep(time.Duration(i) * time.Second)
		}

		// Fallback to try to get the /dev/sr0 device directly, no retry as that wont take time to appear
		if cdrom == "" {
			_, err = os.Stat(cnst.UkiDefaultcdrom)
			if err == nil {
				cdrom = cnst.UkiDefaultcdrom
			} else {
				internalUtils.Log.Debug().Msg(fmt.Sprintf("No media found at %s", cnst.UkiDefaultcdrom))
			}
		}

		// Mount it
		if cdrom != "" {
			err = internalUtils.Mount(cdrom, s.path(cnst.UkiLivecdMountPoint), cnst.UkiDefaultcdromFsType, syscall.MS_RDONLY, "")
			if err != nil {
				internalUtils.Log.Err(err).Msg(fmt.Sprintf("Mounting %s", cdrom))
				return err
			}
			internalUtils.Log.Debug().Msg(fmt.Sprintf("Mounted %s", cdrom))

			// This needs the loop module to be inserted in the kernel!
			cmd := fmt.Sprintf("losetup --show -f %s", s.path(filepath.Join(cnst.UkiLivecdMountPoint, cnst.UkiIsoBootImage)))
			out, err := internalUtils.CommandWithPath(cmd)
			loop := strings.TrimSpace(out)

			if err != nil || loop == "" {
				internalUtils.Log.Err(err).Str("out", out).Msg(cmd)
				return err
			}

			err = internalUtils.Mount(loop, s.path(cnst.UkiIsoBaseTree), cnst.UkiDefaultEfiimgFsType, syscall.MS_RDONLY, "")
			if err != nil {
				internalUtils.Log.Err(err).Msg(fmt.Sprintf("Mounting %s into %s", loop, s.path(cnst.UkiIsoBaseTree)))
				return err
			}
			return nil
		}
		internalUtils.Log.Debug().Msg("No livecd/install media found")
		return nil
	}))...)
}

// UKIBootInitDagStep tries to launch /sbin/init in root and pass over the system
// booting to the real init process
// Drops to emergency if not able to. Panic if it cant even launch emergency.
func (s *State) UKIBootInitDagStep(g *herd.Graph) error {
	return g.Add(cnst.OpUkiInit,
		herd.WeakDeps,
		herd.WithWeakDeps(cnst.OpRootfsHook, cnst.OpInitramfsHook, cnst.OpWriteFstab, cnst.OpUkiLoadSysExtensions),
		herd.WithCallback(func(_ context.Context) error {
			var err error

			ext := "leave-initrd"
			err = UKIExtendPCR(ext)
			if err != nil {
				internalUtils.Log.Err(err).Str("ext", ext).Msg("extend-pcr")
				internalUtils.DropToEmergencyShell()
			}

			internalUtils.Log.Debug().Str("what", s.path(s.Rootdir)).Msg("Mount / RO")
			if err = internalUtils.Mount("", s.path(s.Rootdir), "", syscall.MS_REMOUNT|syscall.MS_RDONLY, "ro"); err != nil {
				internalUtils.Log.Err(err).Msg("Mount / RO")
				internalUtils.DropToEmergencyShell()
			}

			// Print dag before exit, otherwise its never printed as we never exit the program
			internalUtils.Log.Info().Msg(s.WriteDAG(g))
			internalUtils.Log.Debug().Msg("Executing init callback!")
			if err := syscall.Exec("/sbin/init", []string{"/sbin/init"}, os.Environ()); err != nil {
				internalUtils.DropToEmergencyShell()
			}
			return nil
		}))
}

// UKIMountESPPartition tries to mount the ESP into /efi
// Doesnt matter if it fails, its just for niceness.
func (s *State) UKIMountESPPartition(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add("mount-esp", append(opts, herd.WithCallback(func(_ context.Context) error {
		if !state.EfiBootFromInstall(internalUtils.Log) {
			internalUtils.Log.Debug().Msg("Not mounting ESP as we think we are booting from removable media")
			return nil
		}
		cmd := "lsblk -J -o NAME,PARTTYPE"
		out, err := internalUtils.CommandWithPath(cmd)
		internalUtils.Log.Debug().Str("out", out).Str("cmd", cmd).Msg("ESP")
		if err != nil {
			internalUtils.Log.Err(err).Msg("ESP")
			return nil
		}

		lsblk := &schema.LsblkOutput{}
		err = json.Unmarshal([]byte(out), lsblk)
		if err != nil {
			return nil
		}

		for _, bd := range lsblk.Blockdevices {
			for _, cd := range bd.Children {
				if strings.TrimSpace(cd.Parttype) == "c12a7328-f81f-11d2-ba4b-00a0c93ec93b" {
					// This is the ESP device
					device := filepath.Join("/dev", cd.Name)
					if !internalUtils.IsMounted(device) {
						fstab, err := op.MountOPWithFstab(
							device,
							s.path("/efi"),
							"vfat",
							[]string{
								"ro",
							}, 5*time.Second,
						)
						for _, f := range fstab {
							s.fstabs = append(s.fstabs, f)
						}
						return err
					}
				}
			}

		}
		return nil
	}))...)
}

func (s *State) ExtractCerts(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpUkiExtractCerts, append(opts, herd.WithCallback(func(_ context.Context) error {
		// Get all the full certs
		certs, err := signatures.GetAllFullCerts()
		if err != nil {
			return err
		}

		err = os.MkdirAll(s.path(cnst.VerityCertDir), 0755)
		if err != nil {
			return err
		}
		// Write all certs in x509 DER format to /run/verity.d/ for sysextensions to verify against
		for i, cert := range certs.PK {
			err := os.WriteFile(filepath.Join(s.path(cnst.VerityCertDir), fmt.Sprintf("PK%d.crt", i)), cert.Raw, 0644)
			if err != nil {
				return err
			}
		}
		for i, cert := range certs.KEK {
			err := os.WriteFile(filepath.Join(s.path(cnst.VerityCertDir), fmt.Sprintf("PK%d.crt", i)), cert.Raw, 0644)
			if err != nil {
				return err
			}
		}
		for i, cert := range certs.DB {
			err := os.WriteFile(filepath.Join(s.path(cnst.VerityCertDir), fmt.Sprintf("PK%d.crt", i)), cert.Raw, 0644)
			if err != nil {
				return err
			}
		}

		return nil
	}))...)
}

// CopySysExtensionsDagStep Copies extensions from the EFI partitions to the persistent one so they can be started.
func (s *State) CopySysExtensionsDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpUkiCopySysExtensions, append(opts, herd.WithCallback(func(_ context.Context) error {
		// Copy the sys extensions to the rootfs
		// return if the source or dest dir is not there
		if _, err := os.Stat(s.path(cnst.SourceSysExtDir)); os.IsNotExist(err) {
			internalUtils.Log.Debug().Str("dir", s.path(cnst.SourceSysExtDir)).Msg("No sysextensions found")
			return nil
		}
		if _, err := os.Stat(s.path(cnst.DestSysExtDir)); os.IsNotExist(err) {
			_ = os.MkdirAll(s.path(cnst.DestSysExtDir), 0755)
		}
		// Run a defer at the end just in case we fail to load the sysextensions or return early, we dont want to
		// leave stuff around
		defer internalUtils.RemoveSysExtensions()
		err := filepath.WalkDir(s.path(cnst.SourceSysExtDir), func(path string, d fs.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}
			src := filepath.Join(cnst.SourceSysExtDir, d.Name())
			dest := filepath.Join(cnst.DestSysExtDir, d.Name())
			// Copy the file to the sys-extensions directory
			err = internalUtils.Copy(src, dest)
			if err != nil {
				internalUtils.Log.Err(err).Str("src", src).Str("dest", dest).Msg("Copying sysextension")
			}
			// Try to load it and if it fails we remove it as it means its not signed
			internalUtils.Log.Debug().Str("what", src).Msg("Loading sysextension")
			err = internalUtils.LoadSysExtensions()
			if err != nil {
				internalUtils.Log.Err(err).Str("what", dest).Msg("Loading sysextension")
				_ = os.Remove(dest)
				// return nil to continue walking
				return nil
			}
			// unmerge everything before continuing
			err = internalUtils.RemoveSysExtensions()
			return err
		})
		if err != nil {
			return err
		}
		return err
	}))...)
}

// LoadSysExtensionsDagStep loads the sys extensions into the system.
// If it fails it unmerges them and returns nil to not block booting....for now.
func (s *State) LoadSysExtensionsDagStep(g *herd.Graph, opts ...herd.OpOption) error {
	return g.Add(cnst.OpUkiLoadSysExtensions, append(opts, herd.WithCallback(func(_ context.Context) error {
		// Load the sys extensions
		err := internalUtils.LoadSysExtensions()
		if err != nil {
			internalUtils.Log.Err(err).Msg("Loading sys extensions")
			err = internalUtils.RemoveSysExtensions()
			if err != nil {
				internalUtils.Log.Err(err).Msg("Removing sys extensions")
			}
		}
		// we dont return an error yet
		return nil
	}))...)
}
