package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
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
	Errors          []Error
	LastNotify      time.Time
	Threshold       int
	Window          time.Duration
	mu              sync.Mutex
	lastErrorNotify time.Time // Add new field for error message cooldown
	maxErrors       int       // Add maximum number of errors to store
}

var errorTracker = ErrorTracking{
	Threshold: 3,           // Notify after 3 errors
	Window:    time.Minute, // Within 1 minute
	maxErrors: 1000,        // Limit error history
}

// Add method to get 24h error count
func (et *ErrorTracking) get24hErrorCount() int {
	et.mu.Lock()
	defer et.mu.Unlock()

	count := 0
	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)

	// Only count errors from last 24 hours
	for _, err := range et.Errors {
		if err.Timestamp.After(dayAgo) {
			count++
		}
	}

	return count
}

// Add new types for status tracking
type Metrics struct {
	CurrentSKU      string      `json:"current_sku"`
	ErrorCount      int         `json:"error_count"`
	ApiRequests     int         `json:"api_requests_24h"`
	NtfySent        int         `json:"ntfy_messages_sent"`
	StartTime       time.Time   `json:"start_time"`
	LastStatusCheck time.Time   `json:"last_status_check"`
	PurchaseURL     string      `json:"purchase_url"`
	ApiRequestTimes []time.Time // Add this field
	mu              sync.Mutex
}

// Initialize metrics with local time
var metrics = Metrics{
	StartTime:       time.Now(),
	LastStatusCheck: time.Now(),
	ApiRequestTimes: make([]time.Time, 0),
}

func (m *Metrics) incrementApiRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Remove requests older than 24 hours
	recent := make([]time.Time, 0)
	for _, t := range m.ApiRequestTimes {
		if now.Sub(t) <= 24*time.Hour {
			recent = append(recent, t)
		}
	}

	// Add new request
	recent = append(recent, now)
	m.ApiRequestTimes = recent
	m.ApiRequests = len(recent)
	m.LastStatusCheck = now
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

// Add method to update purchase URL
func (m *Metrics) updatePurchaseURL(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PurchaseURL = url
}

// Simplify updateLastCheck
func (m *Metrics) updateLastCheck() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastStatusCheck = time.Now()
}

// Update AddError method with cooldown
func (et *ErrorTracking) AddError(err error) {
	metrics.incrementErrors()
	et.mu.Lock()
	defer et.mu.Unlock()

	now := time.Now()
	dayAgo := now.Add(-24 * time.Hour)

	// Keep only errors from last 24 hours
	recent := make([]Error, 0)
	for _, e := range et.Errors {
		if e.Timestamp.After(dayAgo) {
			recent = append(recent, e)
		}
	}

	// Add new error
	recent = append(recent, Error{Timestamp: now, Err: err})
	et.Errors = recent

	// Check notification threshold within last minute
	recentCount := 0
	minuteAgo := now.Add(-time.Minute)
	for _, e := range recent {
		if e.Timestamp.After(minuteAgo) {
			recentCount++
		}
	}

	if recentCount >= et.Threshold && now.Sub(et.LastNotify) > et.Window {
		if now.Sub(et.lastErrorNotify) > time.Minute {
			msg := fmt.Sprintf("High error rate detected!\nLast error: %v\nTotal errors in last minute: %d",
				err, recentCount)
			if err := sendNtfyNotification("Error Threshold Reached", msg, 4); err != nil {
				log.Printf("Failed to send error notification: %v", err)
			}
			et.lastErrorNotify = now
			et.LastNotify = now
		}
	}
}

// Add global ntfy topic
var ntfyTopic string

// Add daily report time constant
const DAILY_REPORT_TIME = "09:00"

// Add template caching
var templates = template.Must(template.ParseFiles("templates/status.html"))

// Update ntfy function to handle priorities
func sendNtfyNotification(title, message string, priority int) error {
	metrics.incrementNtfy()
	ntfyURL := fmt.Sprintf("https://ntfy.sh/%s", ntfyTopic)
	req, err := http.NewRequest("POST", ntfyURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("creating ntfy request: %v", err)
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", fmt.Sprintf("%d", priority))
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

// Update makeRequest to accept context and timezone
func makeRequest(ctx context.Context, url string) (*NvidiaSearchResponse, error) {
	metrics.incrementApiRequests()
	metrics.updateLastCheck()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
	ProductURL         string
	ApiURL             string
}

// Remove timezone loading from loadEnvConfig
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
		ProductURL:         envVars["NVIDIA_PRODUCT_URL"],
		ApiURL:             apiURL,
	}, nil
}

