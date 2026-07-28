#!/usr/bin/env bash

set -euo pipefail

# This builds a test iso based of hadron base
# gets latest published hadron kairos image
# builds current immucore and copies into the image
# runs kairos-init again on the image so immucore is picked up and initramfs uses it
# uses aurora to build an iso from there

HERE="$(cd "$(dirname "$0")" && pwd)"

# Tag artifacts with the current commit sha, appending -dirty when the working
# tree has uncommitted changes. Falls back to "latest" if we can't reach git
# (e.g. building from a tarball extract).
VERSION="$(git -C "${HERE}" describe --always --dirty 2>/dev/null || echo latest)"

: "${IMAGE_TAG:=immucore-test:${VERSION}}"
: "${OUTPUT_DIR:=${HERE}/build}"
: "${ISO_NAME:=immucore-test-${VERSION}}"
: "${AURORABOOT_IMAGE:=quay.io/kairos/auroraboot:v0.25.0}"

mkdir -p "${OUTPUT_DIR}"

echo ">>> Building ${IMAGE_TAG} via Dockerfile.test"
docker build \
  -f "${HERE}/Dockerfile.test" \
  -t "${IMAGE_TAG}" \
  "${HERE}"

echo ">>> Cleaning stale artifacts in ${OUTPUT_DIR}"
rm -rf "${OUTPUT_DIR:?}/temp-rootfs" \
       "${OUTPUT_DIR:?}/${ISO_NAME}.iso" \
       "${OUTPUT_DIR:?}/${ISO_NAME}.iso.sha256"

echo ">>> Building ISO via ${AURORABOOT_IMAGE} from ${IMAGE_TAG}"
docker run --rm --privileged \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "${OUTPUT_DIR}:/out" \
  "${AURORABOOT_IMAGE}" build-iso \
    --output /out \
    --override-name "${ISO_NAME}" \
    "docker:${IMAGE_TAG}"

ls -lh "${OUTPUT_DIR}/${ISO_NAME}.iso"
echo ">>> Built ${OUTPUT_DIR}/${ISO_NAME}.iso"
