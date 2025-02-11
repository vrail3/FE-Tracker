package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Add response structure
type NvidiaSearchResponse struct {
	SearchedProducts struct {
		ProductDetails []struct {
			DisplayName      string `json:"displayName"`
			IsFounderEdition bool   `json:"isFounderEdition"`
			ProductSKU       string `json:"productSKU"`
		} `json:"productDetails"`
	} `json:"searchedProducts"`
}

// Add new type for inventory response
type InventoryResponse struct {
	ListMap []struct {
		IsActive   string `json:"is_active"`
		ProductURL string `json:"product_url"`
	} `json:"listMap"`
}

// Add HTTP client with timeout
var client = &http.Client{
	Timeout: 10 * time.Second,
}

// Update Error type to include timestamp
type Error struct {
	Timestamp time.Time
	Err       error
}

type ErrorTracking struct {
	Errors     []Error
	LastNotify time.Time
	Threshold  int
	Window     time.Duration
	mu         sync.Mutex
}

// Add method to get 24h error count
func (et *ErrorTracking) get24hErrorCount() int {
	et.mu.Lock()
	defer et.mu.Unlock()

	count := 0
	now := time.Now()
	for _, err := range et.Errors {
		if now.Sub(err.Timestamp) <= 24*time.Hour {
			count++
		}
	}
	return count
}

var errorTracker = ErrorTracking{
	Threshold: 3,           // Notify after 3 errors
	Window:    time.Minute, // Within 1 minute
}

// Add new types for status tracking
type Metrics struct {
	CurrentSKU      string    `json:"current_sku"`
	ErrorCount      int       `json:"error_count"`
	ApiRequests     int       `json:"api_requests_24h"`
	NtfySent        int       `json:"ntfy_messages_sent"`
	StartTime       time.Time `json:"start_time"`
	LastStatusCheck time.Time `json:"last_status_check"`
	mu              sync.Mutex
}

var metrics = Metrics{
	StartTime: time.Now(),
}

func (m *Metrics) incrementApiRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ApiRequests++
	// Reset counter if last check was more than 24h ago
	if time.Since(m.LastStatusCheck) > 24*time.Hour {
		m.ApiRequests = 1
		m.LastStatusCheck = time.Now()
	}
}

func (m *Metrics) incrementErrors() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorCount++
}

func (m *Metrics) incrementNtfy() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NtfySent++
}

func (m *Metrics) updateSKU(sku string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentSKU = sku
}

// Update AddError method
func (et *ErrorTracking) AddError(err error) {
	metrics.incrementErrors()
	et.mu.Lock()
	defer et.mu.Unlock()

	now := time.Now()
	// Clean old errors
	recent := []Error{}
	for _, e := range et.Errors {
		if now.Sub(e.Timestamp) < et.Window {
			recent = append(recent, e)
		}
	}
	et.Errors = recent
	et.Errors = append(et.Errors, Error{Timestamp: now, Err: err})

	// Check if we should notify
	if len(et.Errors) >= et.Threshold && now.Sub(et.LastNotify) > et.Window {
		msg := fmt.Sprintf("High error rate detected!\nLast error: %v\nTotal errors in last minute: %d",
			err, len(et.Errors))
		if err := sendNtfyNotification(os.Getenv("NTFY_TOPIC"), "Error Threshold Reached", msg); err != nil {
			log.Printf("Failed to send error notification: %v", err)
		}
		et.LastNotify = now
		et.Errors = nil // Reset after notification
	}
}

// Add ntfy function
func sendNtfyNotification(topic, title, message string) error {
	metrics.incrementNtfy()
	ntfyURL := fmt.Sprintf("https://ntfy.sh/%s", topic)
	req, err := http.NewRequest("POST", ntfyURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("creating ntfy request: %v", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending ntfy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ntfy returned status: %d", resp.StatusCode)
	}
	return nil
}

func makeRequest(url string) (*NvidiaSearchResponse, error) {
	metrics.incrementApiRequests()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	// Add headers to mimic browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		errorTracker.AddError(err) // Track the error
		return nil, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var response NvidiaSearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %v", err)
	}

	return &response, nil
}

type Config struct {
	Locale             string
	GpuModel           string
	StockCheckInterval string
	SkuCheckInterval   string
	NtfyTopic          string
	ProductURL         string
	ApiURL             string
}

