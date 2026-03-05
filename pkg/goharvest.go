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
	Headless      bool   // Run in headless mode (default: true)
	BrowserPath   string // Path to Chromium/Chrome binary (default: "chromium")
	DebuggingPort string // Remote debugging port (default: "9222")
	UserDataDir   string // User data directory for profile persistence

	// Window and display settings
	WindowSize string // Window size in format "width,height" (default: "1920,1080")
	Language   string // Browser language (default: "en-US,en;q=0.9")
	Timezone   string // Timezone to emulate (default: "America/New_York")
	UseXvfb    bool   // Use Xvfb virtual framebuffer for non-headless mode in Docker (default: false)

	// Anti-detection settings
	UserAgent              string // Custom User-Agent string
	DisableGPU             bool   // Disable GPU acceleration (default: true for headless)
	EnableWebGL            bool   // Enable WebGL (can be used for fingerprinting)
	HideWebDriver          bool   // Hide webdriver property (default: true)
	DisableAutomationFlags bool   // Disable automation flags (default: true)

	// Security and privacy settings
	DisableSecurity      bool // Disable web security (default: false for production)
	IgnoreCertificate    bool // Ignore certificate errors (default: true for scraping)
	DisableDevShmUsage   bool // Disable /dev/shm usage (default: true for Docker)
	NoSandbox            bool // Disable sandbox (default: true for Docker/root)
	DisableExtensions    bool // Disable extensions (default: true)
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
		DisableGPU:             false, // Keep GPU on to look more like a real browser
		EnableWebGL:            true,  // Enable WebGL to pass fingerprint checks
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

// stealthJS is injected into every page immediately after navigation to defeat
// common headless-browser fingerprinting techniques.
const stealthJS = `
(function() {
	// 1. Remove the webdriver flag that headless Chrome exposes
	Object.defineProperty(navigator, 'webdriver', {
		get: () => undefined,
		configurable: true,
	});

	// 2. Override plugins — real browsers have plugins, headless has none
	Object.defineProperty(navigator, 'plugins', {
		get: () => {
			const arr = [1, 2, 3, 4, 5];
			arr.__proto__ = PluginArray.prototype;
			return arr;
		},
		configurable: true,
	});

	// 3. Override mimeTypes — same reason as plugins
	Object.defineProperty(navigator, 'mimeTypes', {
		get: () => {
			const arr = [1, 2, 3];
			arr.__proto__ = MimeTypeArray.prototype;
			return arr;
		},
		configurable: true,
	});

	// 4. Set languages to look like a real en-US user
	Object.defineProperty(navigator, 'languages', {
		get: () => ['en-US', 'en'],
		configurable: true,
	});

	// 5. Add the window.chrome object that real Chrome always exposes
	if (!window.chrome) {
		window.chrome = {
			app: {
				isInstalled: false,
				InstallState: { DISABLED: 'disabled', INSTALLED: 'installed', NOT_INSTALLED: 'not_installed' },
				RunningState: { CANNOT_RUN: 'cannot_run', READY_TO_RUN: 'ready_to_run', RUNNING: 'running' }
			},
			runtime: {
				OnInstalledReason: { CHROME_UPDATE: 'chrome_update', INSTALL: 'install', SHARED_MODULE_UPDATE: 'shared_module_update', UPDATE: 'update' },
				OnRestartRequiredReason: { APP_UPDATE: 'app_update', OS_UPDATE: 'os_update', PERIODIC: 'periodic' },
				PlatformArch: { ARM: 'arm', ARM64: 'arm64', MIPS: 'mips', MIPS64: 'mips64', X86_32: 'x86-32', X86_64: 'x86-64' },
				PlatformNacl: { ARM: 'arm', PNACL: 'pnacl', X86_32: 'x86-32', X86_64: 'x86-64' },
				PlatformOs: { ANDROID: 'android', CROS: 'cros', LINUX: 'linux', MAC: 'mac', OPENBSD: 'openbsd', WIN: 'win' },
				RequestUpdateCheckStatus: { NO_UPDATE: 'no_update', THROTTLED: 'throttled', UPDATE_AVAILABLE: 'update_available' }
			}
		};
	}

	// 6. Patch permissions API — bots are often caught here
	const originalQuery = window.navigator.permissions.query;
	window.navigator.permissions.query = (parameters) => (
		parameters.name === 'notifications' ?
		Promise.resolve({ state: Notification.permission }) :
		originalQuery(parameters)
	);

	// 7. Fix zero-size window properties that headless exposes
	if (window.outerWidth === 0) {
		Object.defineProperty(window, 'outerWidth', { get: () => window.innerWidth });
	}
	if (window.outerHeight === 0) {
		Object.defineProperty(window, 'outerHeight', { get: () => window.innerHeight });
	}

	// 8. Override connection RTT to look like a real network connection
	if (navigator.connection) {
		Object.defineProperty(navigator.connection, 'rtt', {
			get: () => 100,
			configurable: true,
		});
	}
})();
`

