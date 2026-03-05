package pkg

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/geziyor/geziyor"
	"github.com/geziyor/geziyor/client"
)

// Global variables to manage Chromium browser process
var (
	browserProcess    *exec.Cmd
	xvfbProcess       *exec.Cmd
	browserMutex      sync.Mutex
	browserPort       = "9222"
	currentBrowserOps *BrowserOptions // Track current browser options
	xvfbDisplay       = ":99"         // Xvfb display number
)

// BrowserOptions holds configuration for the Chromium browser
type BrowserOptions struct {
	// Basic browser settings
	Headless       bool   // Run in headless mode (default: true)
	BrowserPath    string // Path to Chromium/Chrome binary (default: "chromium")
	DebuggingPort  string // Remote debugging port (default: "9222")
	UserDataDir    string // User data directory for profile persistence

	// Window and display settings
	WindowSize     string // Window size in format "width,height" (default: "1920,1080")
	Language       string // Browser language (default: "en-US,en;q=0.9")
	Timezone       string // Timezone to emulate (default: "America/New_York")
	UseXvfb        bool   // Use Xvfb virtual framebuffer for non-headless mode in Docker (default: false)

	// Anti-detection settings
	UserAgent              string // Custom User-Agent string
	DisableGPU             bool   // Disable GPU acceleration (default: true for headless)
	EnableWebGL            bool   // Enable WebGL (can be used for fingerprinting)
	HideWebDriver          bool   // Hide webdriver property (default: true)
	DisableAutomationFlags bool   // Disable automation flags (default: true)

	// Security and privacy settings
	DisableSecurity     bool // Disable web security (default: false for production)
	IgnoreCertificate   bool // Ignore certificate errors (default: true for scraping)
	DisableDevShmUsage  bool // Disable /dev/shm usage (default: true for Docker)
	NoSandbox           bool // Disable sandbox (default: true for Docker/root)
	DisableExtensions   bool // Disable extensions (default: true)
	DisableBackgrounding bool // Disable background timer throttling

	// Network settings
	ProxyServer     string // Proxy server URL (e.g., "http://proxy:port")
	ProxyBypassList string // Comma-separated list of hosts to bypass proxy

	// Additional custom flags
	ExtraFlags []string // Additional Chromium flags
}

// DefaultBrowserOptions returns a BrowserOptions with secure defaults
func DefaultBrowserOptions() *BrowserOptions {
	return &BrowserOptions{
		Headless:               true,
		BrowserPath:            "chromium",
		DebuggingPort:          "9222",
		UserDataDir:            "/tmp/chrome-profile",
		WindowSize:             "1920,1080",
		Language:               "en-US,en;q=0.9",
		Timezone:               "America/New_York",
		UseXvfb:                false,
		UserAgent:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		DisableGPU:             true,
		EnableWebGL:            false,
		HideWebDriver:          true,
		DisableAutomationFlags: true,
		DisableSecurity:        false,
		IgnoreCertificate:      true,
		DisableDevShmUsage:     true,
		NoSandbox:              true,
		DisableExtensions:      true,
		DisableBackgrounding:   true,
		ProxyServer:            "",
		ProxyBypassList:        "",
		ExtraFlags:             []string{},
	}
}

// StealthBrowserOptions returns a BrowserOptions optimized for anti-detection
func StealthBrowserOptions() *BrowserOptions {
	return &BrowserOptions{
		Headless:               true,
		BrowserPath:            "chromium",
		DebuggingPort:          "9222",
		UserDataDir:            "/tmp/chrome-profile-stealth",
		WindowSize:             "1920,1080",
		Language:               "en-US,en;q=0.9",
		Timezone:               "America/New_York",
		UseXvfb:                false,
		UserAgent:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		DisableGPU:             false,
		EnableWebGL:            true,
		HideWebDriver:          true,
		DisableAutomationFlags: true,
		DisableSecurity:        false,
		IgnoreCertificate:      true,
		DisableDevShmUsage:     true,
		NoSandbox:              true,
		DisableExtensions:      true,
		DisableBackgrounding:   true,
		ProxyServer:            "",
		ProxyBypassList:        "",
		ExtraFlags: []string{
			"--disable-blink-features=AutomationControlled",
		},
	}
}

