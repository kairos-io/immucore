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
    inst_multiple \
        immucore elemental

    inst_script "${moddir}/generator.sh" \
        "${systemdutildir}/system-generators/immucore-generator"

    dracut_need_initqueue
}