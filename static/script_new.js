// Core app class to manage the application state
class FETracker {
    constructor() {
        // Create instances
        this.preferences = new PreferencesManager();
        this.notifications = new NotificationManager();
        this.metrics = new MetricsManager();

        // Other initialization
        this.eventSource = null;
        this.reconnectAttempts = 0;
        this.lastSku = null;
        this.lastPurchaseUrl = '';
    }

    init() {
        this.preferences.loadAll();
        this.setupEventListeners();
        this.connectSSE();
        this.initializeTooltips();
    }

    setupEventListeners() {
        document.getElementById('themeToggle').addEventListener('change', () => this.preferences.toggleTheme());
        document.getElementById('sleepToggle').addEventListener('change', () => this.preferences.toggleSleep());
        document.getElementById('ttsToggle').addEventListener('change', () => this.preferences.toggleTTS());
        document.getElementById('screensaverToggle').addEventListener('change', () => this.preferences.toggleScreensaver());
        document.getElementById('autoOpenWrapper').addEventListener('click', this.handleAutoOpenClick.bind(this));
    }

    handleAutoOpenClick(e) {
        const toggle = document.getElementById('autoToggle');
        if (e.target === toggle || e.target.matches('label[for="autoToggle"]')) return;
        toggle.checked = !toggle.checked;
        toggle.dispatchEvent(new Event('change'));
    }

    connectSSE() {
        this.eventSource = new EventSource('/events');
        this.setupSSEHandlers();
    }

    setupSSEHandlers() {
        this.eventSource.onmessage = (event) => {
            const data = JSON.parse(event.data);
            this.handleServerUpdate(data);
        };

        this.eventSource.onerror = () => {
            this.handleSSEError();
        };
    }

    handleServerUpdate(data) {
        this.metrics.updateMetrics(data);
        this.updatePurchaseButton(data.metrics);
        this.checkSkuChange(data.metrics.current_sku);
    }

    handleSSEError() {
        this.metrics.setStatus('disconnected');
        this.eventSource.close();
        this.attemptReconnection();
    }

    attemptReconnection() {
        const maxAttempts = 5;
        const baseDelay = 1000;

        if (this.reconnectAttempts < maxAttempts) {
            const delay = baseDelay * Math.pow(2, this.reconnectAttempts);
            this.reconnectAttempts++;
            setTimeout(() => this.connectSSE(), delay);
        } else {
            this.notifications.show('Connection Lost', 'Please refresh the page to reconnect');
        }
    }

    initializeTooltips() {
        const tooltipManager = new TooltipManager();
        tooltipManager.init();
    }

    updatePurchaseButton(metrics) {
        const purchaseButton = document.getElementById('purchaseButton');
        if (metrics.purchase_url) {
            purchaseButton.href = metrics.purchase_url;
            purchaseButton.classList.add('available');
            purchaseButton.textContent = 'Purchase Now!';
            purchaseButton.onclick = null;

            // Check for new purchase URL
            if (metrics.purchase_url !== this.lastPurchaseUrl) {
                this.lastPurchaseUrl = metrics.purchase_url;
                this.notifications.show('Product Available!', 'Click to open purchase page', metrics.purchase_url);
                
                const autoToggle = document.getElementById('autoToggle');
                if (autoToggle.checked) {
                    window.open(metrics.purchase_url, '_blank');
                }
            }
        } else {
            purchaseButton.href = 'javascript:void(0)';
            purchaseButton.classList.remove('available');
            purchaseButton.textContent = 'Not Available';
            purchaseButton.onclick = (e) => e.preventDefault();
            this.lastPurchaseUrl = '';
        }
    }

    checkSkuChange(currentSku) {
        if (currentSku && currentSku !== this.lastSku) {
            if (this.lastSku !== null) {
                this.notifications.show('SKU Changed', `New SKU detected: ${currentSku}`);
            }
            this.lastSku = currentSku;
        }
    }
}

class PreferencesManager {
    constructor() {
        this.wakeLock = null;
        this.ttsEnabled = false;
        // Store reference to parent app if available
        this.app = window.app;
        this.speechQueue = [];
        this.isSpeaking = false;
    }

    loadAll() {
        this.loadTheme();
        this.loadSleepPreference();
        this.loadTTSPreference();
        this.loadAutoOpenPreference();
    }

    loadTheme() {
        const theme = localStorage.getItem('theme') || 'dark';
        document.documentElement.setAttribute('data-theme', theme);
        document.getElementById('themeToggle').checked = theme === 'light';
    }