// stealthHeaders mimics the HTTP headers sent by a real Chrome browser.
// Headless Chrome omits several of these by default, triggering bot detection.
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

// buildStealthActions returns the ordered chromedp actions that configure stealth
// mode before page content is scraped. Called for every new page request.
//
// Order: blank page → set headers → enable network → set cookies →
//
//	navigate → inject JS → wait ready → human delay
func buildStealthActions(targetURL string, cookies []map[string]string) []chromedp.Action {
	return []chromedp.Action{
		// Step 1: Start from blank so CDP calls work before navigation
		chromedp.Navigate("about:blank"),

		// Step 2: Set realistic HTTP headers (Sec-Fetch-*, Accept, etc.)
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetExtraHTTPHeaders(stealthHeaders).Do(ctx)
		}),

		// Step 3: Enable network domain so cookies and headers take effect
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.Enable().Do(ctx)
		}),

		// Step 4: Set cookies BEFORE navigation so the server sees them on first request
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
					// Non-fatal — continue with remaining cookies
				}
			}
			return nil
		}),

		// Step 5: Navigate to the actual target URL
		chromedp.Navigate(targetURL),

		// Step 6: Inject stealth JS before the page's own scripts can run fingerprint checks
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(stealthJS, nil).Do(ctx)
		}),

		// Step 7: Wait for the DOM to be fully ready
		chromedp.WaitReady(":root"),

		// Step 8: Brief human-like pause to let dynamic content render
		chromedp.ActionFunc(func(ctx context.Context) error {
			time.Sleep(2 * time.Second)
			return nil
		}),
	}
}

// waitForBrowser polls until the browser's remote debugging endpoint responds
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

// startBrowserIfNotRunning starts Chromium with default options
func startBrowserIfNotRunning() error {
	return startBrowserWithOptions(DefaultBrowserOptions())
}

