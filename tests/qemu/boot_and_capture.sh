#!/usr/bin/env bash
# Boot an immucore test ISO in QEMU with a custom kernel cmdline and capture
# evidence: full serial console, /run/immucore/immucore.log, journalctl -b,
# and a PNG screendump if the boot hits a red failure screen.
#
# Env knobs (all optional unless stated):
#   ISO          test ISO path (default: newest build/immucore-test-*.iso)
#   CMDLINE      extra cmdline tokens injected into the first grub entry,
#                e.g. "kairos.ram.create_partitions rd.immucore.debug"
#                (default: "rd.immucore.debug")
#   USERDATA     path to a #cloud-config file; when set, a cidata (NoCloud)
#                ISO is built and attached as a second cdrom so the datasource
#                stage ingests it (use it to create a login user)
#   DISK         existing qcow2 to attach (default: fresh empty 2G)
#   LOGIN_USER / LOGIN_PASS
#                when set, log in on the serial getty and dump immucore +
#                journal logs into LOGDIR
#   CHECK_CMD    extra shell command run as root after login (output fenced
#                into LOGDIR/check.log)
#   LOGDIR       output dir (default: <repo>/build/logs)
#   TIMEOUT      seconds to wait for login prompt / failure screen (default 300)
#
# Exit codes: 0 boot ok (and login ok if requested), 2 failure screen seen,
# 1 anything else.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "${HERE}/../.." && pwd)"

: "${CMDLINE:=rd.immucore.debug}"
: "${LOGDIR:=${ROOT}/build/logs}"
: "${TIMEOUT:=300}"
: "${SERIAL_PORT:=45601}"
: "${MONITOR_PORT:=45602}"

if [[ -z "${ISO:-}" ]]; then
  ISO="$(ls -t "${ROOT}"/build/immucore-test-*.iso 2>/dev/null | head -1 || true)"
fi
[[ -f "${ISO:-}" ]] || { echo "!!! no test ISO found; run tests/build_test_iso.sh first or set ISO="; exit 1; }

WORK="$(mktemp -d)"
# the || true matters: a failing command in an EXIT trap under set -e
# overwrites the script's exit code
trap 'kill -9 "${qpid:-}" 2>/dev/null || true; rm -rf "$WORK"' EXIT
mkdir -p "$LOGDIR"

# --- inject cmdline into the first grub entry -------------------------------
iso="$WORK/test.iso"
cp "$ISO" "$iso"
xorriso -indev "$iso" -osirrox on -extract /boot/grub2/grub.cfg "$WORK/grub.cfg" >/dev/null 2>&1
sed -e 's/set timeout=10/set timeout=1/' \
    -e "0,/rd.cos.disable/{s|rd.cos.disable |${CMDLINE} |}" \
    -e '0,/install-mode /s|install-mode ||' \
    -e '0,/cdroot /s|cdroot ||' \
    "$WORK/grub.cfg" > "$WORK/grub.cfg.new"
xorriso -boot_image any keep -dev "$iso" -map "$WORK/grub.cfg.new" /boot/grub2/grub.cfg -commit >/dev/null 2>&1

# --- optional cidata ISO from USERDATA ---------------------------------------
CIDATA_ARGS=()
if [[ -n "${USERDATA:-}" ]]; then
  [[ -f "$USERDATA" ]] || { echo "!!! USERDATA file not found: $USERDATA"; exit 1; }
  mkdir "$WORK/cidata"
  cp "$USERDATA" "$WORK/cidata/user-data"
  printf 'instance-id: immucore-qemu-test\n' > "$WORK/cidata/meta-data"
  xorriso -as mkisofs -V cidata -J -R -o "$WORK/cidata.iso" "$WORK/cidata" >/dev/null 2>&1
  CIDATA_ARGS=(-drive "file=$WORK/cidata.iso,format=raw,if=ide,media=cdrom,readonly=on")
fi

# --- disk ---------------------------------------------------------------------
if [[ -z "${DISK:-}" ]]; then
  DISK="$WORK/disk.qcow2"
  qemu-img create -f qcow2 "$DISK" 2G >/dev/null
fi

vars="$WORK/ovmf_vars.fd"
cp -f /usr/share/OVMF/x64/OVMF_VARS.4m.fd "$vars"

echo ">>> booting ${ISO##*/} with cmdline: ${CMDLINE}"
qemu-system-x86_64 \
  -enable-kvm -cpu host -m 2G -smp 2 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/OVMF/x64/OVMF_CODE.4m.fd \
  -drive "if=pflash,format=raw,file=${vars}" \
  -drive "file=${iso},format=raw,if=ide,media=cdrom,readonly=on" \
  "${CIDATA_ARGS[@]}" \
  -drive "file=${DISK},format=qcow2,if=virtio" \
  -boot d -vga std -display none \
  -monitor "telnet:127.0.0.1:${MONITOR_PORT},server,nowait" \
  -serial "telnet:127.0.0.1:${SERIAL_PORT},server,nowait" \
  &
qpid=$!

sleep 2
rc=0
TIMEOUT="$TIMEOUT" LOGIN_USER="${LOGIN_USER:-}" LOGIN_PASS="${LOGIN_PASS:-}" \
CHECK_CMD="${CHECK_CMD:-}" SCREENDUMP="$LOGDIR/failure-screen" \
python3 "$HERE/login_check.py" "$SERIAL_PORT" "$MONITOR_PORT" "$WORK/serial.raw.log" || rc=$?
kill -9 "$qpid" 2>/dev/null || true
wait "$qpid" 2>/dev/null || true

# --- split fenced dumps out of the serial capture -----------------------------
tr -d '\r' < "$WORK/serial.raw.log" | strings > "$LOGDIR/boot-serial.log"
awk '/BEGIN_IMMUCORE_LOG/{f=1;next} /END_IMMUCORE_LOG/{f=0} f' "$LOGDIR/boot-serial.log" > "$LOGDIR/immucore.log"
awk '/BEGIN_JOURNAL/{f=1;next} /END_JOURNAL/{f=0} f'           "$LOGDIR/boot-serial.log" > "$LOGDIR/journalctl.log"
awk '/BEGIN_CHECK/{f=1;next} /END_CHECK/{f=0} f'               "$LOGDIR/boot-serial.log" > "$LOGDIR/check.log"
if [[ -f "$LOGDIR/failure-screen.ppm" ]]; then
  command -v magick >/dev/null && magick "$LOGDIR/failure-screen.ppm" "$LOGDIR/failure-screen.png" && rm -f "$LOGDIR/failure-screen.ppm"
fi
find "$LOGDIR" -maxdepth 1 -empty -delete
echo ">>> exit=$rc; evidence in $LOGDIR:"
ls -la "$LOGDIR"
exit "$rc"
