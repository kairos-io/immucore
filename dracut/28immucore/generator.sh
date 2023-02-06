#!/bin/bash

set +x

type getarg >/dev/null 2>&1 || . /lib/dracut-lib.sh

GENERATOR_DIR="$2"
[ -z "$GENERATOR_DIR" ] && exit 1
[ -d "$GENERATOR_DIR" ] || mkdir "$GENERATOR_DIR"


## GENERATE SYSROOT
cos_img=$(getarg cos-img/filename=)
[ -z "${cos_img}" ] && exit 0

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