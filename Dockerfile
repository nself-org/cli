# syntax=docker/dockerfile:1
#
# Builds the static `nself` CLI from vendored source and ships it on a minimal
# runtime base. Multi-arch: the builder runs natively on the build platform and
# cross-compiles for the target arch (Go honours TARGETOS/TARGETARCH), so we get
# linux/amd64 + linux/arm64 without emulating the toolchain under QEMU.
#
# Consumed by .github/workflows/docker-publish.yml (docker/build-push-action,
# context: .). Build args mirror .goreleaser.yml ldflags so the image is
# identical to the released binaries. License verification uses the committed
# public keys in internal/license/keys.go, with no build-time key injection.
#
# GO_VERSION must track the `go` directive in go.mod (currently 1.26).

ARG GO_VERSION=1.26

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# Vendored deps are checked in, so the build is fully offline. Force the image's
# own toolchain (no network fetch of a newer patch release named in go.mod).
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=vendor

COPY . .

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
ARG TARGETOS
ARG TARGETARCH

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w \
        -X github.com/nself-org/cli/internal/version.Version=${VERSION} \
        -X github.com/nself-org/cli/internal/version.Commit=${COMMIT} \
        -X github.com/nself-org/cli/internal/version.BuildDate=${BUILD_DATE}" \
      -o /out/nself ./cmd/nself/

# Runtime: the image IS the CLI. nself shells out to the `docker` client (100+
# call sites, no vendored Docker API client), so backend orchestration needs the
# docker CLI + compose plugin present and the host socket mounted at run time
# (docker run -v /var/run/docker.sock:/var/run/docker.sock). Runs as root so it
# can reach that socket. Extras: ca-certificates for HTTPS license validation to
# ping.nself.org; git for `nself plugin` installs.
FROM alpine:3.20
RUN apk add --no-cache ca-certificates git docker-cli docker-cli-compose
COPY --from=build /out/nself /usr/local/bin/nself
WORKDIR /workspace
ENTRYPOINT ["nself"]
CMD ["--help"]
