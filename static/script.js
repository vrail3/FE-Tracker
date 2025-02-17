// Initialize everything when DOM is loaded - place this at the top of the file
document.addEventListener('DOMContentLoaded', () => {
    // Load all preferences before adding event listeners
    const theme = localStorage.getItem('theme') || 'dark';
    const preventSleep = localStorage.getItem('preventSleep') === 'true';
    const ttsEnabled = localStorage.getItem('ttsEnabled') === 'true';
    const screensaverEnabled = localStorage.getItem('screensaverEnabled') === 'true';
    const autoOpen = localStorage.getItem('autoOpen') !== 'false';

    // Set initial states without triggering change events
    document.documentElement.setAttribute('data-theme', theme);
    document.getElementById('themeToggle').checked = theme === 'light';
    document.getElementById('sleepToggle').checked = preventSleep;
    document.getElementById('ttsToggle').checked = ttsEnabled;
    document.getElementById('screensaverToggle').checked = screensaverEnabled;
    document.getElementById('autoToggle').checked = autoOpen;

    // Initialize features based on preferences
    if (preventSleep && 'wakeLock' in navigator) {
        requestWakeLock();
    }
    
    if (screensaverEnabled && window.innerWidth >= 768) {
        startScreensaver();
    }

    // Now add event listeners
    document.getElementById('themeToggle').addEventListener('change', toggleTheme);
    document.getElementById('sleepToggle').addEventListener('change', toggleSleep);
    document.getElementById('ttsToggle').addEventListener('change', toggleTTS);
    document.getElementById('screensaverToggle').addEventListener('change', toggleScreensaver);
    
    // Initialize other features
    if (Notification.permission === "default") {
        document.getElementById('notificationPermission').style.display = 'block';
    }

    // Initialize tooltips
    initializeTooltips();
});

// Theme handling
function loadTheme() {
    const theme = localStorage.getItem('theme') || 'dark';
    document.documentElement.setAttribute('data-theme', theme);
    document.getElementById('themeToggle').checked = theme === 'light';
}

function toggleTheme() {
    const theme = document.getElementById('themeToggle').checked ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('theme', theme);
}

function loadSleepPreference() {
    const preventSleep = localStorage.getItem('preventSleep') === 'true';
    document.getElementById('sleepToggle').checked = preventSleep;
    if (preventSleep && 'wakeLock' in navigator) {
        requestWakeLock();
    }
}

function loadTTSPreference() {
    const ttsEnabled = localStorage.getItem('ttsEnabled') === 'true';
    document.getElementById('ttsToggle').checked = ttsEnabled;
}

function loadScreensaverPreference() {
    const screensaverEnabled = localStorage.getItem('screensaverEnabled') === 'true';
    document.getElementById('screensaverToggle').checked = screensaverEnabled;
    if (screensaverEnabled && window.innerWidth >= 768) {
        startScreensaver();
    }
}



// Simplified update function
function updateMetric(elementId, value) {
    document.getElementById(elementId).textContent = value;
}

// SSE handling with reconnection
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;
const reconnectDelay = 1000;

function connectSSE() {
    const sse = new EventSource('/events');
    
    sse.onmessage = (event) => {
        reconnectAttempts = 0;
        const data = JSON.parse(event.data);
        
        // Update status
        const statusElement = document.getElementById('status');
        statusElement.textContent = data.status;
        statusElement.className = data.status === 'running' ? 'status-ok' : 'status-error';

        // Update metrics
        updateMetric('uptime', data.uptime);
        updateMetric('currentSku', data.metrics.current_sku || 'N/A');
        updateMetric('errorCount', data.metrics.error_count_24h);
        updateMetric('apiRequests', data.metrics.api_requests_24h);
        updateMetric('ntfySent', data.metrics.ntfy_messages_sent);
        updateMetric('startTime', new Date(data.metrics.start_time).toLocaleString());

        // Check error rate when error count updates
        checkErrorRate(data.metrics.error_count_24h);

        // Update purchase button
        const purchaseButton = document.getElementById('purchaseButton');
        if (data.metrics.purchase_url) {
            purchaseButton.href = data.metrics.purchase_url;
            purchaseButton.classList.add('available');
            purchaseButton.textContent = 'Purchase Now!';
            purchaseButton.onclick = null;

            // Check for new purchase URL
            if (data.metrics.purchase_url !== lastPurchaseUrl) {
                lastPurchaseUrl = data.metrics.purchase_url;
                showNotification('Product Available!', 'Click to open purchase page', data.metrics.purchase_url);
                
                if (autoToggle.checked) {
                    window.open(data.metrics.purchase_url, '_blank');
                }
            }
        } else {
            purchaseButton.href = 'javascript:void(0)';
            purchaseButton.classList.remove('available');
            purchaseButton.textContent = 'Not Available';
            purchaseButton.onclick = (e) => e.preventDefault();
            lastPurchaseUrl = '';
        }

        // Check for SKU changes
        if (data.metrics.current_sku && data.metrics.current_sku !== lastSku) {
            if (lastSku !== null) {
                showNotification('SKU Changed', `New SKU detected: ${data.metrics.current_sku}`);
            }
            lastSku = data.metrics.current_sku;
        }
    };

    sse.onerror = () => {
        const statusElement = document.getElementById('status');
        statusElement.textContent = 'disconnected';
        statusElement.className = 'status-error';

        // Close current connection
        sse.close();

        // Attempt to reconnect with exponential backoff
        if (reconnectAttempts < maxReconnectAttempts) {
            const delay = reconnectDelay * Math.pow(2, reconnectAttempts);
            reconnectAttempts++;
            console.log(`Reconnecting in ${delay}ms (attempt ${reconnectAttempts}/${maxReconnectAttempts})`);
            setTimeout(connectSSE, delay);
        } else {
            console.log('Max reconnection attempts reached');
            showNotification('Connection Lost', 'Please refresh the page to reconnect');
        }
    };

    return sse;
}

