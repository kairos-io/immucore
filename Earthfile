VERSION 0.6
# renovate: datasource=docker depName=quay.io/kairos/osbuilder-tools versioning=semver-coerced
ARG OSBUILDER_VERSION=v0.11.1
ARG OSBUILDER_IMAGE=quay.io/kairos/osbuilder-tools:$OSBUILDER_VERSION
# renovate: datasource=docker depName=golang
ARG GO_VERSION=1.20
# renovate: datasource=docker depName=golangci/golangci-lint
ARG GOLINT_VERSION=v1.55.2

version:
    FROM alpine
    RUN apk add git
    COPY . ./
    RUN --no-cache echo $(git describe --tags | sed 's/\(.*\)-.*/\1/') > VERSION
    RUN --no-cache echo $(git describe --always --dirty) > COMMIT
    ARG VERSION=$(cat VERSION)
    ARG COMMIT=$(cat COMMIT)
    SAVE ARTIFACT COMMIT COMMIT
    SAVE ARTIFACT VERSION VERSION

golang-image:
    ARG GO_VERSION
    FROM golang:$GO_VERSION
    WORKDIR /build
    COPY go.mod go.sum ./
    RUN go mod download

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
    RUN golangci-lint run -v

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

# Alias for ease of use
build:
    BUILD +build-immucore

dracut-artifacts:
    FROM scratch
    WORKDIR /build
    COPY --dir dracut/28immucore .
    COPY dracut/10-immucore.conf .
    SAVE ARTIFACT 28immucore 28immucore
    SAVE ARTIFACT 10-immucore.conf 10-immucore.conf