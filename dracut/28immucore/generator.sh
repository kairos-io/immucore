#!/bin/bash

set +x

type getarg >/dev/null 2>&1 || . /lib/dracut-lib.sh

GENERATOR_DIR="$2"
[ -z "$GENERATOR_DIR" ] && exit 1
[ -d "$GENERATOR_DIR" ] || mkdir "$GENERATOR_DIR"


## GENERATE SYSROOT
cos_img=$(getarg cos-img/filename=)
[ -z "${cos_img}" ] && exit 0

# This is necessary because otherwise systemd-fstab-generator will se the cmdline with the root=LABEL=X stanza and
# say, hey this is the ROOT where we need to boot! so it auto creates a sysroot.mount with the content of the value
# passed in the cmdline. But because we usually pass the label of the img (COS_ACTIVE) it will create the wrong mount
# service and be stuck in there forever.
# by generating it ourselves we get the sysroot.mount into the generators.early dir, which tells systemd to not generate it
# as it already exists and the rest is history
{
    echo "[Unit]"
    echo "Before=initrd-root-fs.target"
    echo "DefaultDependencies=no"
    echo "[Mount]"
    echo "Where=/sysroot"
    echo "What=/run/initramfs/cos-state/${cos_img#/}"
    echo "Options=ro,suid,dev,exec,auto,nouser,async"
} > "$GENERATOR_DIR"/sysroot.mount

## END GENERATE SYSROOT