// Initialize state
let lastSku = null;
let lastPurchaseUrl = '';
let eventSource = connectSSE();

// Notification handling
function requestNotificationPermission() {
    if (!("Notification" in window)) {
        alert("This browser does not support desktop notification");
        return;
    }

    Notification.requestPermission().then(permission => {
        if (permission === "granted") {
            document.getElementById('notificationPermission').style.display = 'none';
            showNotification("Notifications Enabled", "You will now receive notifications when stock becomes available.");
        } else if (permission === "denied") {
            alert("You have blocked notifications. Please enable them in your browser settings to receive stock alerts.");
        }
    });
}

function showNotification(title, message, url = null) {
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

        // Only speak the title in English
        speak(title);
    } else if (Notification.permission === "default") {
        document.getElementById('notificationPermission').style.display = 'block';
    }
}

// Auto-open toggle handling
const autoToggle = document.getElementById('autoToggle');
autoToggle.checked = localStorage.getItem('autoOpen') !== 'false';
autoToggle.addEventListener('change', () => {
    localStorage.setItem('autoOpen', autoToggle.checked);
});

// Add this near your other initialization code
document.getElementById('autoOpenWrapper').addEventListener('click', function(e) {
    const toggle = document.getElementById('autoToggle');
    // Don't interfere with direct clicks on toggle or label
    if (e.target === toggle || e.target.matches('label[for="autoToggle"]')) {
        return;
    }
    // For other elements, toggle the checkbox
    toggle.checked = !toggle.checked;
    toggle.dispatchEvent(new Event('change'));
});

// Add wake lock handling
let wakeLock = null;

async function requestWakeLock() {
    try {
        wakeLock = await navigator.wakeLock.request('screen');
        console.log('Wake Lock is active');
    } catch (err) {
        console.error(`Wake Lock error: ${err.name}, ${err.message}`);
        document.getElementById('sleepToggle').checked = false;
    }
}

async function releaseWakeLock() {
    if (wakeLock) {
        await wakeLock.release();
        wakeLock = null;
        console.log('Wake Lock released');
    }
}

async function toggleSleep() {
    const preventSleep = document.getElementById('sleepToggle').checked;
    if (preventSleep) {
        await requestWakeLock();
    } else {
        await releaseWakeLock();
    }
    localStorage.setItem('preventSleep', preventSleep);
}

// Load saved sleep prevention preference
document.addEventListener('DOMContentLoaded', async () => {
    if ('wakeLock' in navigator) {
        document.getElementById('sleepToggleWrapper').style.display = 'block';
        if (localStorage.getItem('preventSleep') === 'true') {
            document.getElementById('sleepToggle').checked = true;
            await requestWakeLock();
        }
    }
});

// Handle visibility change
document.addEventListener('visibilitychange', async () => {
    if (wakeLock !== null && document.visibilityState === 'visible') {
        await requestWakeLock();
    }
});

// TTS handling
let ttsEnabled = localStorage.getItem('ttsEnabled') === 'true';

function toggleTTS() {
    ttsEnabled = document.getElementById('ttsToggle').checked;
    localStorage.setItem('ttsEnabled', ttsEnabled);
    if (ttsEnabled) {
        speak('TTS enabled');
    }
}

function speak(text) {
    if (ttsEnabled && 'speechSynthesis' in window) {
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

        utterance.onerror = (event) => {
            console.error('TTS Error:', event);
            // Try to recover by resuming audio context
            audioContext.resume();
        };

        speechSynthesis.speak(utterance);
    }
}

// Screensaver handling
let screensaverEnabled = localStorage.getItem('screensaverEnabled') === 'true';
const canvas = document.getElementById('screensaver');
const ctx = canvas.getContext('2d');
let animationId = null;
let dvdImage = new Image();
dvdImage.src = 'https://assets.nvidia.partners/images/png/RTX5080-3QTR-Back-Right.png';

// DVD animation state
let x = 0;
let y = 0;
const speed = 0.5; // Fixed speed
let xSpeed = speed;
let ySpeed = speed;
const scale = 0.1; // Scale down the image
let imageWidth = 0;
let imageHeight = 0;

dvdImage.onload = function() {
    imageWidth = dvdImage.width * scale;
    imageHeight = dvdImage.height * scale;
    resizeCanvas();
    if (screensaverEnabled) {
        startScreensaver();
    }
};

