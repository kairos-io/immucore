#!/bin/bash

# called by dracut
check() {
    # Always include this module
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
    # systemd path
    if dracut_module_included "systemd"; then
      inst_script "${moddir}/generator.sh" "${systemdutildir}/system-generators/immucore-generator"
      inst_simple "${moddir}/immucore.service" "${systemdsystemunitdir}/immucore.service"
      mkdir -p "${initdir}/${systemdsystemunitdir}/initrd-fs.target.requires"
      ln_r "../immucore.service" "${systemdsystemunitdir}/initrd-fs.target.requires/immucore.service"

      # Until this is done on immucore, we need to ship it as part of the dracut module
      inst_simple "${moddir}/kairos-setup-initramfs.service" "${systemdsystemunitdir}/kairos-setup-initramfs.service"
      mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
      ln_r "../kairos-setup-initramfs.service" "${systemdsystemunitdir}/initrd.target.requires/kairos-setup-initramfs.service"
    else
      inst_simple "${moddir}/immucore.rc" "/etc/init.d/immucore.rc"
      # No idea how to force it to start?
    fi

    dracut_need_initqueue
}