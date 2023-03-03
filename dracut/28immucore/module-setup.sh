#!/bin/bash

# called by dracut
check() {
    require_binaries "$systemdutildir"/systemd || return 1
    return 255
}

# called by dracut 
depends() {
    echo systemd rootfs-block dm fs-lib 
    #tpm2-tss
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

    # Until this is done on immucore, we need to ship it as part of the dracut module
    inst_simple "${moddir}/kairos-setup-initramfs.service" "${systemdsystemunitdir}/kairos-setup-initramfs.service"
    mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
    ln_r "../kairos-setup-initramfs.service" "${systemdsystemunitdir}/initrd.target.requires/kairos-setup-initramfs.service"

    dracut_need_initqueue
}