// startBrowserWithOptions starts Chromium with the given options.
// Reuses an existing process if options match; restarts if they differ.
func startBrowserWithOptions(opts *BrowserOptions) error {
	if opts == nil {
		opts = DefaultBrowserOptions()
	}

	browserMutex.Lock()
	defer browserMutex.Unlock()

	port := opts.DebuggingPort
	if port == "" {
		port = browserPort
	}
	browserPort = port

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

	// Clean up stale processes
	if browserProcess != nil {
		browserProcess.Process.Kill()
		browserProcess = nil
	}
	if xvfbProcess != nil {
		xvfbProcess.Process.Kill()
		xvfbProcess = nil
	}

	// Start Xvfb for non-headless mode inside Docker
	var display string
	if !opts.Headless && opts.UseXvfb {
		log.Println("Starting Xvfb virtual display...")
		xvfbCmd := exec.Command("Xvfb", xvfbDisplay, "-screen", "0", "1920x1080x24", "-ac", "-nolisten", "tcp", "-dpi", "96")
		if err := xvfbCmd.Start(); err != nil {
			return fmt.Errorf("failed to start Xvfb: %w (install with: apt-get install xvfb)", err)
		}
		xvfbProcess = xvfbCmd
		display = xvfbDisplay
		log.Printf("Xvfb started on display %s", display)
		time.Sleep(500 * time.Millisecond)
	}

	// Build Chromium launch arguments
	args := []string{}

	// Use --headless=old: significantly less detectable than --headless=new
	if opts.Headless {
		args = append(args, "--headless=old")
	} else if display != "" {
		args = append(args, "--display="+display)
	}

	args = append(args, "--remote-debugging-port="+port)
	args = append(args, "--remote-debugging-address=127.0.0.1")

	if opts.UserDataDir != "" {
		args = append(args, "--user-data-dir="+opts.UserDataDir)
	}
	if opts.WindowSize != "" {
		args = append(args, "--window-size="+opts.WindowSize)
		args = append(args, "--start-maximized")
	}
	if opts.UserAgent != "" {
		args = append(args, "--user-agent="+opts.UserAgent)
	}
	if opts.DisableGPU {
		args = append(args, "--disable-gpu")
	}
	if opts.EnableWebGL {
		args = append(args, "--enable-webgl")
	}
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
	if opts.ProxyServer != "" {
		args = append(args, "--proxy-server="+opts.ProxyServer)
	}
	if opts.ProxyBypassList != "" {
		args = append(args, "--proxy-bypass-list="+opts.ProxyBypassList)
	}
	if opts.Language != "" {
		args = append(args, "--lang="+opts.Language)
	}

	args = append(args, opts.ExtraFlags...)

	browserPath := opts.BrowserPath
	if browserPath == "" {
		browserPath = "chromium"
	}

	browserCmd := exec.Command(browserPath, args...)

	env := os.Environ()
	if display != "" {
		env = append(env, "DISPLAY="+display)
	}
	if opts.Timezone != "" {
		env = append(env, "TZ="+opts.Timezone)
	}
	browserCmd.Env = env

	// Suppress browser stdout/stderr to keep logs clean
	browserCmd.Stderr = io.Discard
	browserCmd.Stdout = io.Discard

	browserProcess = browserCmd
	currentBrowserOps = opts

	if err := browserProcess.Start(); err != nil {
		return fmt.Errorf("failed to start Chromium: %w (path: %s, args: %v)", err, browserPath, args)
	}

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

	log.Printf("Chromium browser started successfully on port %s", port)
	return nil
}

// browserOptionsMatch returns true when two BrowserOptions are functionally
// equivalent, used to decide whether the running browser can be reused.
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
	Text ExtractionFunc = func(sel *goquery.Selection) string {
		return strings.TrimSpace(sel.Text())
	}

	FirstText ExtractionFunc = func(sel *goquery.Selection) string {
		return strings.TrimSpace(sel.First().Text())
	}

	HTML ExtractionFunc = func(sel *goquery.Selection) string {
		html, _ := sel.Html()
		return html
	}

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
	ClickAction  PreScrapeActionType = iota // Click on an element
	ScrollAction                            // Scroll to an element
	WaitAction                              // Wait for a duration
)

// PreScrapeAction represents an action to be performed before scraping
type PreScrapeAction struct {
	Type      PreScrapeActionType
	Selector  string
	Duration  time.Duration
	WaitUntil string
}

// ExtractTextOrAttr extracts text content or an attribute value from a selection
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