// DefaultAntiDetectionOptions returns options balancing detection avoidance and stability
func DefaultAntiDetectionOptions() *BrowserOptions {
	opts := DefaultBrowserOptions()
	opts.HideWebDriver = true
	opts.DisableAutomationFlags = true
	opts.ExtraFlags = []string{
		"--disable-blink-features=AutomationControlled",
	}
	return opts
}

// DockerBrowserOptions returns options optimized for running in Docker containers
func DockerBrowserOptions() *BrowserOptions {
	return &BrowserOptions{
		Headless:               false,
		BrowserPath:            "chromium",
		DebuggingPort:          "9222",
		UserDataDir:            "/tmp/chrome-profile-docker",
		WindowSize:             "1920,1080",
		Language:               "en-US,en;q=0.9",
		Timezone:               "UTC",
		UseXvfb:                true,
		UserAgent:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		DisableGPU:             true,
		EnableWebGL:            false,
		HideWebDriver:          true,
		DisableAutomationFlags: true,
		DisableSecurity:        false,
		IgnoreCertificate:      true,
		DisableDevShmUsage:     true,
		NoSandbox:              true,
		DisableExtensions:      true,
		DisableBackgrounding:   true,
		ProxyServer:            "",
		ProxyBypassList:        "",
		ExtraFlags: []string{
			"--disable-blink-features=AutomationControlled",
			"--disable-gpu",
			"--no-sandbox",
			"--disable-dev-shm-usage",
		},
	}
}

// ExtractionFunc defines a function that takes a *goquery.Selection and returns a string
type ExtractionFunc func(*goquery.Selection) string

// Selector defines how to extract data from HTML
type Selector struct {
	Name        string         // Struct Field Name or Map Key
	Query       string         // CSS selector query
	Attr        string         // Attribute to extract (e.g., "src", "href", or empty for text content)
	IsArray     bool           // Whether to extract multiple values as an array
	ExtractFunc ExtractionFunc // Function to extract data from selection
}

// Scrapper represents a configurable scraper that can handle different types of data
type Scrapper[T any] struct {
	Urls                        []string
	RequestDelay                time.Duration
	Timeout                     time.Duration
	RobotsTxtDisabled           bool
	LogDisabled                 bool
	ConcurrentRequests          int
	ConcurrentRequestsPerDomain int
	UserAgent                   string
	Cookies                     []map[string]string       // Custom cookies to be sent for requests
	Selectors                   []Selector                // List of selectors to extract data
	ParseFunc                   func(*goquery.Document) T // Optional: Custom parser. If nil, Selectors are used.
	ExportChan                  chan T                    // Channel to export results
	LinkHunt                    string                    // CSS selector for links to auto-click and scrape
	EachEvent                   func(T)
	PreScrapeActions            []PreScrapeAction // Actions to perform before scraping (e.g., clicks, scrolls)
	URLFieldName                string            // Field name to store the URL in the result (default: "URL")
	visitedURLs                 map[string]bool   // Track URLs that have been visited to prevent duplicates
	visitedMutex                sync.RWMutex      // Mutex to protect visitedURLs map
	KeepBrowserOpen             bool              // Whether to keep the browser open after scraping (default: false)
	BrowserOptions              *BrowserOptions   // Custom browser configuration options
}

// Options for configuring the scraper
type Options struct {
	CategoryURL string
	MaxScroll   int
}

