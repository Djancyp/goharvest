package pkg

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestKeepBrowserOpen tests the KeepBrowserOpen functionality
func TestKeepBrowserOpen(t *testing.T) {
	// Create a mock server to simulate a webpage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`
			<html>
				<head><title>Test Page</title></head>
				<body>
					<h1 id="title">Test Title</h1>
					<p id="content">Test Content</p>
				</body>
			</html>`))
	}))
	defer server.Close()

	type TestData struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	// Test with KeepBrowserOpen = false (default behavior)
	t.Run("KeepBrowserOpen=false", func(t *testing.T) {
		scraper := &Scrapper[TestData]{
			Urls:         []string{server.URL},
			KeepBrowserOpen: false, // Explicitly set to false
			Selectors: []Selector{
				{Name: "Title", Query: "#title"},
				{Name: "Content", Query: "#content"},
			},
			RequestDelay: 10 * time.Millisecond, // Short delay for faster tests
		}

		results, err := scraper.Scrape()
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Test Title", results[0].Title)
		assert.Equal(t, "Test Content", results[0].Content)

		// Verify that the browser was closed after scraping
		// Since we can't directly check if the browser process is closed,
		// we rely on the implementation that should close it when KeepBrowserOpen is false
	})

	// Test with KeepBrowserOpen = true
	t.Run("KeepBrowserOpen=true", func(t *testing.T) {
		scraper := &Scrapper[TestData]{
			Urls:         []string{server.URL},
			KeepBrowserOpen: true, // Keep browser open after scraping
			Selectors: []Selector{
				{Name: "Title", Query: "#title"},
				{Name: "Content", Query: "#content"},
			},
			RequestDelay: 10 * time.Millisecond, // Short delay for faster tests
		}

		results, err := scraper.Scrape()
		assert.NoError(t, err)
		assert.Len(t, results, 1)
		assert.Equal(t, "Test Title", results[0].Title)
		assert.Equal(t, "Test Content", results[0].Content)

		// Browser should remain open, so we should be able to call CloseBrowser
		// to explicitly close it
		scraper.CloseBrowser()
	})

	// Test with ScrapeStream and KeepBrowserOpen = true
	t.Run("ScrapeStream with KeepBrowserOpen=true", func(t *testing.T) {
		scraper := &Scrapper[TestData]{
			Urls:         []string{server.URL},
			KeepBrowserOpen: true, // Keep browser open after scraping
			Selectors: []Selector{
				{Name: "Title", Query: "#title"},
				{Name: "Content", Query: "#content"},
			},
			RequestDelay: 10 * time.Millisecond, // Short delay for faster tests
		}

		streamChan, err := scraper.ScrapeStream()
		assert.NoError(t, err)

		count := 0
		for result := range streamChan {
			assert.Equal(t, "Test Title", result.Title)
			assert.Equal(t, "Test Content", result.Content)
			count++
		}
		assert.Equal(t, 1, count)

		// Browser should remain open, so we should be able to call CloseBrowser
		scraper.CloseBrowser()
	})

	// Test multiple scrapes with same browser instance
	t.Run("Multiple scrapes with same browser instance", func(t *testing.T) {
		// Create a second server to have a different URL for the second scrape
		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`
				<html>
					<head><title>Test Page 2</title></head>
					<body>
						<h1 id="title">Test Title 2</h1>
						<p id="content">Test Content 2</p>
					</body>
				</html>`))
		}))
		defer server2.Close()

		scraper := &Scrapper[TestData]{
			Urls:         []string{server.URL},
			KeepBrowserOpen: true, // Keep browser open after scraping
			Selectors: []Selector{
				{Name: "Title", Query: "#title"},
				{Name: "Content", Query: "#content"},
			},
			RequestDelay: 10 * time.Millisecond, // Short delay for faster tests
		}

		// First scrape
		results1, err := scraper.Scrape()
		assert.NoError(t, err)
		assert.Len(t, results1, 1)
		assert.Equal(t, "Test Title", results1[0].Title)

		// Second scrape with different URL
		scraper.Urls = []string{server2.URL}
		results2, err := scraper.Scrape()
		assert.NoError(t, err)
		assert.Len(t, results2, 1)
		assert.Equal(t, "Test Title 2", results2[0].Title)

		// Close browser when done
		scraper.CloseBrowser()
	})
}