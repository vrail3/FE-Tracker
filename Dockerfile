# syntax=docker/dockerfile:1.4
FROM --platform=$BUILDPLATFORM golang:1.23.6-alpine AS builder
WORKDIR /src
#RUN apk --no-cache add ca-certificates

# Initialize module and build
COPY main.go .
RUN go mod init fe-tracker

# Build the application
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false \
    -ldflags="-w -s" \
    -o /app/fe-tracker

# Compression stage
FROM alpine:latest AS compressor
RUN apk --no-cache add upx
COPY --from=builder /app/fe-tracker /app/fe-tracker
RUN upx --best --lzma /app/fe-tracker

# Final stage, use full debian image for compatibility
FROM debian:bullseye-slim
#COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=compressor /app/fe-tracker /fe-tracker

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/fe-tracker", "-health-check"]
ENTRYPOINT ["/fe-tracker"]