// stealthJS contains JavaScript to evade common bot detection mechanisms
const stealthJS = `
(function() {
	// Override webdriver property - most basic check
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined,
		configurable: true,
	});

	// Override plugins to appear as a real browser (empty plugins = headless)
	Object.defineProperty(navigator, 'plugins', {
		get: () => {
			const arr = [1, 2, 3, 4, 5];
			arr.__proto__ = PluginArray.prototype;
			return arr;
		},
		configurable: true,
	});

	// Override mimeTypes
	Object.defineProperty(navigator, 'mimeTypes', {
		get: () => {
			const arr = [1, 2, 3];
			arr.__proto__ = MimeTypeArray.prototype;
			return arr;
		},
		configurable: true,
	});

	// Override languages to look like a real user
	Object.defineProperty(navigator, 'languages', {
		get: () => ['en-US', 'en'],
		configurable: true,
	});

	// Add chrome property that real Chrome has
	if (!window.chrome) {
		window.chrome = {
			app: {
				isInstalled: false,
				InstallState: {
					DISABLED: 'disabled',
					INSTALLED: 'installed',
					NOT_INSTALLED: 'not_installed'
				},
				RunningState: {
					CANNOT_RUN: 'cannot_run',
					READY_TO_RUN: 'ready_to_run',
					RUNNING: 'running'
				}
			},
			runtime: {
				OnInstalledReason: {
					CHROME_UPDATE: 'chrome_update',
					INSTALL: 'install',
					SHARED_MODULE_UPDATE: 'shared_module_update',
					UPDATE: 'update'
				},
				OnRestartRequiredReason: {
					APP_UPDATE: 'app_update',
					OS_UPDATE: 'os_update',
					PERIODIC: 'periodic'
				},
				PlatformArch: {
					ARM: 'arm',
					ARM64: 'arm64',
					MIPS: 'mips',
					MIPS64: 'mips64',
					X86_32: 'x86-32',
					X86_64: 'x86-64'
				},
				PlatformNacl: {
					ARM: 'arm',
					PNACL: 'pnacl',
					X86_32: 'x86-32',
					X86_64: 'x86-64'
				},
				PlatformOs: {
					ANDROID: 'android',
					CROS: 'cros',
					LINUX: 'linux',
					MAC: 'mac',
					OPENBSD: 'openbsd',
					WIN: 'win'
				},
				RequestUpdateCheckStatus: {
					NO_UPDATE: 'no_update',
					THROTTLED: 'throttled',
					UPDATE_AVAILABLE: 'update_available'
				}
			}
		};
	}

	// Override permissions query to prevent notification-based detection
	const originalQuery = window.navigator.permissions.query;
	window.navigator.permissions.query = (parameters) => (
		parameters.name === 'notifications' ?
		Promise.resolve({ state: Notification.permission }) :
		originalQuery(parameters)
	);

	// Fix hairline feature detection
	if (window.outerWidth === 0) {
		Object.defineProperty(window, 'outerWidth', { get: () => window.innerWidth });
	}
	if (window.outerHeight === 0) {
		Object.defineProperty(window, 'outerHeight', { get: () => window.innerHeight });
	}

	// Override connection rtt to not look like a bot
	if (navigator.connection) {
		Object.defineProperty(navigator.connection, 'rtt', {
			get: () => 100,
			configurable: true,
		});
	}
})();
`

// stealthHeaders returns the HTTP headers that mimic a real browser
var stealthHeaders = network.Headers{
	"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
	"Accept-Language":           "en-US,en;q=0.9",
	"Accept-Encoding":           "gzip, deflate, br",
	"Cache-Control":             "max-age=0",
	"Sec-Ch-Ua":                 `"Chromium";v="122", "Not(A:Brand";v="24", "Google Chrome";v="122"`,
	"Sec-Ch-Ua-Mobile":          "?0",
	"Sec-Ch-Ua-Platform":        `"Windows"`,
	"Sec-Fetch-Dest":            "document",
	"Sec-Fetch-Mode":            "navigate",
	"Sec-Fetch-Site":            "none",
	"Sec-Fetch-User":            "?1",
	"Upgrade-Insecure-Requests": "1",
}

// buildStealthActions returns the chromedp actions to inject stealth scripts and headers
func buildStealthActions(targetURL string, cookies []map[string]string) []chromedp.Action {
	return []chromedp.Action{
		// 1. Navigate to blank page first to allow CDP calls
		chromedp.Navigate("about:blank"),

		// 2. Set extra HTTP headers to mimic real browser
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetExtraHTTPHeaders(stealthHeaders).Do(ctx)
		}),

		// 3. Enable network domain
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.Enable().Do(ctx)
		}),

		// 4. Set cookies BEFORE navigation
		chromedp.ActionFunc(func(ctx context.Context) error {
			parsedURL, err := url.Parse(targetURL)
			if err != nil {
				log.Println("Failed to parse URL for cookie domain:", err)
				return err
			}
			domain := parsedURL.Host
			if !strings.HasPrefix(domain, ".") {
				domain = "." + domain
			}

			for _, cookie := range cookies {
				err := network.SetCookie(cookie["name"], cookie["value"]).
					WithDomain(domain).
					WithPath("/").
					WithHTTPOnly(false).
					WithSecure(false).
					Do(ctx)
				if err != nil {
					log.Println("Failed to set cookie:", cookie["name"], err)
				}
			}
			return nil
		}),

		// 5. Navigate to the actual target URL
		chromedp.Navigate(targetURL),

		// 6. Inject stealth JS right after navigation begins
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(stealthJS, nil).Do(ctx)
		}),

		// 7. Wait for the page root to be ready
		chromedp.WaitReady(":root"),

		// 8. Small human-like delay to let JS finish rendering
		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(2 * time.Second)
			return nil
		}),
	}
}