    loadSleepPreference() {
        const preventSleep = localStorage.getItem('preventSleep') === 'true';
        document.getElementById('sleepToggle').checked = preventSleep;
        if (preventSleep && 'wakeLock' in navigator) {
            this.requestWakeLock();
        }
    }

    loadTTSPreference() {
        this.ttsEnabled = localStorage.getItem('ttsEnabled') === 'true';
        document.getElementById('ttsToggle').checked = this.ttsEnabled;
    }

    loadAutoOpenPreference() {
        const autoOpen = localStorage.getItem('autoOpen') !== 'false';
        document.getElementById('autoToggle').checked = autoOpen;
    }

    async toggleTheme() {
        const theme = document.getElementById('themeToggle').checked ? 'light' : 'dark';
        document.documentElement.setAttribute('data-theme', theme);
        localStorage.setItem('theme', theme);
    }

    async toggleSleep() {
        const preventSleep = document.getElementById('sleepToggle').checked;
        if (preventSleep) {
            await this.requestWakeLock();
        } else {
            await this.releaseWakeLock();
        }
        localStorage.setItem('preventSleep', preventSleep);
    }

    async requestWakeLock() {
        if (!('wakeLock' in navigator)) return;
        try {
            this.wakeLock = await navigator.wakeLock.request('screen');
            console.log('Wake Lock is active');
        } catch (err) {
            console.error(`Wake Lock error: ${err.name}, ${err.message}`);
            document.getElementById('sleepToggle').checked = false;
        }
    }

    async releaseWakeLock() {
        if (this.wakeLock) {
            await this.wakeLock.release();
            this.wakeLock = null;
            console.log('Wake Lock released');
        }
    }

    toggleTTS() {
        this.ttsEnabled = document.getElementById('ttsToggle').checked;
        localStorage.setItem('ttsEnabled', this.ttsEnabled);
        this.speak('TTS enabled');
    }

    async speak(text) {
        if (this.ttsEnabled && 'speechSynthesis' in window) {
            // Cancel any ongoing speech
            speechSynthesis.cancel();
    
            const utterance = new SpeechSynthesisUtterance(text);
            utterance.lang = 'en-US';
            utterance.rate = 1;
            // Set volume to maximum to help with background playback
            utterance.volume = 1;
    
            // Create an audio context to keep audio working in background
            const audioContext = new (window.AudioContext || window.webkitAudioContext)();
            
            // Resume audio context if it's suspended
            if (audioContext.state === 'suspended') {
                audioContext.resume();
            }
    
            // Ensure speech synthesis doesn't get interrupted
            utterance.onend = () => {
                // Keep audio context active
                audioContext.resume();
            };
    
            speechSynthesis.speak(utterance);
        }
    }

    toggleScreensaver() {
        if (window.innerWidth < 768) {
            document.getElementById('screensaverToggle').checked = false;
            localStorage.setItem('screensaverEnabled', false);
            if (this.app) {
                this.app.screensaver.stop();
            }
            return;
        }

        const screensaverEnabled = document.getElementById('screensaverToggle').checked;
        localStorage.setItem('screensaverEnabled', screensaverEnabled);
        
        if (screensaverEnabled && this.app) {
            this.app.screensaver.start();
        } else if (this.app) {
            this.app.screensaver.stop();
        }
    }

    toggleAutoOpen() {
        const autoOpen = document.getElementById('autoToggle').checked;
        localStorage.setItem('autoOpen', autoOpen);
    }
}

class NotificationManager {
    constructor() {
        this.checkPermission();
    }

    checkPermission() {
        if (Notification.permission === "default") {
            document.getElementById('notificationPermission').style.display = 'block';
        }
    }

    requestPermission() {
        if (!("Notification" in window)) {
            alert("This browser does not support desktop notification");
            return;
        }

        Notification.requestPermission().then(permission => {
            if (permission === "granted") {
                document.getElementById('notificationPermission').style.display = 'none';
                this.show("Notifications Enabled", "You will now receive notifications when stock becomes available.");
            } else if (permission === "denied") {
                alert("You have blocked notifications. Please enable them in your browser settings to receive stock alerts.");
            }
        });
    }

    show(title, message, url = null) {
        if (Notification.permission === "granted") {
            const notification = new Notification(title, {
                body: message,
                icon: '/static/favicon.ico',
            });

            if (url) {
                notification.onclick = () => {
                    window.open(url, '_blank');
                    notification.close();
                };
            }

            if (window.app?.preferences?.ttsEnabled) {
                window.app.preferences.speak(title);
            }
            
        } else if (Notification.permission === "default") {
            document.getElementById('notificationPermission').style.display = 'block';
        }
    }
}

