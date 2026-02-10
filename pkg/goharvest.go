package pkg

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	browserProcess *exec.Cmd
	browserMutex   sync.Mutex
	browserPort    = "9222"
)

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
	Cookies                     []map[string]string       // Custom cookies to be set for requests
	Selectors                   []Selector                // List of selectors to extract data
	ParseFunc                   func(*goquery.Document) T // Optional: Custom parser. If nil, Selectors are used.
	ExportChan                  chan T                    // Channel to export results
	LinkHunt                    string                    // this will be a class name for a link so we can do auto click and scrapp pages
	EachEvent                   func(T)
	PreScrapeActions            []PreScrapeAction // Actions to perform before scraping (e.g., clicks, scrolls)
	URLFieldName                string            // Field name to store the URL in the result (default: "URL")
	visitedURLs                 map[string]bool   // Track URLs that have been visited to prevent duplicates
	visitedMutex                sync.RWMutex      // Mutex to protect visitedURLs map
	KeepBrowserOpen             bool              // Whether to keep the browser open after scraping (default: false)
}

// Options for configuring the scraper
type Options struct {
	CategoryURL string
	MaxScroll   int
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
	browserMutex.Lock()
	defer browserMutex.Unlock()

	// Check if the browser is already running
	if isBrowserRunning(browserPort) {
		log.Println("Chromium browser is already running on port", browserPort)
		return nil
	}

	// If there's an old process, clean it up
	if browserProcess != nil {
		browserProcess.Process.Kill()
		browserProcess = nil
	}

	// Create and start a new browser process
	browserProcess = exec.Command("chromium",
		"--headless=new",
		"--remote-debugging-port="+browserPort,
		"--remote-debugging-address=127.0.0.1",
		"--disable-blink-features=AutomationControlled",
		"--disable-gpu",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--user-data-dir=/tmp/chrome-profile",
		"--disable-features=IsolateOrigins,site-per-process",
		"--window-size=1920,1080",
		"--allow-running-insecure-content",
		"--ignore-certificate-errors",
		"--ignore-urlfetcher-cert-requests",
		"--disable-web-security",
		"--disable-features=VizDisplayCompositor",
		`--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36`,
	)

	err := browserProcess.Start()
	if err != nil {
		return fmt.Errorf("failed to start Chromium: %w", err)
	}

	log.Println("Starting Chromium browser...")
	if err := waitForBrowser(browserPort); err != nil {
		browserProcess.Process.Kill()
		browserProcess = nil
		return fmt.Errorf("failed to wait for browser: %w", err)
	}

	log.Println("Chromium browser started successfully on port", browserPort)
	return nil
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
}