function resizeCanvas() {
    canvas.width = window.innerWidth;
    canvas.height = window.innerHeight;
    // Reset position when resizing only if canvas changes
    if (x + imageWidth > canvas.width || y + imageHeight > canvas.height) {
        x = Math.random() * (canvas.width - imageWidth);
        y = Math.random() * (canvas.height - imageHeight);
    }
    
}

function animate() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    
    // Update position
    x += xSpeed;
    y += ySpeed;

    // Bounce off walls
    if (x + imageWidth > canvas.width || x < 0) {
        xSpeed = -xSpeed;
        x = x < 0 ? 0 : canvas.width - imageWidth;
    }
    if (y + imageHeight > canvas.height || y < 0) {
        ySpeed = -ySpeed;
        y = y < 0 ? 0 : canvas.height - imageHeight;
    }

    // Draw image
    ctx.drawImage(dvdImage, x, y, imageWidth, imageHeight);
    animationId = requestAnimationFrame(animate);
}

function startScreensaver() {
    canvas.classList.add('active');
    if (!animationId) {
        animate();
    }
}

function stopScreensaver() {
    canvas.classList.remove('active');
    if (animationId) {
        cancelAnimationFrame(animationId);
        animationId = null;
    }
    // save position for next time
    localStorage.setItem('screensaverX', x);
    localStorage.setItem('screensaverY', y);
}

function toggleScreensaver() {
    // Don't enable screensaver on small screens
    if (window.innerWidth < 768) {
        document.getElementById('screensaverToggle').checked = false;
        localStorage.setItem('screensaverEnabled', false);
        stopScreensaver();
        return;
    }

    screensaverEnabled = document.getElementById('screensaverToggle').checked;
    localStorage.setItem('screensaverEnabled', screensaverEnabled);
    
    if (screensaverEnabled) {
        startScreensaver();
    } else {
        stopScreensaver();
    }
}

// Handle window resize
window.addEventListener('resize', () => {
    resizeCanvas();
    // Disable screensaver on small screens
    if (window.innerWidth < 768 && screensaverEnabled) {
        document.getElementById('screensaverToggle').checked = false;
        localStorage.setItem('screensaverEnabled', false);
        stopScreensaver();
    }
});

// Add these variables at the top of the script
const errorBuffer = [];
const ONE_MINUTE = 60 * 1000; // 1 minute in milliseconds
let lastErrorCount = 0;

function checkErrorRate(currentErrorCount) {
    // Calculate new errors
    const newErrors = currentErrorCount - lastErrorCount;
    if (newErrors > 0) {
        // Add new error timestamp
        const now = Date.now();
        for (let i = 0; i < newErrors; i++) {
            errorBuffer.push(now);
        }
    }

    // Remove errors older than 1 minute
    const cutoff = Date.now() - ONE_MINUTE;
    while (errorBuffer.length > 0 && errorBuffer[0] < cutoff) {
        errorBuffer.shift();
    }

    // Update the error rate display
    const errorRate = errorBuffer.length;
    updateMetric('errorRate', errorRate);

    // Check if error rate is high
    if (errorRate >= 5) {
        showNotification(
            'High Error Rate', 
            `${errorRate} errors detected in the last minute!`
        );
        speak('Warning: High error rate detected');
    }

    // Update last error count
    lastErrorCount = currentErrorCount;
}

// Event listeners for toggles
document.getElementById('themeToggle').addEventListener('change', toggleTheme);
document.getElementById('sleepToggle').addEventListener('change', toggleSleep);
document.getElementById('ttsToggle').addEventListener('change', toggleTTS);
document.getElementById('screensaverToggle').addEventListener('change', toggleScreensaver);

// Move the tooltip initialization to a separate function
function initializeTooltips() {
    const isMobile = window.innerWidth <= 767;
    const tooltips = document.querySelectorAll('.toggle-tooltip');
    let hideTimeout;

    if (isMobile) {
        tooltips.forEach(tooltip => {
            tooltip.addEventListener('touchstart', (e) => {
                // Clear any existing timeout
                clearTimeout(hideTimeout);
                
                // Remove show-tooltip class from all tooltips
                tooltips.forEach(t => t.classList.remove('show-tooltip'));
                
                // Add show-tooltip class to current tooltip
                tooltip.classList.add('show-tooltip');
                
                // Set timeout to hide tooltip after 2 seconds
                hideTimeout = setTimeout(() => {
                    tooltip.classList.remove('show-tooltip');
                }, 2000);
            });
        });

        // Hide tooltips when clicking/touching anywhere else
        document.addEventListener('touchstart', (e) => {
            if (!e.target.closest('.toggle-tooltip')) {
                tooltips.forEach(tooltip => tooltip.classList.remove('show-tooltip'));
                clearTimeout(hideTimeout);
            }
        });
    } else {
        // Desktop hover behavior
        tooltips.forEach(tooltip => {
            tooltip.addEventListener('mouseenter', () => {
                tooltip.classList.add('show-tooltip');
            });
            tooltip.addEventListener('mouseleave', () => {
                tooltip.classList.remove('show-tooltip');
            });
        });
    }
}