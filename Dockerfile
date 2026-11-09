# Build stage: compile the Doko frontend binary
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

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