func loadEnvConfig() (Config, error) {
	log.Println("Loading configuration from environment...")

	// Define required variables
	envVars := map[string]string{
		"NVIDIA_PRODUCT_URL":   "",
		"STOCK_CHECK_INTERVAL": "",
		"SKU_CHECK_INTERVAL":   "",
		"NTFY_TOPIC":           "",
	}

	missingVars := []string{}

	// Check environment variables directly (no .env loading)
	for key := range envVars {
		if value := os.Getenv(key); value != "" {
			envVars[key] = value
			log.Printf("- %s: %s", key, value)
		} else {
			log.Printf("- %s: not set", key)
			missingVars = append(missingVars, key)
		}
	}

	if len(missingVars) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %v", missingVars)
	}

	// Parse URL for locale and GPU model
	re := regexp.MustCompile(`/([a-z]{2}-[a-z]{2})/.*?rtx-(\d{4})`)
	matches := re.FindStringSubmatch(strings.ToLower(envVars["NVIDIA_PRODUCT_URL"]))
	if matches == nil {
		return Config{}, fmt.Errorf("invalid URL format. Expected pattern: .../xx-xx/...rtx-XXXX")
	}

	locale, gpuModel := matches[1], matches[2]
	apiURL := fmt.Sprintf("https://api.nvidia.partners/edge/product/search?page=1&limit=12&locale=%s&gpu=RTX%%20%s",
		locale, gpuModel)

	return Config{
		Locale:             locale,
		GpuModel:           gpuModel,
		StockCheckInterval: envVars["STOCK_CHECK_INTERVAL"],
		SkuCheckInterval:   envVars["SKU_CHECK_INTERVAL"],
		NtfyTopic:          envVars["NTFY_TOPIC"],
		ProductURL:         envVars["NVIDIA_PRODUCT_URL"],
		ApiURL:             apiURL,
	}, nil
}

func sendStartupNotification(config Config) error {
	startupMsg := fmt.Sprintf(`FE Tracker Started
- Locale: %s
- GPU Model: %s
- Stock Check Interval: %s
- SKU Check Interval: %s
- Product URL: %s`,
		config.Locale,
		config.GpuModel,
		config.StockCheckInterval,
		config.SkuCheckInterval,
		config.ProductURL)

	return sendNtfyNotification(
		config.NtfyTopic,
		"FE Tracker Started",
		startupMsg,
	)
}

// Add function to check inventory
func checkInventory(sku, locale, ntfyTopic string) error {
	url := fmt.Sprintf("https://api.store.nvidia.com/partner/v1/feinventory?skus=%s&locale=%s", sku, locale)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating inventory request: %v", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("inventory request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading inventory response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("inventory check returned status: %d", resp.StatusCode)
	}

	var inventory InventoryResponse
	if err := json.Unmarshal(body, &inventory); err != nil {
		return fmt.Errorf("parsing inventory JSON: %v", err)
	}

	if len(inventory.ListMap) > 0 {
		item := inventory.ListMap[0]
		if item.IsActive != "false" {
			msg := fmt.Sprintf(`RTX %s IN STOCK!
SKU: %s

Direct purchase link:
%s

`,
				sku,
				sku,
				item.ProductURL)

			log.Print(msg)
			return sendNtfyNotification(ntfyTopic, "STOCK FOUND!", msg)
		}
	}

	return nil
}

// Update checkSkuStatus to also check inventory when SKU is found
func checkSkuStatus(config Config) error {
	response, err := makeRequest(config.ApiURL)
	if err != nil {
		return fmt.Errorf("API request failed: %v", err)
	}

	foundFE := false
	for _, product := range response.SearchedProducts.ProductDetails {
		if product.IsFounderEdition && strings.Contains(product.DisplayName, config.GpuModel) {
			foundFE = true
			metrics.updateSKU(product.ProductSKU)

			if err := checkInventory(product.ProductSKU, config.Locale, config.NtfyTopic); err != nil {
				log.Printf("Inventory check failed: %v", err)
			}
			break
		}
	}

	if !foundFE {
		log.Printf("No matching FE card found")
	}

	return nil
}

func cleanup(config Config) {
	msg := fmt.Sprintf(`FE Tracker Stopped
- Locale: %s
- GPU Model: %s`,
		config.Locale,
		config.GpuModel)

	if err := sendNtfyNotification(config.NtfyTopic, "FE Tracker Stopped", msg); err != nil {
		log.Printf("Failed to send shutdown notification: %v", err)
	} else {
		log.Printf("Shutdown notification sent successfully")
	}
}

