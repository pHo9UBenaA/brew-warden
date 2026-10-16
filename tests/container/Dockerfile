# Development-only Linux checks; not evidence of macOS Homebrew compatibility.
FROM golang:1.27.2-bookworm@sha256:2bcff0c6ad6e8c5353e8c0d5c7f17bfcc87d667d0169615e34db1acc0db985b9
RUN test "$(go env GOVERSION)" = go1.27.2
WORKDIR /work
COPY --chown=10001:10001 . /work
ENV HOME=/tmp/home GOCACHE=/tmp/go-build GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off GOFLAGS=-mod=readonly CGO_ENABLED=0
USER 10001:10001
CMD ["sh", "scripts/container-check.sh"]