// waitForBrowser waits for the browser to start and be ready to accept connections
func waitForBrowser(port string) error {
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for browser to start on port %s", port)
		case <-ticker.C:
			resp, err := http.Get("http://127.0.0.1:" + port + "/json/version")
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// startBrowserIfNotRunning starts the Chromium browser if it's not already running
func startBrowserIfNotRunning() error {
	return startBrowserWithOptions(DefaultBrowserOptions())
}

// startBrowserWithOptions starts the Chromium browser with custom options
func startBrowserWithOptions(opts *BrowserOptions) error {
	if opts == nil {
		opts = DefaultBrowserOptions()
	}

	browserMutex.Lock()
	defer browserMutex.Unlock()

	// Use the port from options or default
	port := opts.DebuggingPort
	if port == "" {
		port = browserPort
	}
	browserPort = port

	// Check if the browser is already running with the same options
	if isBrowserRunning(port) {
		if currentBrowserOps != nil && !browserOptionsMatch(currentBrowserOps, opts) {
			log.Println("Browser options changed, restarting browser...")
			if browserProcess != nil {
				browserProcess.Process.Kill()
				browserProcess = nil
			}
			if xvfbProcess != nil {
				xvfbProcess.Process.Kill()
				xvfbProcess = nil
			}
		} else {
			log.Println("Chromium browser is already running on port", port)
			return nil
		}
	}

	// Clean up old processes
	if browserProcess != nil {
		browserProcess.Process.Kill()
		browserProcess = nil
	}
	if xvfbProcess != nil {
		xvfbProcess.Process.Kill()
		xvfbProcess = nil
	}

	// Start Xvfb if needed (for non-headless mode in Docker)
	var display string
	if !opts.Headless && opts.UseXvfb {
		log.Println("Starting Xvfb virtual display...")
		xvfbCmd := exec.Command("Xvfb", xvfbDisplay, "-screen", "0", "1920x1080x24", "-ac", "-nolisten", "tcp", "-dpi", "96")
		if err := xvfbCmd.Start(); err != nil {
			return fmt.Errorf("failed to start Xvfb: %w (make sure xvfb is installed: apt-get install xvfb)", err)
		}
		xvfbProcess = xvfbCmd
		display = xvfbDisplay
		log.Printf("Xvfb started on display %s", display)
		time.Sleep(500 * time.Millisecond)
	}

	// Build command line arguments
	args := []string{}

	// Headless mode
	if opts.Headless {
		args = append(args, "--headless=old") // Use old headless - less detectable
	} else if display != "" {
		args = append(args, "--display="+display)
	}

	// Remote debugging
	args = append(args, "--remote-debugging-port="+port)
	args = append(args, "--remote-debugging-address=127.0.0.1")

	// User data directory
	if opts.UserDataDir != "" {
		args = append(args, "--user-data-dir="+opts.UserDataDir)
	}

	// Window size
	if opts.WindowSize != "" {
		args = append(args, "--window-size="+opts.WindowSize)
	}

	// User agent
	if opts.UserAgent != "" {
		args = append(args, "--user-agent="+opts.UserAgent)
	}

	// GPU settings
	if opts.DisableGPU {
		args = append(args, "--disable-gpu")
	}
	if opts.EnableWebGL {
		args = append(args, "--enable-webgl")
	}

	// Security settings
	if opts.NoSandbox {
		args = append(args, "--no-sandbox")
	}
	if opts.DisableDevShmUsage {
		args = append(args, "--disable-dev-shm-usage")
	}
	if opts.DisableSecurity {
		args = append(args, "--disable-web-security")
		args = append(args, "--allow-running-insecure-content")
	}
	if opts.IgnoreCertificate {
		args = append(args, "--ignore-certificate-errors")
		args = append(args, "--ignore-urlfetcher-cert-requests")
	}
	if opts.DisableExtensions {
		args = append(args, "--disable-extensions")
	}

	// Anti-detection settings
	if opts.HideWebDriver {
		args = append(args, "--disable-blink-features=AutomationControlled")
	}
	if opts.DisableAutomationFlags {
		args = append(args, "--disable-infobars")
		args = append(args, "--disable-default-apps")
		args = append(args, "--no-first-run")
		args = append(args, "--disable-features=IsolateOrigins,site-per-process")
		args = append(args, "--disable-features=VizDisplayCompositor")
	}
	if opts.DisableBackgrounding {
		args = append(args, "--disable-backgrounding-occluded-windows")
		args = append(args, "--disable-renderer-backgrounding")
		args = append(args, "--disable-background-timer-throttling")
	}

	// Network settings
	if opts.ProxyServer != "" {
		args = append(args, "--proxy-server="+opts.ProxyServer)
	}
	if opts.ProxyBypassList != "" {
		args = append(args, "--proxy-bypass-list="+opts.ProxyBypassList)
	}

	// Language
	if opts.Language != "" {
		args = append(args, "--lang="+opts.Language)
	}

	// Add extra flags
	args = append(args, opts.ExtraFlags...)

	// Determine browser path
	browserPath := opts.BrowserPath
	if browserPath == "" {
		browserPath = "chromium"
	}

	// Create browser command
	browserCmd := exec.Command(browserPath, args...)

	// Set environment variables
	if display != "" {
		browserCmd.Env = append(os.Environ(), "DISPLAY="+display)
	}
	if opts.Timezone != "" {
		browserCmd.Env = append(browserCmd.Env, "TZ="+opts.Timezone)
	}

	browserProcess = browserCmd

	// Suppress stderr/stdout to reduce log noise (optional - comment out to see full logs)
	browserProcess.Stderr = io.Discard
	browserProcess.Stdout = io.Discard

	err := browserProcess.Start()
	if err != nil {
		return fmt.Errorf("failed to start Chromium: %w (path: %s, args: %v)", err, browserPath, args)
	}

	// Store current options
	currentBrowserOps = opts

	log.Println("Starting Chromium browser...")
	if err := waitForBrowser(port); err != nil {
		browserProcess.Process.Kill()
		browserProcess = nil
		if xvfbProcess != nil {
			xvfbProcess.Process.Kill()
			xvfbProcess = nil
		}
		return fmt.Errorf("failed to wait for browser: %w", err)
	}

	log.Println("Chromium browser started successfully on port", port)
	return nil
}

// browserOptionsMatch compares two BrowserOptions structs
func browserOptionsMatch(a, b *BrowserOptions) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Headless == b.Headless &&
		a.DebuggingPort == b.DebuggingPort &&
		a.UserDataDir == b.UserDataDir &&
		a.WindowSize == b.WindowSize &&
		a.UserAgent == b.UserAgent &&
		a.DisableGPU == b.DisableGPU &&
		a.EnableWebGL == b.EnableWebGL &&
		a.HideWebDriver == b.HideWebDriver &&
		a.DisableAutomationFlags == b.DisableAutomationFlags &&
		a.ProxyServer == b.ProxyServer &&
		a.UseXvfb == b.UseXvfb
}

