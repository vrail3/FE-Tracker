# FE-Tracker

A robust stock tracking service for NVIDIA Founders Edition graphics cards with real-time notifications.

## Features

- Real-time stock monitoring
- SKU change detection
- Configurable check intervals
- Ntfy.sh notifications for SKU changes, stock availability, and errors
- HTTP status endpoint
- Docker support
- Error rate monitoring
- Health checks
- 24-hour metrics tracking

## Quick Start

1. Get the docker-compose.yml file:

   ```bash
   curl -O https://raw.githubusercontent.com/vrail3/fe-tracker/main/docker-compose.yml
   ```

2. Update environment variables in docker-compose.yml:

   ```yaml
   environment:
     NVIDIA_PRODUCT_URL: "https://marketplace.nvidia.com/de-de/consumer/graphics-cards/nvidia-geforce-rtx-5080/"
     STOCK_CHECK_INTERVAL: "1000"  # milliseconds
     SKU_CHECK_INTERVAL: "10000"   # milliseconds
     NTFY_TOPIC: "your-topic"
   ```

3. Run with Docker:

   ```bash
   docker compose up -d
   ```

## Status Monitoring

Check service status at `http://localhost/status`:

```json
{
  "status": "running",
  "uptime": "1h2m15s",
  "metrics": {
    "current_sku": "RTX5080-FE",
    "error_count_24h": 5,
    "api_requests_24h": 1234,
    "ntfy_messages_sent": 3,
    "start_time": "2024-02-11T15:04:05Z",
    "last_status_check": "2024-02-11T15:04:05Z"
  }
}
```