func startMonitoring(ctx context.Context, config Config) error {
	// Convert interval strings to durations
	stockInterval, err := time.ParseDuration(config.StockCheckInterval + "ms")
	if err != nil {
		return fmt.Errorf("invalid stock check interval: %v", err)
	}

	skuInterval, err := time.ParseDuration(config.SkuCheckInterval + "ms")
	if err != nil {
		return fmt.Errorf("invalid SKU check interval: %v", err)
	}

	log.Printf("Starting monitoring (Stock: %v, SKU: %v)", stockInterval, skuInterval)

	// Create ticker for stock and SKU checks
	stockTicker := time.NewTicker(stockInterval)
	skuTicker := time.NewTicker(skuInterval)
	defer stockTicker.Stop()
	defer skuTicker.Stop()

	// Ensure cleanup runs on exit
	defer cleanup(config)

	// Monitoring loop
	for {
		select {
		case <-stockTicker.C:
			if err := checkSkuStatus(config); err != nil {
				log.Printf("Check failed: %v", err)
			}
		case <-skuTicker.C:
			if err := checkSkuStatus(config); err != nil {
				log.Printf("SKU check failed: %v", err)
			}
		case <-ctx.Done():
			log.Printf("Monitoring stopped")
			return nil
		}
	}
}

// Update handleStatus to show 24h errors
func handleStatus(w http.ResponseWriter, r *http.Request) {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	status := struct {
		Status  string `json:"status"`
		Uptime  string `json:"uptime"`
		Metrics struct {
			CurrentSKU      string    `json:"current_sku"`
			ErrorCount24h   int       `json:"error_count_24h"` // Changed from ErrorCount
			ApiRequests     int       `json:"api_requests_24h"`
			NtfySent        int       `json:"ntfy_messages_sent"`
			StartTime       time.Time `json:"start_time"`
			LastStatusCheck time.Time `json:"last_status_check"`
		} `json:"metrics"`
	}{
		Status: "running",
		Uptime: time.Since(metrics.StartTime).Round(time.Second).String(),
		Metrics: struct {
			CurrentSKU      string    `json:"current_sku"`
			ErrorCount24h   int       `json:"error_count_24h"`
			ApiRequests     int       `json:"api_requests_24h"`
			NtfySent        int       `json:"ntfy_messages_sent"`
			StartTime       time.Time `json:"start_time"`
			LastStatusCheck time.Time `json:"last_status_check"`
		}{
			CurrentSKU:      metrics.CurrentSKU,
			ErrorCount24h:   errorTracker.get24hErrorCount(), // Use new method
			ApiRequests:     metrics.ApiRequests,
			NtfySent:        metrics.NtfySent,
			StartTime:       metrics.StartTime,
			LastStatusCheck: metrics.LastStatusCheck,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(status); err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}
}

// Update performHealthCheck function
func performHealthCheck() bool {
	metrics.mu.Lock()
	defer metrics.mu.Unlock()

	// Check if we've had any API requests in the last minute
	if time.Since(metrics.LastStatusCheck) > time.Minute {
		return false
	}

	// Check if error rate is acceptable (less than 50% in the last minute)
	recentErrors := 0
	now := time.Now()
	for _, err := range errorTracker.Errors {
		if now.Sub(err.Timestamp) < time.Minute {
			recentErrors++
		}
	}

	return float64(recentErrors)/60.0 < 0.5
}

func main() {
	// Add command line flag for health check
	healthCheck := flag.Bool("health-check", false, "Perform health check and exit")
	flag.Parse()

	// Handle health check request
	if *healthCheck {
		if performHealthCheck() {
			os.Exit(0)
		}
		os.Exit(1)
	}

	config, err := loadEnvConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Send startup notification
	if err := sendStartupNotification(config); err != nil {
		log.Printf("Failed to send startup notification: %v", err)
	}

	// Create a context with cancellation for coordinated shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create WaitGroup to wait for goroutines
	var wg sync.WaitGroup

	// Setup shutdown channel
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Start monitoring in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := startMonitoring(ctx, config); err != nil {
			log.Printf("Monitoring failed: %v", err)
			cancel() // Cancel context if monitoring fails
		}
	}()

	// Setup routes - only status endpoint
	http.HandleFunc("/status", handleStatus)

	// Create HTTP server with timeout configs
	srv := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// Start HTTP server in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Starting server on :8080")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			cancel() // Cancel context if server fails
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("Shutdown signal received")

	// Cancel context to notify all goroutines
	cancel()

	// Shutdown HTTP server gracefully
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}

	// Wait for all goroutines to finish
	wg.Wait()
	log.Println("Shutdown complete")
}
