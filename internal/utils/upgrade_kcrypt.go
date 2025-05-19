package utils

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gofrs/uuid"
	"github.com/jaypipes/ghw"
	"github.com/kairos-io/kairos-sdk/utils"
)

// UpgradeKcryptPartitions will try check for the uuid of the persistent partition and upgrade its uuid.
func UpgradeKcryptPartitions() error {
	// Generate the predictable UUID
	persistentUUID := uuid.NewV5(uuid.NamespaceURL, "COS_PERSISTENT")
	// Check if there are any LUKS partitions, otherwise ignore
	blk, err := ghw.Block()
	if err != nil {
		return err
	}

	for _, disk := range blk.Disks {
		for _, p := range disk.Partitions {
			if p.Type == "crypto_LUKS" {
				// Check against known partition label on persistent
				KLog.Logger.Debug().Str("label", p.Label).Str("dev", p.Name).Msg("found luks partition")
				if p.Label == "persistent" {
					// Get current UUID
					volumeUUID, err := utils.SH(fmt.Sprintf("cryptsetup luksUUID %s", filepath.Join("/dev", p.Name)))
					if err != nil {
						KLog.Logger.Err(err).Send()
						return err
					}
					volumeUUID = strings.TrimSpace(volumeUUID)
					volumeUUIDParsed, err := uuid.FromString(volumeUUID)
					KLog.Logger.Debug().Interface("volumeUUID", volumeUUIDParsed).Send()
					KLog.Logger.Debug().Interface("persistentUUID", persistentUUID).Send()
					if err != nil {
						KLog.Logger.Err(err).Send()
						return err
					}

					// Check to see if it's the same already to not do anything
					if volumeUUIDParsed.String() != persistentUUID.String() {
						KLog.Logger.Debug().Str("old", volumeUUIDParsed.String()).Str("new", persistentUUID.String()).Msg("Uuid is different, updating")
						out, err := utils.SH(fmt.Sprintf("cryptsetup luksUUID -q --uuid %s %s", persistentUUID, filepath.Join("/dev", p.Name)))
						if err != nil {
							KLog.Logger.Err(err).Str("out", out).Msg("Updating uuid failed")
							return err
						}
					} else {
						KLog.Logger.Debug().Msg("UUIDs are the same, not updating")
					}
				}
			}
		}
	}

	return nil
}
