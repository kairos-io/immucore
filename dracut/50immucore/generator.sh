#!/bin/bash

set +x

type getarg >/dev/null 2>&1 || . /lib/dracut-lib.sh

GENERATOR_DIR="$2"
[ -z "$GENERATOR_DIR" ] && exit 1
[ -d "$GENERATOR_DIR" ] || mkdir "$GENERATOR_DIR"

# Add a timeout to the sysroot so it waits a bit for immucore to mount it properly
mkdir -p "$GENERATOR_DIR"/sysroot.mount.d
{
    echo "[Mount]"
    echo "TimeoutSec=300"
} > "$GENERATOR_DIR"/sysroot.mount.d/timeout.conf

# Make sure initrd-root-fs.target depends on sysroot.mount
# This seems to affect mainly ubuntu-22 where initrd-usr-fs depends on sysroot, but it has a broken link to it as sysroot.mount
# is generated under the generator.early dir but the link points to the generator dir.
# So it makes everything else a bit broken if you insert deps in the middle.
# By default other distros seem to do this as it shows on the map page https://man7.org/linux/man-pages/man7/dracut.bootup.7.html
if ! [ -L "$GENERATOR_DIR"/initrd-root-fs.target.wants/sysroot.mount ]; then
  [ -d "$GENERATOR_DIR"/initrd-root-fs.target.wants ] || mkdir -p "$GENERATOR_DIR"/initrd-root-fs.target.wants
  ln -s ../sysroot.mount "$GENERATOR_DIR"/initrd-root-fs.target.wants/sysroot.mount
fi