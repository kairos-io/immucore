#!/bin/bash

set +x

type getarg >/dev/null 2>&1 || . /lib/dracut-lib.sh

GENERATOR_DIR="$2"
[ -z "$GENERATOR_DIR" ] && exit 1
[ -d "$GENERATOR_DIR" ] || mkdir "$GENERATOR_DIR"

cos_img=$(getarg cos-img/filename=)
[ -z "${cos_img}" ] && exit 0

# Add a timeout to the sysroot so it waits a bit for immucore to mount it properly
mkdir -p "$GENERATOR_DIR"/sysroot.mount.d
{
    echo "[Mount]"
    echo "TimeoutSec=300"
} > "$GENERATOR_DIR"/sysroot.mount.d/timeout.conf

## END GENERATE SYSROOT