func stopBrowser() {
	browserMutex.Lock()
	defer browserMutex.Unlock()

	if browserProcess != nil {
		log.Println("Stopping Chromium browser...")
		browserProcess.Process.Kill()
		browserProcess = nil
		log.Println("Chromium browser stopped")
	}

	// Also stop Xvfb if running
	if xvfbProcess != nil {
		log.Println("Stopping Xvfb...")
		xvfbProcess.Process.Kill()
		xvfbProcess = nil
		log.Println("Xvfb stopped")
	}
}

func isBrowserRunning(port string) bool {
	checkURL := fmt.Sprintf("http://127.0.0.1:%s/json/version", port)
	resp, err := http.Get(checkURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Default extraction functions
var (
	// Text extracts all text content from matching elements
	Text ExtractionFunc = func(sel *goquery.Selection) string {
		return strings.TrimSpace(sel.Text())
	}

	// FirstText extracts text content only from the first matching element
	FirstText ExtractionFunc = func(sel *goquery.Selection) string {
		return strings.TrimSpace(sel.First().Text())
	}

	// HTML extracts the HTML content from matching elements
	HTML ExtractionFunc = func(sel *goquery.Selection) string {
		html, _ := sel.Html()
		return html
	}

	// Attr extracts attribute value from matching elements
	Attr = func(attrName string) ExtractionFunc {
		return func(sel *goquery.Selection) string {
			val, exists := sel.Attr(attrName)
			if exists {
				return val
			}
			return ""
		}
	}
)

// PreScrapeActionType defines the type of pre-scraping action
type PreScrapeActionType int

const (
	// ClickAction performs a click on the selected element
	ClickAction PreScrapeActionType = iota
	// ScrollAction scrolls to the selected element
	ScrollAction
	// WaitAction waits for a specified duration
	WaitAction
)

// PreScrapeAction represents an action to be performed before scraping
type PreScrapeAction struct {
	Type      PreScrapeActionType // Type of action to perform
	Selector  string              // CSS selector for the element to act on
	Duration  time.Duration       // Duration for wait actions
	WaitUntil string              // Selector to wait for after action (optional)
}

// ExtractTextOrAttr extracts text content or attribute value from a selection
func ExtractTextOrAttr(sel *goquery.Selection, attr string) string {
	if attr != "" {
		val, exists := sel.Attr(attr)
		if exists {
			return val
		}
		return ""
	}
	return strings.TrimSpace(sel.Text())
}

// parseWithSelectors uses reflection to populate type T using the struct's Selectors
func (s *Scrapper[T]) parseWithSelectors(doc *goquery.Document, g *geziyor.Geziyor, baseUrl string, currentURL string) T {
	var result T
	val := reflect.ValueOf(&result).Elem()

	// Check if T is a slice of structs (e.g., []MyStruct)
	isSliceOfStruct := val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Struct

	// Handle Map Types
	if val.Kind() == reflect.Map {
		if val.IsNil() {
			val.Set(reflect.MakeMap(val.Type()))
		}
	}

	for _, sel := range s.Selectors {
		if sel.IsArray {
			// Extract array of individual values
			var dataArray []string
			selections := doc.Find(sel.Query)
			selections.Each(func(i int, selection *goquery.Selection) {
				var data string
				if sel.ExtractFunc != nil {
					data = sel.ExtractFunc(selection)
				} else {
					data = ExtractTextOrAttr(selection, sel.Attr)
				}
				if data != "" {
					dataArray = append(dataArray, data)
				}
			})

			if len(dataArray) >= 3 {
				fmt.Printf("Extracted %d items for '%s': %v\n", len(dataArray), sel.Name, dataArray[:3])
			} else {
				fmt.Printf("Extracted %d items for '%s': %v\n", len(dataArray), sel.Name, dataArray)
			}

			if isSliceOfStruct {
				elemType := val.Type().Elem()
				if val.Len() == 0 {
					for _, data := range dataArray {
						newElem := reflect.New(elemType).Elem()
						field := newElem.FieldByName(sel.Name)
						if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
							field.SetString(data)
							val.Set(reflect.Append(val, newElem))
						} else {
							log.Printf("Warning: Field '%s' not string in %s", sel.Name, elemType.Name())
						}
					}
				} else {
					for i, data := range dataArray {
						var targetElem reflect.Value
						if i < val.Len() {
							targetElem = val.Index(i)
						} else {
							newElem := reflect.New(elemType).Elem()
							val.Set(reflect.Append(val, newElem))
							targetElem = val.Index(val.Len() - 1)
						}
						field := targetElem.FieldByName(sel.Name)
						if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
							field.SetString(data)
						}
					}
				}
			} else if val.Kind() == reflect.Struct {
				field := val.FieldByName(sel.Name)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String {
					field.Set(reflect.ValueOf(dataArray))
				}
			} else if val.Kind() == reflect.Map {
				val.SetMapIndex(reflect.ValueOf(sel.Name), reflect.ValueOf(dataArray))
			}
		} else {
			// Single value extraction
			selection := doc.Find(sel.Query)

			var data string
			if sel.ExtractFunc != nil {
				data = sel.ExtractFunc(selection)
			} else {
				data = ExtractTextOrAttr(selection, sel.Attr)
			}

			if isSliceOfStruct && data != "" {
				for i := 0; i < val.Len(); i++ {
					elem := val.Index(i)
					field := elem.FieldByName(sel.Name)
					if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
						field.SetString(data)
					}
				}
				if val.Len() == 0 {
					elemType := val.Type().Elem()
					newElem := reflect.New(elemType).Elem()
					field := newElem.FieldByName(sel.Name)
					if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
						field.SetString(data)
						val.Set(reflect.Append(val, newElem))
					}
				}
			} else if val.Kind() == reflect.Struct {
				field := val.FieldByName(sel.Name)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
					field.SetString(data)
				}
			} else if val.Kind() == reflect.Map {
				val.SetMapIndex(reflect.ValueOf(sel.Name), reflect.ValueOf(data))
			}
		}
	}

	// Add URL to result if URLFieldName is specified and field exists
	if s.URLFieldName != "" {
		if val.Kind() == reflect.Struct {
			field := val.FieldByName(s.URLFieldName)
			if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
				field.SetString(currentURL)
			}
		} else if isSliceOfStruct {
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i)
				field := elem.FieldByName(s.URLFieldName)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
					field.SetString(currentURL)
				}
			}
		}
	}

	// Handle LinkHunt: find links in the page and queue them for scraping
	if s.LinkHunt != "" {
		doc.Find(s.LinkHunt).Each(func(i int, selection *goquery.Selection) {
			link, exists := selection.Attr("href")
			if !exists {
				return
			}

			link = strings.TrimSpace(link)
			if !strings.HasPrefix(link, "http") && !strings.HasPrefix(link, "//") && !strings.HasPrefix(link, "#") {
				if strings.HasPrefix(link, "/") {
					link = baseUrl + link
				} else {
					link = baseUrl + "/" + link
				}
			} else if strings.HasPrefix(link, "//") {
				link = "https:" + link
			} else if strings.HasPrefix(link, "#") {
				return
			} else if strings.Contains(link, baseUrl) {
				fmt.Println("This is the link:", link)
			} else {
				return
			}
			fmt.Println("This is the link:", link)

			// Check if already visited
			s.visitedMutex.RLock()
			_, visited := s.visitedURLs[link]
			s.visitedMutex.RUnlock()

			if visited {
				fmt.Println("URL already visited, skipping:", link)
				return
			}

			// Mark as visited
			s.visitedMutex.Lock()
			s.visitedURLs[link] = true
			s.visitedMutex.Unlock()

			req, err := client.NewRequest("GET", link, nil)
			if err != nil {
				log.Println("Failed to create request:", err)
				return
			}

			req.Rendered = true

			// Build stealth actions for the linked page
			stealthActions := buildStealthActions(link, s.Cookies)

			// Append the extraction action
			extractAction := chromedp.ActionFunc(func(ctx context.Context) error {
				node, err := dom.GetDocument().Do(ctx)
				if err != nil {
					return err
				}
				body, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
				if err != nil {
					return err
				}
				doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
				if err != nil {
					return err
				}

				var result T
				if s.ParseFunc != nil {
					result = s.ParseFunc(doc)
				} else {
					result = s.parseWithSelectors(doc, g, baseUrl, link)
				}

				if reflect.ValueOf(result).IsZero() {
					fmt.Println("result is empty")
					return nil
				}

				s.ExportChan <- result
				return nil
			})

			req.Actions = append(stealthActions, extractAction)

			g.Do(req, g.Opt.ParseFunc)
		})
	}

	return result
}

