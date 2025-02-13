 _____ _____    _____ ____      _    ____ _  _______ ____  
|  ___| ____|  |_   _|  _ \    / \  / ___| |/ / ____|  _ \ 
| |_  |  _| _____| | | |_) |  / _ \| |   | ' /|  _| | |_) |
|  _| | |__|_____| | |  _ <  / ___ \ |___| . \| |___|  _ < 
|_|   |_____|    |_| |_| \_\/_/   \_\____|_|\_\_____|_| \_\

                                                                                                    
                                +##%                                                                
                             *#%%%%                                                                 
                          =*#%%*##*%                                                                
                       =+%@@@@@@@@#+                                                                
                     =#@@@@@@@@@@@@@@+---*                                                          
                  =*@@@@@@@@@@@@@@@@@@@*==-+*                                                       
               =*%@@@@@@@@@@@@@@@@@@@@@@@%++==*                                                     
          #  +%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@*++-+*                                                  
          *#@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@#*++==*                                                
        =%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%%+*#***=+#                                             
       =%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%%+++*#****#                                            
        +%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%%++++***#####                                          
        +*+#@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%#+++******#%                                           
         ****+@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%#+++********##                                         
         *******%@@@@@@@@@@@@@@@@@@@@@@@@@@@%#++***********###                                      
          #####***#@@@@@@@@@@@@@@@@@@@@@@@@@%#+**************##%                                    
            %######**@@@@@@@@@@@@@@@@@@@@@@@%=*****************##%#                                 
              %######*+**+++++++++++++++*+++===*%%%%@@@@@@@@@@@@@@@%*#+                             
                %######%%%%***+++++++++*******@@@@@@@@@@@@@@@@@@@@@@@@@%*                           
                   %###%%%#%%************+****%@@@@@@@@@@@@@@@@@@@@@@@@@@@%+                        
                     %#%@%%%%%%#**************%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@*+                     
                        %@@%%%%%%#************%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%+                   
                           @@%%%%#%%**********%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@*                 
                            %@@@%%%#%%#*******%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%+              
                               %@@%%%%%%#*****%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%*             
                                 %@@%%%##@%***%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%+             
                                   %@@%%%%#%%*%@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%**              
                                     %@%@@@%#+@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%****              
                                        %@@@%**#@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@%*###**              
                                          %@%##***@@@@@@@@@@@@@@@@@@@@@@@@@@@%*######               
                                            %####***@@@@@@@@@@@@@@@@@@@@@@@########%                
                                              ##*+****%@@@@@@@@@@@@@@@@@@########%                  
                                               ####+*#**#@@@@@@@@@@@@@@#######%%                    
                                                 ####**##**@@@@@@@@@@*######%%                      
                                                    ####*#***%@@@@%*######%%                        
                                                      ####***############%                          
                                                        ###############%                            
                                                          ###########                               
                                                             ######                                 
                                                                                                    

A robust stock tracking service for NVIDIA Founders Edition graphics cards with real-time notifications and mobile-friendly interface.

## Features

- Real-time stock monitoring with live updates
- SKU change detection with browser notifications
- Configurable check intervals for stock and SKU monitoring
- Ntfy.sh notifications for:
  - SKU changes
  - Stock availability
  - Error rate thresholds
  - Daily status reports
- Web interface with:
  - Mobile-responsive design
  - Dark/Light theme support
  - Auto-open purchase URLs
  - Real-time metric updates
  - Connection status monitoring
- HTTP status endpoint for monitoring
- Docker support with multi-arch builds
- Error rate monitoring with cooldown
- Automatic reconnection handling
- Memory-optimized event streaming
- Health checks with Docker integration
- 24-hour metrics tracking for:
  - API requests
  - Error counts
  - Notifications sent

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
     TZ: "Europe/Berlin"           # your timezone
   ```

3. Run with Docker:

   ```bash
   docker compose up -d
   ```

## Web Interface

Access the web interface at `http://localhost/`:

- Real-time status updates
- Mobile-friendly design
- Theme toggle (Dark/Light)
- Auto-open toggle for purchase URLs (browser permission required)
- Live metrics display
- Connection status indicator
- Responsive layout for all devices

## Status API

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
    "last_status_check": "2024-02-11T15:04:05Z",
    "purchase_url": ""
  }
}
```

## Browser Notifications

The web interface supports desktop notifications for:

- Stock availability
- SKU changes
- Connection issues
- Purchase URL updates

Enable notifications when prompted for real-time alerts.