// parseWithSelectors uses reflection to populate type T from the parsed document.
// Handles struct, slice-of-struct, and map result types.
func (s *Scrapper[T]) parseWithSelectors(doc *goquery.Document, g *geziyor.Geziyor, baseUrl string, currentURL string) T {
	var result T
	val := reflect.ValueOf(&result).Elem()

	isSliceOfStruct := val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Struct

	if val.Kind() == reflect.Map && val.IsNil() {
		val.Set(reflect.MakeMap(val.Type()))
	}

	for _, sel := range s.Selectors {
		if sel.IsArray {
			var dataArray []string
			doc.Find(sel.Query).Each(func(i int, selection *goquery.Selection) {
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

	// Store the source URL in the result if URLFieldName is configured
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

	// LinkHunt: find matching href links and queue them for scraping
	if s.LinkHunt != "" {
		doc.Find(s.LinkHunt).Each(func(i int, selection *goquery.Selection) {
			link, exists := selection.Attr("href")
			if !exists {
				return
			}

			link = strings.TrimSpace(link)
			switch {
			case strings.HasPrefix(link, "#"):
				return
			case strings.HasPrefix(link, "//"):
				link = "https:" + link
			case !strings.HasPrefix(link, "http"):
				if strings.HasPrefix(link, "/") {
					link = baseUrl + link
				} else {
					link = baseUrl + "/" + link
				}
			case !strings.Contains(link, baseUrl):
				return // Off-domain — skip
			}

			fmt.Println("LinkHunt found:", link)

			s.visitedMutex.RLock()
			_, visited := s.visitedURLs[link]
			s.visitedMutex.RUnlock()

			if visited {
				fmt.Println("URL already visited, skipping:", link)
				return
			}

			s.visitedMutex.Lock()
			s.visitedURLs[link] = true
			s.visitedMutex.Unlock()

			req, err := client.NewRequest("GET", link, nil)
			if err != nil {
				log.Println("Failed to create request:", err)
				return
			}
			req.Rendered = true

			capturedLink := link
			stealthActions := buildStealthActions(capturedLink, s.Cookies)

			extractAction := chromedp.ActionFunc(func(ctx context.Context) error {
				node, err := dom.GetDocument().Do(ctx)
				if err != nil {
					return err
				}
				body, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
				if err != nil {
					return err
				}
				parsedDoc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
				if err != nil {
					return err
				}

				var res T
				if s.ParseFunc != nil {
					res = s.ParseFunc(parsedDoc)
				} else {
					res = s.parseWithSelectors(parsedDoc, g, baseUrl, capturedLink)
				}

				if reflect.ValueOf(res).IsZero() {
					fmt.Println("result is empty, skipping")
					return nil
				}

				s.ExportChan <- res
				return nil
			})

			req.Actions = append(stealthActions, extractAction)
			g.Do(req, g.Opt.ParseFunc)
		})
	}

	return result
}

// ScrapeStream starts scraping and returns a channel that receives results as they arrive.
// The channel is closed once all URLs have been processed.
func (s *Scrapper[T]) ScrapeStream() (<-chan T, error) {
	var err error
	if s.BrowserOptions != nil {
		err = startBrowserWithOptions(s.BrowserOptions)
	} else {
		err = startBrowserIfNotRunning()
	}
	if err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	if s.visitedURLs == nil {
		s.visitedURLs = make(map[string]bool)
	}

	s.ExportChan = make(chan T, 100)

	g := geziyor.NewGeziyor(&geziyor.Options{
		StartRequestsFunc: func(g *geziyor.Geziyor) {
			for _, u := range s.Urls {
				s.visitedMutex.RLock()
				_, visited := s.visitedURLs[u]
				s.visitedMutex.RUnlock()

				if visited {
					fmt.Println("Initial URL already visited, skipping:", u)
					continue
				}

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

				req.Rendered = true

				// Capture loop variables for closures
				currentURL := u
				currentBaseURL := ur.Scheme + "://" + ur.Host

				stealthActions := buildStealthActions(currentURL, s.Cookies)

				// User-defined pre-scrape actions
				var preScrapeActions []chromedp.Action
				for _, action := range s.PreScrapeActions {
					a := action
					switch a.Type {
					case ClickAction:
						preScrapeActions = append(preScrapeActions, chromedp.ActionFunc(func(ctx context.Context) error {
							if err := chromedp.Click(a.Selector).Do(ctx); err != nil {
								log.Printf("Error clicking %s: %v", a.Selector, err)
								return nil
							}
							if a.WaitUntil != "" {
								if err := chromedp.WaitVisible(a.WaitUntil).Do(ctx); err != nil {
									log.Printf("Error waiting for %s: %v", a.WaitUntil, err)
								}
							} else {
								time.Sleep(1 * time.Second)
							}
							return nil
						}))
					case ScrollAction:
						preScrapeActions = append(preScrapeActions, chromedp.ActionFunc(func(ctx context.Context) error {
							if err := chromedp.ScrollIntoView(a.Selector).Do(ctx); err != nil {
								log.Printf("Error scrolling to %s: %v", a.Selector, err)
								return nil
							}
							if a.WaitUntil != "" {
								if err := chromedp.WaitVisible(a.WaitUntil).Do(ctx); err != nil {
									log.Printf("Error waiting for %s: %v", a.WaitUntil, err)
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

				// Final extraction action
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

				// Action order: stealth → pre-scrape → extract
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

// Scrape is a blocking wrapper around ScrapeStream that collects all results
// before returning. Maintains full backward compatibility.
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

// CloseBrowser manually stops the browser. Only needed when KeepBrowserOpen is true.
func (s *Scrapper[T]) CloseBrowser() {
	stopBrowser()
}
