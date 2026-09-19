# Development-only Linux checks; not evidence of macOS Homebrew compatibility.
FROM golang:1.27.1-bookworm@sha256:1668bbf856490c9bd0b77204ae1cdaebe273dc459b5eeb3a31a4ddc78ca6fae1
RUN test "$(go env GOVERSION)" = go1.27.1
WORKDIR /work
COPY --chown=10001:10001 . /work
ENV HOME=/tmp/home GOCACHE=/tmp/go-build GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS=-mod=readonly CGO_ENABLED=0
USER 10001:10001
CMD ["sh", "scripts/container-check.sh"]
