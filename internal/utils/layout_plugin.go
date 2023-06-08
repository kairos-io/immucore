package utils

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kairos-io/immucore/internal/constants"
	"github.com/mudler/yip/pkg/logger"
	"github.com/mudler/yip/pkg/plugins"
	"github.com/mudler/yip/pkg/schema"
	"github.com/rancher/elemental-cli/pkg/partitioner"
	"github.com/twpayne/go-vfs"
)

// YipLayoutPlugin is the immucore implementation of Layout yip's plugin based
// on partitioner package.
func YipLayoutPlugin(l logger.Interface, s schema.Stage, _ vfs.FS, _ plugins.Console) (err error) {
	if s.Layout.Device == nil {
		return nil
	}

	var dev *partitioner.Disk

	if len(strings.TrimSpace(s.Layout.Device.Label)) > 0 {
		partDevice, err := GetDiskByPartLabel(s.Layout.Device.Label, 5)
		if err != nil {
			l.Errorf("Exiting, disk not found:\n %s", err.Error())
			return err
		}
		dev = partitioner.NewDisk(
			partDevice,
		)
	} else if len(strings.TrimSpace(s.Layout.Device.Path)) > 0 {
		dev = partitioner.NewDisk(
			s.Layout.Device.Path,
		)
	} else {
		l.Warnf("No target device defined, nothing to do")
		return nil
	}

	if !dev.Exists() {
		l.Errorf("Exiting, disk not found:\n %s", s.Layout.Device.Path)
		return errors.New("target disk not found")
	}

	if s.Layout.Expand != nil {
		l.Infof("Extending last partition up to %d MiB", s.Layout.Expand.Size)
		out, err := dev.ExpandLastPartition(s.Layout.Expand.Size)
		if err != nil {
			l.Error(out)
			return err
		}
	}

	for _, part := range s.Layout.Parts {
		_, err := GetPartByLabel(part.FSLabel, 1)
		if err == nil {
			l.Warnf("Partition with FSLabel: %s already exists, ignoring", part.FSLabel)
			continue
		}

		// Set default filesystem
		if part.FileSystem == "" {
			part.FileSystem = constants.LinuxFs
		}

		l.Infof("Creating %s partition", part.FSLabel)
		partNum, err := dev.AddPartition(part.Size, part.FileSystem, part.PLabel)
		if err != nil {
			return fmt.Errorf("Failed creating partitions: %w", err)
		}
		out, err := dev.FormatPartition(partNum, part.FileSystem, part.FSLabel)
		if err != nil {
			return fmt.Errorf("Formatting partition failed: %s\nError: %w", out, err)
		}
	}
	return nil
}
