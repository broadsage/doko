# Build stage: compile the Doko frontend binary
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:3889b425f035be855a72fb4755265311293b6d414521f0a519d819df32222d83 AS builder

ARG TARGETOS
ARG TARGETARCH

ARG VERSION=v1.0.0-dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-X main.version=$VERSION -X main.commit=$COMMIT -X main.buildTime=$BUILD_TIME" -o /doko ./cmd/doko

RUN mkdir -p /tmp && chmod 1777 /tmp

# Final stage: minimal image with only the frontend binary
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /tmp /tmp
COPY --from=builder /doko /bin/doko

ENTRYPOINT ["/bin/doko"]
