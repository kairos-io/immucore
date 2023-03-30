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

    inst_multiple immucore
    # add utils used by yip stages
    inst_multiple kcrypt partprobe sync udevadm parted mkfs.ext2 mkfs.ext3 mkfs.ext4 mkfs.vfat mkfs.fat blkid e2fsck resize2fs mount umount sgdisk rsync
    # missing mkfs.xfs xfs_growfs in image?
    inst_script "${moddir}/generator.sh" "${systemdutildir}/system-generators/immucore-generator"
    inst_simple "${moddir}/immucore.service" "${systemdsystemunitdir}/immucore.service"
    mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
    ln_r "../immucore.service" "${systemdsystemunitdir}/initrd.target.requires/immucore.service"

    dracut_need_initqueue
}
