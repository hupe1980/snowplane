# Build stage
FROM golang:1.25-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" \
    -o manager ./cmd/manager

# Runtime stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
