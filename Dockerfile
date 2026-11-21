# The relay as a container image: docker build --build-arg VERSION=1.2.3 -t protobase-realtime .
# The Go version follows go.mod's go line.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd cmd
COPY internal internal
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/relay ./cmd/relay

# A static binary needs no libc; the image carries CA certificates for presence reports over https and runs as a
# non-root user. It has no shell or HTTP client, so it has no HEALTHCHECK: probe the relay from outside.
FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/relay /relay
ENV RELAY_APPS_FILE=/etc/relay/apps.json
EXPOSE 4000
ENTRYPOINT ["/relay"]
