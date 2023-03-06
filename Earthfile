VERSION 0.6
# Note the base image needs to have dracut.
# TODO: This needs to come from pre-built kernels in c3os repos, immucore included.
# Framework images should use our initrd
ARG FLAVOR=core-opensuse-leap
ARG BASE_IMAGE=quay.io/kairos/$FLAVOR
ARG OSBUILDER_IMAGE=quay.io/kairos/osbuilder-tools
ARG ISO_NAME=$FLAVOR-immucore

ARG GO_VERSION=1.18
ARG GOLINT_VERSION=v1.47.3

version:
    FROM alpine
    RUN apk add git
    COPY . ./
    RUN --no-cache echo $(git describe --tags | sed 's/\(.*\)-.*/\1/') > VERSION
    RUN --no-cache echo $(git describe --always --dirty) > COMMIT
    ARG VERSION=$(cat VERSION)
    ARG COMMIT=$(cat COMMMIT)
    SAVE ARTIFACT COMMIT COMMIT
    SAVE ARTIFACT VERSION VERSION

golang-image:
    ARG GO_VERSION
    FROM golang:$GO_VERSION
    WORKDIR /build
    COPY go.mod go.sum ./
    RUN go mod download

go-deps:
    ARG GO_VERSION
    FROM golang:$GO_VERSION
    WORKDIR /build
    COPY go.mod go.sum ./
    RUN go mod download
    RUN apt-get update
    SAVE ARTIFACT go.mod AS LOCAL go.mod
    SAVE ARTIFACT go.sum AS LOCAL go.sum

test:
    FROM +golang-image
    WORKDIR /build
    RUN go install -mod=mod github.com/onsi/ginkgo/v2/ginkgo
    COPY . .
    RUN ginkgo run --race --covermode=atomic --coverprofile=coverage.out -p -r ./...
    SAVE ARTIFACT coverage.out AS LOCAL coverage.out

lint:
    ARG GOLINT_VERSION
    FROM golangci/golangci-lint:$GOLINT_VERSION
    WORKDIR /build
    COPY . .
    RUN golangci-lint run

build-immucore:
    FROM +golang-image
    WORKDIR /work
    COPY go.mod go.sum /work
    COPY main.go /work
    COPY --dir internal /work
    COPY --dir pkg /work
    COPY +version/VERSION ./
    COPY +version/COMMIT ./
    ARG VERSION=$(cat VERSION)
    ARG COMMIT=$(cat COMMIT)
    ARG LDFLAGS="-s -w -X github.com/kairos-io/immucore/internal/version.version=$VERSION -X github.com/kairos-io/immucore/internal/version.gitCommit=$COMMIT"
    RUN echo ${LDFLAGS}
    RUN CGO_ENABLED=0 go build -o immucore -ldflags "${LDFLAGS}"
    SAVE ARTIFACT /work/immucore immucore AS LOCAL build/immucore-$VERSION

dracut-artifacts:
    FROM $BASE_IMAGE
    WORKDIR /build
    COPY --dir dracut/28immucore .
    COPY dracut/10-immucore.conf .
    SAVE ARTIFACT 28immucore 28immucore
    SAVE ARTIFACT 10-immucore.conf 10-immucore.conf