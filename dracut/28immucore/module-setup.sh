#!/bin/bash

# called by dracut
check() {
    return 0
}

# called by dracut 
depends() {
    echo rootfs-block dm fs-lib
    return 0
}

# called by dracut
installkernel() {
    instmods overlay
}

# called by dracut
install() {
    declare moddir=${moddir}
    declare systemdutildir=${systemdutildir}
    declare systemdsystemunitdir=${systemdsystemunitdir}

    # Add missing elemental binary, drop once we get yip lib inside immucore as its only needed to run the stages
    inst_multiple immucore elemental
    # add utils used by elemental or stages
    inst_multiple partprobe sync udevadm parted mkfs.ext2 mkfs.ext3 mkfs.ext4 mkfs.vfat mkfs.fat blkid e2fsck resize2fs mount umount sgdisk rsync
    # missing mkfs.xfs xfs_growfs in image?
    inst_script "${moddir}/generator.sh" "${systemdutildir}/system-generators/immucore-generator"
    inst_simple "${moddir}/immucore.service" "${systemdsystemunitdir}/immucore.service"
    mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
    ln_r "../immucore.service" "${systemdsystemunitdir}/initrd.target.requires/immucore.service"

    dracut_need_initqueue
}