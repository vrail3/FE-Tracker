# syntax=docker/dockerfile:1.4
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS builder
WORKDIR /src
RUN apk --no-cache add ca-certificates tzdata

# Copy all necessary files
COPY main.go .
COPY templates/ templates/
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

# Final stage
FROM scratch
WORKDIR /app
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo/ /usr/share/zoneinfo/
COPY --from=builder /src/templates/ templates/
COPY --from=compressor /app/fe-tracker .

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/fe-tracker", "-health-check"]
ENTRYPOINT ["/app/fe-tracker"]