func isBrowserRunning(port string) bool {
	url := fmt.Sprintf("http://127.0.0.1:%s/json/version", port)
	resp, err := http.Get(url)
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

	// NEW: Check if T is a slice of structs (e.g., []Al)
	isSliceOfStruct := val.Kind() == reflect.Slice && val.Type().Elem().Kind() == reflect.Struct
	if isSliceOfStruct {
		// Slice will be built by appending below
	}

	// Handle Map Types (unchanged)
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
					// Fallback to original behavior if no extraction function is provided
					data = ExtractTextOrAttr(selection, sel.Attr)
				}
				if data != "" { // Skip empty
					dataArray = append(dataArray, data)
				}
			})
			// Debug: Print first 3 elements if they exist, otherwise print all
			if len(dataArray) >= 3 {
				fmt.Printf("Extracted %d items for '%s': %v\n", len(dataArray), sel.Name, dataArray[:3])
			} else {
				fmt.Printf("Extracted %d items for '%s': %v\n", len(dataArray), sel.Name, dataArray)
			}

			if isSliceOfStruct {
				// For slice of structs with array data:
				// If no structs exist yet, create them
				// If structs exist, try to update them (assuming 1:1 mapping)
				elemType := val.Type().Elem()

				// If the slice is empty, create structs for each array item
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
					// If structs already exist, update each one with the corresponding array item
					// If there are more array items than structs, create new structs
					for i, data := range dataArray {
						var targetElem reflect.Value
						if i < val.Len() {
							// Update existing struct
							targetElem = val.Index(i)
						} else {
							// Create new struct
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
				// Existing: Set slice field in struct
				field := val.FieldByName(sel.Name)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String {
					field.Set(reflect.ValueOf(dataArray))
				}
			} else if val.Kind() == reflect.Map {
				// Existing: Set map key to array
				val.SetMapIndex(reflect.ValueOf(sel.Name), reflect.ValueOf(dataArray))
			}
		} else {
			// Single value
			selection := doc.Find(sel.Query)

			var data string
			if sel.ExtractFunc != nil {
				data = sel.ExtractFunc(selection)
			} else {
				// Fallback to original behavior if no extraction function is provided
				data = ExtractTextOrAttr(selection, sel.Attr)
			}

			if isSliceOfStruct && data != "" {
				// For slice of structs, we need to update all elements with the single value
				// This is appropriate for page-level data like body content
				for i := 0; i < val.Len(); i++ {
					elem := val.Index(i)
					field := elem.FieldByName(sel.Name)
					if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
						field.SetString(data)
					}
				}

				// If no elements exist yet, create one and add it
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
				// Existing
				field := val.FieldByName(sel.Name)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
					field.SetString(data)
				}
			} else if val.Kind() == reflect.Map {
				// Existing
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
			// For slice of structs, add URL to all elements
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i)
				field := elem.FieldByName(s.URLFieldName)
				if field.IsValid() && field.CanSet() && field.Kind() == reflect.String {
					field.SetString(currentURL)
				}
			}
		}
	}

	// Handle LinkHunt after processing selectors
	if s.LinkHunt != "" {
		doc.Find(fmt.Sprintf("%s", s.LinkHunt)).Each(func(i int, selection *goquery.Selection) {
			link, exists := selection.Attr("href")
			if !exists {
				return
			}

			link = strings.TrimSpace(link)
			if !strings.HasPrefix(link, "http") && !strings.HasPrefix(link, "//") && !strings.HasPrefix(link, "#") {
				// check if link starts with a slash
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

			// Check if the URL has already been visited to prevent duplicates
			s.visitedMutex.RLock()
			_, visited := s.visitedURLs[link]
			s.visitedMutex.RUnlock()

			if visited {
				fmt.Println("URL already visited, skipping:", link)
				return // Skip if already visited
			}

			// Mark the URL as visited
			s.visitedMutex.Lock()
			s.visitedURLs[link] = true
			s.visitedMutex.Unlock()

			// Queue the product page scraping
			req, err := client.NewRequest("GET", link, nil)
			if err != nil {
				log.Println("Failed to create request:", err)
				return
			}

			req.Rendered = true

			// Use a closure to capture `link`
			req.Actions = []chromedp.Action{
				// Set cookies for the link
				chromedp.ActionFunc(func(ctx context.Context) error {
					parsedURL, err := url.Parse(link)
					if err != nil {
						log.Println("Failed to parse link URL for cookie domain:", err)
						return err
					}
					domain := parsedURL.Host
					if !strings.HasPrefix(domain, ".") {
						domain = "." + domain
					}

					for _, cookie := range s.Cookies {
						err := network.SetCookie(cookie["name"], cookie["value"]).
							WithDomain(domain).
							WithPath("/").
							WithHTTPOnly(false).
							WithSecure(false).
							Do(ctx)
						if err != nil {
							log.Println("Failed to set cookie for link:", cookie["name"], err)
							// Continue with other cookies even if one fails
						}
					}
					return nil
				}),

				chromedp.Navigate(link),
				chromedp.WaitReady(":root"),
				chromedp.ActionFunc(func(ctx context.Context) error {
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
						result = s.parseWithSelectors(doc, g, baseUrl, link) // Pass the link as currentURL
					}
					// check if the result is empty
					if reflect.ValueOf(result).IsZero() {
						fmt.Println("result is empty")
						return nil
					}

					s.ExportChan <- result
					return nil
				}),
			}

			// Queue it in Geziyor
			g.Do(req, g.Opt.ParseFunc)
		})
	}

	return result
}

