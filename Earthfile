VERSION 0.6
# renovate: datasource=docker depName=quay.io/kairos/osbuilder-tools versioning=semver-coerced
ARG OSBUILDER_VERSION=v0.300.3
ARG OSBUILDER_IMAGE=quay.io/kairos/osbuilder-tools:$OSBUILDER_VERSION
# renovate: datasource=docker depName=golangci/golangci-lint
ARG GOLINT_VERSION=v1.61.0
# renovate: datasource=docker depName=golang
ARG GO_VERSION=1.23-bookworm

version:
    FROM +go-deps
    COPY . ./
    RUN --no-cache echo $(git describe --always --tags --dirty) > VERSION
    RUN --no-cache echo $(git describe --always --dirty) > COMMIT
    ARG VERSION=$(cat VERSION)
    ARG COMMIT=$(cat COMMIT)
    SAVE ARTIFACT VERSION VERSION
    SAVE ARTIFACT COMMIT COMMIT

go-deps:
    ARG GO_VERSION
    FROM golang:$GO_VERSION
    WORKDIR /build
    COPY . .
    RUN go mod tidy
    RUN go mod download
    RUN go mod verify

test:
    FROM +go-deps
    WORKDIR /build
    RUN go run github.com/onsi/ginkgo/v2/ginkgo --race --covermode=atomic --coverprofile=coverage.out -p -r ./...
    SAVE ARTIFACT coverage.out AS LOCAL coverage.out

golint:
    ARG GOLINT_VERSION
    FROM golangci/golangci-lint:$GOLINT_VERSION
    WORKDIR /build
    COPY . .
    RUN go mod tidy
    RUN golangci-lint run -v

build-immucore:
    FROM +go-deps
    COPY +version/VERSION ./
    COPY +version/COMMIT ./
    ARG VERSION=$(cat VERSION)
    ARG COMMIT=$(cat COMMIT)
    ARG LDFLAGS="-s -w -X github.com/kairos-io/immucore/internal/version.version=$VERSION -X github.com/kairos-io/immucore/internal/version.gitCommit=$COMMIT"
    RUN echo ${LDFLAGS}
    RUN CGO_ENABLED=0 go build -o immucore -ldflags "${LDFLAGS}"
    SAVE ARTIFACT immucore immucore AS LOCAL build/immucore-$VERSION

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
