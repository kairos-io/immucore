VERSION 0.6
# Note the base image needs to have dracut.
# TODO: This needs to come from pre-built kernels in c3os repos, immucore included.
# Framework images should use our initrd
ARG BASE_IMAGE=quay.io/kairos/core-opensuse-leap

build-immucore:
    FROM golang:alpine
    RUN apk add git
    COPY . /work
    WORKDIR /work
    ARG VERSION="$(git describe --tags)"
    RUN CGO_ENABLED=0 go build -o immucore -ldflags "-X main.Version=$VERSION"
    SAVE ARTIFACT /work/immucore AS LOCAL immucore

build-dracut:
    FROM $BASE_IMAGE
    COPY . /work
    COPY +build-immucore/immucore /usr/bin/immucore
    WORKDIR /work
    RUN cp -r dracut/28immucore /usr/lib/dracut/modules.d
    RUN cp dracut/dracut.conf /etc/dracut.conf.d/10-immucore.conf
    RUN kernel=$(ls /lib/modules | head -n1) && \
        dracut -f "/boot/initrd-${kernel}" "${kernel}" && \
        ln -sf "initrd-${kernel}" /boot/initrd
    ARG INITRD=$(readlink -f /boot/initrd)
    SAVE ARTIFACT $INITRD AS LOCAL initrd

image:
    FROM $BASE_IMAGE
    ARG IMAGE=dracut
    ARG INITRD=$(readlink -f /boot/initrd)
    ARG NAME=$(basename $INITRD)
    COPY +build-dracut/$NAME $INITRD
    # For initrd use
    COPY +build-immucore/immucore /usr/bin/immucore
    RUN ln -s /usr/lib/systemd/systemd /init

    SAVE IMAGE $IMAGE

iso: 
    ARG ISO_NAME=test
    FROM quay.io/kairos/osbuilder-tools

    WORKDIR /build
    RUN zypper in -y jq docker wget
    RUN mkdir -p files-iso/boot/grub2
    RUN wget https://raw.githubusercontent.com/c3os-io/c3os/master/overlay/files-iso/boot/grub2/grub.cfg -O files-iso/boot/grub2/grub.cfg
    WITH DOCKER --allow-privileged --load $IMG=(+image)
        RUN /entrypoint.sh --name $ISO_NAME --debug build-iso --date=false --local --overlay-iso /build/files-iso dracut:latest --output /build/
    END
   # See: https://github.com/rancher/elemental-cli/issues/228
    RUN sha256sum $ISO_NAME.iso > $ISO_NAME.iso.sha256
    SAVE ARTIFACT /build/$ISO_NAME.iso iso AS LOCAL build/$ISO_NAME.iso
    SAVE ARTIFACT /build/$ISO_NAME.iso.sha256 sha256 AS LOCAL build/$ISO_NAME.iso.sha256