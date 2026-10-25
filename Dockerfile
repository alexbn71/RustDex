# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/rustdex . \
 && mkdir -p /out/data /out/tmp && chmod 1777 /out/tmp

FROM scratch
COPY --from=build /out/rustdex /rustdex
COPY --from=build /out/tmp /tmp
COPY --from=build --chown=65532:65532 /out/data /data
# Starts as root only to hand /data to PUID:PGID, then switches to that user.
ENV RUSTDEX_DATA_DIR=/data PUID=65532 PGID=65532
VOLUME /data
EXPOSE 21114
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["/rustdex", "healthcheck"]
ENTRYPOINT ["/rustdex"]