// ScrapeStream starts scraping and returns a channel that will receive results as they are scraped
func (s *Scrapper[T]) ScrapeStream() (<-chan T, error) {
	// Use custom browser options if provided, otherwise use defaults
	var err error
	if s.BrowserOptions != nil {
		err = startBrowserWithOptions(s.BrowserOptions)
	} else {
		err = startBrowserIfNotRunning()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	// Initialize the visited URLs map
	if s.visitedURLs == nil {
		s.visitedURLs = make(map[string]bool)
	}

	s.ExportChan = make(chan T, 100)

	g := geziyor.NewGeziyor(&geziyor.Options{
		StartRequestsFunc: func(g *geziyor.Geziyor) {
			for _, u := range s.Urls {
				// Check if already visited
				s.visitedMutex.RLock()
				_, visited := s.visitedURLs[u]
				s.visitedMutex.RUnlock()

				if visited {
					fmt.Println("Initial URL already visited, skipping:", u)
					continue
				}

				// Mark as visited
				s.visitedMutex.Lock()
				s.visitedURLs[u] = true
				s.visitedMutex.Unlock()

				req, err := client.NewRequest("GET", u, nil)
				if err != nil {
					log.Println("Failed to create request:", err)
					continue
				}

				ur, err := url.Parse(u)
				if err != nil {
					log.Println("Failed to parse url:", err)
					continue
				}
				baseUrl := ur.Scheme + "://" + ur.Host

				req.Rendered = true

				// Capture loop variable for closure
				currentURL := u
				currentBaseURL := baseUrl

				// Build stealth actions for the initial URL
				stealthActions := buildStealthActions(currentURL, s.Cookies)

				// Build pre-scrape user-defined actions
				var preScrapeActions []chromedp.Action
				for _, action := range s.PreScrapeActions {
					a := action
					switch a.Type {
					case ClickAction:
						preScrapeActions = append(preScrapeActions, chromedp.ActionFunc(func(ctx context.Context) error {
							err := chromedp.Click(a.Selector).Do(ctx)
							if err != nil {
								log.Printf("Error clicking element %s: %v", a.Selector, err)
								return nil
							}
							if a.WaitUntil != "" {
								if err = chromedp.WaitVisible(a.WaitUntil).Do(ctx); err != nil {
									log.Printf("Error waiting for element %s: %v", a.WaitUntil, err)
								}
							} else {
								time.Sleep(1 * time.Second)
							}
							return nil
						}))

					case ScrollAction:
						preScrapeActions = append(preScrapeActions, chromedp.ActionFunc(func(ctx context.Context) error {
							err := chromedp.ScrollIntoView(a.Selector).Do(ctx)
							if err != nil {
								log.Printf("Error scrolling to element %s: %v", a.Selector, err)
								return nil
							}
							if a.WaitUntil != "" {
								if err = chromedp.WaitVisible(a.WaitUntil).Do(ctx); err != nil {
									log.Printf("Error waiting for element %s: %v", a.WaitUntil, err)
								}
							} else {
								time.Sleep(1 * time.Second)
							}
							return nil
						}))

					case WaitAction:
						preScrapeActions = append(preScrapeActions, chromedp.ActionFunc(func(ctx context.Context) error {
							time.Sleep(a.Duration)
							return nil
						}))
					}
				}

				// Build the HTML extraction action
				extractAction := chromedp.ActionFunc(func(ctx context.Context) error {
					node, err := dom.GetDocument().Do(ctx)
					if err != nil {
						return err
					}
					body, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
					if err != nil {
						return err
					}

					doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
					if err != nil {
						return err
					}

					var result T
					if s.ParseFunc != nil {
						result = s.ParseFunc(doc)
					} else {
						result = s.parseWithSelectors(doc, g, currentBaseURL, currentURL)
					}

					s.ExportChan <- result
					if s.EachEvent != nil {
						s.EachEvent(result)
					}
					return nil
				})

				// Combine: stealth actions + pre-scrape actions + extraction
				req.Actions = append(stealthActions, append(preScrapeActions, extractAction)...)

				g.Do(req, g.Opt.ParseFunc)
			}
		},
		BrowserEndpoint:             "ws://127.0.0.1:" + browserPort,
		RequestDelay:                s.RequestDelay,
		Timeout:                     s.Timeout,
		RobotsTxtDisabled:           s.RobotsTxtDisabled,
		LogDisabled:                 s.LogDisabled,
		ConcurrentRequests:          s.ConcurrentRequests,
		ConcurrentRequestsPerDomain: s.ConcurrentRequestsPerDomain,

		ErrorFunc: func(g *geziyor.Geziyor, r *client.Request, err error) {
			log.Printf("Error during request: %v for URL: %s", err, r.URL.String())
		},

		UserAgent: s.UserAgent,
	})

	go func() {
		g.Start()
		close(s.ExportChan)
		if !s.KeepBrowserOpen {
			stopBrowser()
		}
	}()

	return s.ExportChan, nil
}

// Scrape collects all results before returning (backward compatible wrapper around ScrapeStream)
func (s *Scrapper[T]) Scrape() ([]T, error) {
	streamChan, err := s.ScrapeStream()
	if err != nil {
		return nil, err
	}

	var results []T
	for result := range streamChan {
		results = append(results, result)
	}

	return results, nil
}

// CloseBrowser manually closes the browser instance if it's running
func (s *Scrapper[T]) CloseBrowser() {
	stopBrowser()
}
