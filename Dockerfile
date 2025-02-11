# syntax=docker/dockerfile:1.4
# Build stage - shared build environment
FROM --platform=$BUILDPLATFORM golang:1.23.6-alpine AS base
WORKDIR /src
RUN apk --no-cache add ca-certificates

# Deps stage - download and verify dependencies
FROM base AS deps
COPY go.mod go.sum ./
RUN go mod download

# Build stage - compile the application
FROM deps AS builder
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false \
    -ldflags="-w -s" \
    -o /app/fe-tracker

# UPX stage - compress the binary
FROM alpine:latest AS compressor
COPY --from=builder /app/fe-tracker /app/fe-tracker
RUN apk add --no-cache upx && \
    upx --best --lzma /app/fe-tracker

# Final stage - minimal runtime
FROM scratch
COPY --from=base /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=compressor /app/fe-tracker /fe-tracker

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/fe-tracker", "-health-check"]
ENTRYPOINT ["/fe-tracker"]
