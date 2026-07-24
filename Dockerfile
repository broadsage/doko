# Build stage: compile the Doko frontend binary
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /doko ./cmd/doko

# Final stage: minimal image with only the frontend binary
FROM alpine:3.23

COPY --from=builder /doko /bin/doko

ENTRYPOINT ["/bin/doko"]