// ScrapeStream starts scraping and returns a channel that will receive results as they are scraped
func (s *Scrapper[T]) ScrapeStream() (<-chan T, error) {
	err := startBrowserIfNotRunning()
	if err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	// Initialize the visited URLs map if it hasn't been initialized yet
	if s.visitedURLs == nil {
		s.visitedURLs = make(map[string]bool)
	}

	s.ExportChan = make(chan T, 100)

	g := geziyor.NewGeziyor(&geziyor.Options{
		StartRequestsFunc: func(g *geziyor.Geziyor) {
			for _, u := range s.Urls {
				// Check if the URL has already been visited to prevent duplicates
				s.visitedMutex.RLock()
				_, visited := s.visitedURLs[u]
				s.visitedMutex.RUnlock()

				if visited {
					fmt.Println("Initial URL already visited, skipping:", u)
					continue // Skip if already visited
				}

				// Mark the URL as visited
				s.visitedMutex.Lock()
				s.visitedURLs[u] = true
				s.visitedMutex.Unlock()

				req, err := client.NewRequest("GET", u, nil)
				if err != nil {
					log.Println("Failed to create request:", err)
					continue
				}
				// get the base url
				ur, err := url.Parse(u)
				if err != nil {
					log.Println("Failed to parse url:", err)
					continue
				}
				baseUrl := ur.Scheme + "://" + ur.Host

				req.Rendered = true

				req.Actions = []chromedp.Action{
					chromedp.Navigate("about:blank"),
					// chromedp.ActionFunc(func(ctx context.Context) error {
					// 	return nil
					// }),

					// 1️⃣ Set cookies BEFORE navigation
					chromedp.ActionFunc(func(ctx context.Context) error {
						parsedURL, err := url.Parse(u)
						if err != nil {
							log.Println("Failed to parse URL for cookie domain:", err)
							return err
						}
						domain := parsedURL.Host
						if !strings.HasPrefix(domain, ".") {
							domain = "." + domain
						}

						for _, cookie := range s.Cookies {
							err := network.SetCookie(cookie["name"], cookie["value"]).
								WithDomain(domain).
								WithPath("/").
								WithHTTPOnly(false).
								WithSecure(false).
								Do(ctx)
							if err != nil {
								log.Println("Failed to set cookie:", cookie["name"], err)
								return err
							}
						}
						return nil
					}),

					// 2️⃣ Navigate to the URL
					chromedp.Navigate(u),

					// 3️⃣ Wait until the page root is ready
					chromedp.WaitReady(":root"),

					// 4️⃣ Execute pre-scraping actions
					chromedp.ActionFunc(func(ctx context.Context) error {
						for _, action := range s.PreScrapeActions {
							switch action.Type {
							case ClickAction:
								// Click the element specified by the selector
								err := chromedp.Click(action.Selector).Do(ctx)
								if err != nil {
									log.Printf("Error clicking element %s: %v", action.Selector, err)
									// Continue with other actions even if one fails
									continue
								}

								// If there's a wait condition, wait for it
								if action.WaitUntil != "" {
									err = chromedp.WaitVisible(action.WaitUntil).Do(ctx)
									if err != nil {
										log.Printf("Error waiting for element %s: %v", action.WaitUntil, err)
									}
								} else {
									// Default wait of 1 second after click
									time.Sleep(1 * time.Second)
								}

							case ScrollAction:
								// Scroll to the element specified by the selector
								err := chromedp.ScrollIntoView(action.Selector).Do(ctx)
								if err != nil {
									log.Printf("Error scrolling to element %s: %v", action.Selector, err)
									continue
								}

								if action.WaitUntil != "" {
									err = chromedp.WaitVisible(action.WaitUntil).Do(ctx)
									if err != nil {
										log.Printf("Error waiting for element %s: %v", action.WaitUntil, err)
									}
								} else {
									time.Sleep(1 * time.Second)
								}

							case WaitAction:
								// Wait for the specified duration
								time.Sleep(action.Duration)
							}
						}
						return nil
					}),

					// 5️⃣ Extract full HTML and parse
					chromedp.ActionFunc(func(ctx context.Context) error {
						node, err := dom.GetDocument().Do(ctx)
						if err != nil {
							return err
						}
						body, err := dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
						if err != nil {
							return err
						}

						var result T
						doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
						if err != nil {
							return err
						}

						if s.ParseFunc != nil {
							result = s.ParseFunc(doc)
						} else {
							result = s.parseWithSelectors(doc, g, baseUrl, u) // Pass the URL as currentURL
						}

						s.ExportChan <- result
						if s.EachEvent != nil {
							s.EachEvent(result)
						}
						return nil
					}),
				}

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

// Generic scraping function that accepts URLs and expected scraping selectors
// This maintains backward compatibility by collecting all results before returning
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

// CloseBrowser closes the browser instance if it's running
func (s *Scrapper[T]) CloseBrowser() {
	stopBrowser()
}
