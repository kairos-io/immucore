#!/bin/bash

# called by dracut
check() {
    return 0
}

# called by dracut 
depends() {
    echo rootfs-block dm fs-lib lvm
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
    inst_multiple kairos-agent
    # add utils used by yip stages
    inst_multiple partprobe sync udevadm parted mkfs.ext2 mkfs.ext3 mkfs.ext4 mkfs.vfat mkfs.fat blkid lsblk e2fsck resize2fs mount umount sgdisk rsync cryptsetup growpart
    # missing mkfs.xfs xfs_growfs in image?
    inst_script "${moddir}/generator.sh" "${systemdutildir}/system-generators/immucore-generator"
    # SERVICES FOR SYSTEMD-BASED SYSTEMS
    inst_simple "${moddir}/immucore.service" "${systemdsystemunitdir}/immucore.service"
    mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
    ln_r "../immucore.service" "${systemdsystemunitdir}/initrd.target.requires/immucore.service"
    # END SYSTEMD SERVICES

    dracut_need_initqueue
}
