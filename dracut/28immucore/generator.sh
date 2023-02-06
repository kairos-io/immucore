#!/bin/bash

type getarg >/dev/null 2>&1 || . /lib/dracut-lib.sh

GENERATOR_DIR="$2"

[ -z "$GENERATOR_DIR" ] && exit 1
[ -d "$GENERATOR_DIR" ] || mkdir "$GENERATOR_DIR"

oem_label=$(getarg rd.cos.oemlabel=)

# See https://github.com/kairos-io/packages/blob/d12b12b043a71d8471454f7b4fc84c3181d2bf60/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-generator.sh#L29
{
    echo "[Unit]"
    echo "DefaultDependencies=no"
    echo "After=initrd-root-fs.target"
    echo "Requires=initrd-root-fs.target"
    echo "Conflicts=initrd-switch-root.target"
    echo "[Service]"
    echo "Type=oneshot"
    echo "RemainAfterExit=no"
    echo "ExecStart=/usr/bin/immucore start"

    echo "[Install]"
    echo "RequiredBy=initrd-fs.target"
} > "$GENERATOR_DIR"/immucore.service


if [ ! -e "$GENERATOR_DIR/initrd-fs.target.requires/immucore.service" ]; then
    mkdir -p "$GENERATOR_DIR"/initrd-fs.target.requires
    ln -s "$GENERATOR_DIR"/immucore.service \
        "$GENERATOR_DIR"/initrd-fs.target.requires/immucore.service
fi