class MetricsManager {
    constructor() {
        this.errorBuffer = [];
        this.lastErrorCount = 0;
        this.ONE_MINUTE = 60 * 1000; // 1 minute in milliseconds
        this.lastErrorNotification = 0;
        this.NOTIFICATION_COOLDOWN = 60000; // 60 seconds between notifications
    }

    updateMetrics(data) {
        // Update status
        this.setStatus(data.status);

        // Update basic metrics
        this.updateMetric('uptime', data.uptime);
        this.updateMetric('currentSku', data.metrics.current_sku || 'N/A');
        this.updateMetric('errorCount', data.metrics.error_count_24h);
        this.updateMetric('apiRequests', data.metrics.api_requests_24h);
        this.updateMetric('ntfySent', data.metrics.ntfy_messages_sent);
        this.updateMetric('startTime', new Date(data.metrics.start_time).toLocaleString());

        // Check error rate
        this.checkErrorRate(data.metrics.error_count_24h);
    }

    updateMetric(elementId, value) {
        const element = document.getElementById(elementId);
        if (element) {
            element.textContent = value;
        }
    }

    checkErrorRate(currentErrorCount) {
        // Calculate new errors
        const newErrors = currentErrorCount - this.lastErrorCount;
        if (newErrors > 0) {
            // Add new error timestamp
            const now = Date.now();
            for (let i = 0; i < newErrors; i++) {
                this.errorBuffer.push(now);
            }
        }

        // Remove errors older than 1 minute
        const cutoff = Date.now() - this.ONE_MINUTE;
        while (this.errorBuffer.length > 0 && this.errorBuffer[0] < cutoff) {
            this.errorBuffer.shift();
        }

        // Update the error rate display
        const errorRate = this.errorBuffer.length;
        this.updateMetric('errorRate', errorRate);

        // Check if error rate is high and enough time has passed since last notification
        const now = Date.now();
        if (errorRate >= 5 && window.app && window.app.notifications && 
            (now - this.lastErrorNotification) >= this.NOTIFICATION_COOLDOWN) {
            window.app.notifications.show(
                'High Error Rate', 
                `${errorRate} errors detected in the last minute!`
            );
            this.lastErrorNotification = now;
        }

        // Update last error count
        this.lastErrorCount = currentErrorCount;
    }

    setStatus(status) {
        const statusElement = document.getElementById('status');
        statusElement.textContent = status;
        statusElement.className = status === 'running' ? 'status-ok' : 'status-error';
    }
}

class TooltipManager {
    constructor() {
        this.hideTimeout = null;
    }

    init() {
        const isMobile = window.innerWidth <= 767;
        const tooltips = document.querySelectorAll('.toggle-tooltip');
        
        if (isMobile) {
            this.setupMobileTooltips(tooltips);
        } else {
            this.setupDesktopTooltips(tooltips);
        }
    }

    setupMobileTooltips(tooltips) {
        tooltips.forEach(tooltip => {
            tooltip.addEventListener('touchstart', (e) => {
                // Clear any existing timeout
                clearTimeout(this.hideTimeout);
                
                // Hide all other tooltips
                tooltips.forEach(t => t.classList.remove('show-tooltip'));
                
                // Show current tooltip
                tooltip.classList.add('show-tooltip');
                
                // Auto-hide after 2 seconds
                this.hideTimeout = setTimeout(() => {
                    tooltip.classList.remove('show-tooltip');
                }, 2000);
            });
        });

        // Hide tooltips when touching outside
        document.addEventListener('touchstart', (e) => {
            if (!e.target.closest('.toggle-tooltip')) {
                tooltips.forEach(tooltip => tooltip.classList.remove('show-tooltip'));
                clearTimeout(this.hideTimeout);
            }
        });
    }

    setupDesktopTooltips(tooltips) {
        tooltips.forEach(tooltip => {
            // Show on hover
            tooltip.addEventListener('mouseenter', () => {
                tooltip.classList.add('show-tooltip');
            });
            
            // Hide when mouse leaves
            tooltip.addEventListener('mouseleave', () => {
                tooltip.classList.remove('show-tooltip');
            });
        });
    }
}

// Initialize the application when the DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.app = new FETracker();
    window.app.init();
});