func sendStartupNotification(config Config) error {
	startupMsg := fmt.Sprintf(`- Locale: %s
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
		"FE Tracker Started",
		startupMsg,
		3,
	)
}

// Update checkInventory to accept context and timezone
func checkInventory(ctx context.Context, sku, locale string) error {
	metrics.updateLastCheck() // Add this line
	url := fmt.Sprintf("https://api.store.nvidia.com/partner/v1/feinventory?skus=%s&locale=%s", sku, locale)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
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
			// Update purchase URL in metrics
			metrics.updatePurchaseURL(item.ProductURL)

			msg := fmt.Sprintf(`RTX %s IN STOCK!
SKU: %s

Direct purchase link:
%s

`,
				sku,
				sku,
				item.ProductURL)

			log.Print(msg)
			return sendNtfyNotification("STOCK FOUND!", msg, 5) // Highest priority
		}
	}

	// Clear purchase URL if not available
	metrics.updatePurchaseURL("")
	return nil
}

// Update checkSkuStatus to accept and use context and timezone
func checkSkuStatus(ctx context.Context, config Config) error {
	response, err := makeRequest(ctx, config.ApiURL)
	if err != nil {
		return fmt.Errorf("API request failed: %v", err)
	}

	foundFE := false
	for _, product := range response.SearchedProducts.ProductDetails {
		if product.IsFounderEdition && strings.Contains(product.DisplayName, config.GpuModel) {
			foundFE = true
			metrics.updateSKU(product.ProductSKU)

			if err := checkInventory(ctx, product.ProductSKU, config.Locale); err != nil {
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
	msg := fmt.Sprintf(`- Locale: %s
- GPU Model: %s`,
		config.Locale,
		config.GpuModel)

	if err := sendNtfyNotification("FE Tracker Stopped", msg, 3); err != nil {
		log.Printf("Failed to send shutdown notification: %v", err)
	} else {
		log.Printf("Shutdown notification sent successfully")
	}
}

// Add last report time tracking
var (
	lastReportTime time.Time
	reportMutex    sync.Mutex
)

// Add simple duration formatter with spaces
func simpleDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		if hours > 0 {
			if minutes > 0 {
				return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
			}
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}

	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}

	return "just now"
}

// Update daily status report function to prevent duplicates
func sendDailyReport() {
	reportMutex.Lock()
	now := time.Now()
	today := now.Format("2006-01-02")
	lastReport := lastReportTime.Format("2006-01-02")

	// Only send if we haven't sent a report today
	if today == lastReport {
		reportMutex.Unlock()
		return
	}

	// Update last report time before sending
	lastReportTime = now
	reportMutex.Unlock()

	metrics.mu.Lock()
	report := fmt.Sprintf(`- Uptime: %s
- Current SKU: %s
- API Requests (24h): %d
- Errors (24h): %d
- Notifications Sent: %d`,
		simpleDuration(time.Since(metrics.StartTime)),
		metrics.CurrentSKU,
		metrics.ApiRequests,
		errorTracker.get24hErrorCount(),
		metrics.NtfySent,
	)
	metrics.mu.Unlock()

	if err := sendNtfyNotification("Status Report", report, 3); err != nil {
		log.Printf("Failed to send daily report: %v", err)
	} else {
		log.Printf("Daily report sent successfully")
	}
}

// Update daily report check to use timezone
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

	// Create error channel for goroutine errors
	errChan := make(chan error, 1)

	// Ensure cleanup runs on exit
	defer cleanup(config)

	// Add daily report ticker with timezone
	reportTicker := time.NewTicker(time.Minute)
	defer reportTicker.Stop()

	go func() {
		for {
			now := time.Now()
			currentTime := now.Format("15:04")
			if currentTime == DAILY_REPORT_TIME {
				sendDailyReport()
			}
			<-reportTicker.C
		}
	}()

	// Monitoring loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			return fmt.Errorf("monitoring error: %v", err)
		case <-stockTicker.C:
			// Use goroutine for stock check to prevent blocking
			go func() {
				if err := checkSkuStatus(ctx, config); err != nil {
					select {
					case errChan <- err:
					default:
						log.Printf("Check failed: %v", err)
					}
				}
			}()
		case <-skuTicker.C:
			// Use goroutine for SKU check to prevent blocking
			go func() {
				if err := checkSkuStatus(ctx, config); err != nil {
					select {
					case errChan <- err:
					default:
						log.Printf("SKU check failed: %v", err)
					}
				}
			}()
		}
	}
}

// Update handleStatus to properly initialize the status struct with current data
func handleStatus(w http.ResponseWriter, r *http.Request) {
	metrics.mu.Lock()
	status := struct {
		Status  string `json:"status"`
		Uptime  string `json:"uptime"`
		Metrics struct {
			CurrentSKU      string    `json:"current_sku"`
			ErrorCount24h   int       `json:"error_count_24h"`
			ApiRequests     int       `json:"api_requests_24h"`
			NtfySent        int       `json:"ntfy_messages_sent"`
			StartTime       time.Time `json:"start_time"`
			LastStatusCheck time.Time `json:"last_status_check"`
			PurchaseURL     string    `json:"purchase_url"`
		} `json:"metrics"`
	}{
		Status: "running",
		Uptime: simpleDuration(time.Since(metrics.StartTime)),
		Metrics: struct {
			CurrentSKU      string    `json:"current_sku"`
			ErrorCount24h   int       `json:"error_count_24h"`
			ApiRequests     int       `json:"api_requests_24h"`
			NtfySent        int       `json:"ntfy_messages_sent"`
			StartTime       time.Time `json:"start_time"`
			LastStatusCheck time.Time `json:"last_status_check"`
			PurchaseURL     string    `json:"purchase_url"`
		}{
			CurrentSKU:      metrics.CurrentSKU,
			ErrorCount24h:   errorTracker.get24hErrorCount(),
			ApiRequests:     metrics.ApiRequests,
			NtfySent:        metrics.NtfySent,
			StartTime:       metrics.StartTime,
			LastStatusCheck: metrics.LastStatusCheck,
			PurchaseURL:     metrics.PurchaseURL,
		},
	}
	metrics.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// Add connection tracking
var (
	activeConnections sync.Map
	connectionID      uint64
	connectionMutex   sync.Mutex
)

// Update handleEvents to track connections
func handleEvents(w http.ResponseWriter, r *http.Request) {
	// Generate unique connection ID
	connectionMutex.Lock()
	connID := atomic.AddUint64(&connectionID, 1)
	connectionMutex.Unlock()

	// Store connection in active connections map
	activeConnections.Store(connID, time.Now())
	defer activeConnections.Delete(connID)

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	// Increase read timeout for the specific connection
	if _, ok := w.(http.Flusher); !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Create channel for client disconnect detection
	done := r.Context().Done()

	// Create ticker for updates with shorter intervals
	ticker := time.NewTicker(1 * time.Second)      // More frequent updates
	pingTicker := time.NewTicker(10 * time.Second) // Keep ping interval
	defer ticker.Stop()
	defer pingTicker.Stop()

	// Initial connection message and data
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	w.(http.Flusher).Flush()

	// Immediately send first data update
	sendStatusUpdate(w)

	// Create cleanup ticker
	cleanupTicker := time.NewTicker(30 * time.Second)
	defer cleanupTicker.Stop()

	// Create done channel for cleanup
	doneCleanup := make(chan struct{})
	defer close(doneCleanup)

	// Start cleanup goroutine
	go func() {
		for {
			select {
			case <-doneCleanup:
				return
			case <-cleanupTicker.C:
				runtime.GC() // Suggest garbage collection
			}
		}
	}()

	// Send updates with connection monitoring
	for {
		select {
		case <-done:
			return
		case <-cleanupTicker.C:
			// Update last active time
			activeConnections.Store(connID, time.Now())
		case <-pingTicker.C:
			if err := sendPing(w); err != nil {
				return
			}
		case <-ticker.C:
			if err := sendStatusUpdate(w); err != nil {
				return
			}
		}
	}
}

// Helper function to send status update
func sendStatusUpdate(w http.ResponseWriter) error {
	metrics.mu.Lock()
	status := struct {
		Status  string `json:"status"`
		Uptime  string `json:"uptime"`
		Metrics struct {
			CurrentSKU      string    `json:"current_sku"`
			ErrorCount24h   int       `json:"error_count_24h"`
			ApiRequests     int       `json:"api_requests_24h"`
			NtfySent        int       `json:"ntfy_messages_sent"`
			StartTime       time.Time `json:"start_time"`
			LastStatusCheck time.Time `json:"last_status_check"`
			PurchaseURL     string    `json:"purchase_url,omitempty"`
		} `json:"metrics"`
	}{
		Status: "running",
		Uptime: simpleDuration(time.Since(metrics.StartTime)),
	}

	// Copy metrics data
	status.Metrics.CurrentSKU = metrics.CurrentSKU
	status.Metrics.ErrorCount24h = errorTracker.get24hErrorCount()
	status.Metrics.ApiRequests = metrics.ApiRequests
	status.Metrics.NtfySent = metrics.NtfySent
	status.Metrics.StartTime = metrics.StartTime
	status.Metrics.LastStatusCheck = metrics.LastStatusCheck
	status.Metrics.PurchaseURL = metrics.PurchaseURL
	metrics.mu.Unlock()

	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("marshal error: %v", err)
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("write error: %v", err)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// Add helper functions for sending data
func sendPing(w http.ResponseWriter) error {
	_, err := fmt.Fprint(w, ": ping\n\n")
	if err != nil {
		return err
	}
	w.(http.Flusher).Flush()
	return nil
}

// Update performHealthCheck function
func performHealthCheck() bool {
	metrics.mu.Lock()
	lastCheck := metrics.LastStatusCheck
	metrics.mu.Unlock()

	timeSinceLastCheck := time.Since(lastCheck)
	if timeSinceLastCheck > 5*time.Minute {
		log.Printf("Health check failed: No activity in %.1f minutes",
			timeSinceLastCheck.Minutes())
		return false
	}

	return true
}

// Update log format to be simpler
func setupLogger() {
	// Only show date and time, no microseconds or timezone prefix
	log.SetFlags(log.Ldate | log.Ltime)
	// Remove previous SetPrefix call
}

// Update server configuration in main()
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

	// Set global ntfy topic at startup
	ntfyTopic = os.Getenv("NTFY_TOPIC")
	if ntfyTopic == "" {
		log.Fatal("NTFY_TOPIC environment variable is required")
	}

	config, err := loadEnvConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup logger
	setupLogger()

	// Send startup notification
	if err := sendStartupNotification(config); err != nil {
		log.Printf("Failed to send startup notification: %v", err)
	}

	// Create base context with cancellation
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
		if err := startMonitoring(ctx, config); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Monitoring failed: %v", err)
			cancel() // Cancel context if monitoring fails with non-cancellation error
		}
	}()

	// Add static file serving
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Update existing route handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/favicon.ico" {
			http.ServeFile(w, r, "static/favicon.ico")
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		templates.ExecuteTemplate(w, "status.html", nil)
	})

	http.HandleFunc("/status", handleStatus)
	http.HandleFunc("/events", handleEvents)

	// Create HTTP server with adjusted timeout settings for SSE
	srv := &http.Server{
		Addr:              ":8080",
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // Disable write timeout for SSE
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		// Add handler explicitly
		Handler: nil, // Will use default ServeMux
	}

	// Start HTTP server in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("Starting server on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
			cancel() // Cancel context if server fails
		}
	}()

	// Add connection cleanup routine
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			now := time.Now()
			activeConnections.Range(func(key, value interface{}) bool {
				if lastActive, ok := value.(time.Time); ok {
					if now.Sub(lastActive) > 10*time.Minute {
						activeConnections.Delete(key)
					}
				}
				return true
			})
		}
	}()

	// Update daily report ticker to use config timezone
	reportTicker := time.NewTicker(time.Minute)
	defer reportTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reportTicker.C:
				now := time.Now()
				currentTime := now.Format("15:04")
				if currentTime == DAILY_REPORT_TIME {
					sendDailyReport()
				}
			}
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
