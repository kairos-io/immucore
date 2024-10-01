#!/bin/bash

# check() is called by dracut to evaluate the inclusion of a dracut module in the initramfs.
# we always want to have this module so we return 0
check() {
    return 0
}

# The function depends() should echo all other dracut module names the module depends on
depends() {
    echo rootfs-block dm fs-lib lvm
    return 0
}

# In installkernel() all kernel related files should be installed
installkernel() {
    instmods overlay
}

# custom function to check if binaries exist before calling inst_multiple
inst_check_multiple() {
    for bin in "$@"; do
        if ! command -v "$bin" >/dev/null 2>&1; then
            derror "Required binary $bin not found!"
            exit 1
        fi
    done
    inst_multiple "$@"
}


# The install() function is called to install everything non-kernel related.
install() {
    declare moddir=${moddir}
    declare systemdutildir=${systemdutildir}
    declare systemdsystemunitdir=${systemdsystemunitdir}

    inst_check_multiple immucore kairos-agent
    # add utils used by yip stages
    inst_check_multiple partprobe sync udevadm parted mkfs.ext2 mkfs.ext3 mkfs.ext4 mkfs.vfat mkfs.fat blkid lsblk e2fsck resize2fs mount umount sgdisk rsync cryptsetup growpart sfdisk gawk awk

    # Install libraries needed by gawk
    inst_libdir_file "libsigsegv.so*"
    inst_libdir_file "libmpfr.so*"

    # missing mkfs.xfs xfs_growfs in image?
    inst_script "${moddir}/generator.sh" "${systemdutildir}/system-generators/immucore-generator"
    # SERVICES FOR SYSTEMD-BASED SYSTEMS
    inst_simple "${moddir}/immucore.service" "${systemdsystemunitdir}/immucore.service"
    mkdir -p "${initdir}/${systemdsystemunitdir}/initrd.target.requires"
    ln_r "../immucore.service" "${systemdsystemunitdir}/initrd.target.requires/immucore.service"
    # END SYSTEMD SERVICES

    dracut_need_initqueue
}
