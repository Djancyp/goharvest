package pkg

import (
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

// TestScrapperInitialization tests the initialization of the Scrapper struct
func TestScrapperInitialization(t *testing.T) {
	scrapper := &Scrapper[struct{}]{
		Urls:                        []string{"https://example.com"},
		RequestDelay:                1 * time.Second,
		Timeout:                     30 * time.Second,
		RobotsTxtDisabled:           true,
		LogDisabled:                 false,
		ConcurrentRequests:          5,
		ConcurrentRequestsPerDomain: 2,
		UserAgent:                   "Test-Agent",
		Cookies:                     []map[string]string{{"name": "test", "value": "value"}},
		LinkHunt:                    ".link-class a",
	}

	assert.Equal(t, []string{"https://example.com"}, scrapper.Urls)
	assert.Equal(t, 1*time.Second, scrapper.RequestDelay)
	assert.Equal(t, 30*time.Second, scrapper.Timeout)
	assert.Equal(t, true, scrapper.RobotsTxtDisabled)
	assert.Equal(t, false, scrapper.LogDisabled)
	assert.Equal(t, 5, scrapper.ConcurrentRequests)
	assert.Equal(t, 2, scrapper.ConcurrentRequestsPerDomain)
	assert.Equal(t, "Test-Agent", scrapper.UserAgent)
	assert.Equal(t, []map[string]string{{"name": "test", "value": "value"}}, scrapper.Cookies)
	assert.Equal(t, ".link-class a", scrapper.LinkHunt)
}

// TestScrapperWithSimpleSelectors tests basic scraping functionality with simple selectors
func TestScrapperWithSimpleSelectors(t *testing.T) {
	type SimpleData struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Image   string `json:"image"`
		Link    string `json:"link"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
			{
				Name:  "Content",
				Query: ".content",
			},
			{
				Name:  "Image",
				Query: "img",
				Attr:  "src",
			},
			{
				Name:  "Link",
				Query: ".link",
				Attr:  "href",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Test Title", result.Title)
	assert.Equal(t, "This is test content", result.Content)
	assert.Equal(t, "image.jpg", result.Image)
	assert.Equal(t, "https://example.com", result.Link)
}

// TestScrapperWithArrays tests scraping with array selectors
func TestScrapperWithArrays(t *testing.T) {
	type ArrayData struct {
		Repeated []string `json:"repeated"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[ArrayData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:    "Repeated",
				Query:   ".repeated",
				IsArray: true,
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Len(t, result.Repeated, 3)
	assert.Contains(t, result.Repeated, "Item 1")
	assert.Contains(t, result.Repeated, "Item 2")
	assert.Contains(t, result.Repeated, "Item 3")
}

// TestScrapperWithCustomExtractionFunction tests scraping with custom extraction functions
func TestScrapperWithCustomExtractionFunction(t *testing.T) {
	type CustomData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	customExtractor := func(sel *goquery.Selection) string {
		text := sel.Text()
		return "Custom: " + text
	}

	scrapper := &Scrapper[CustomData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:        "Title",
				Query:       "#title",
				ExtractFunc: customExtractor,
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Custom: Test Title", result.Title)
}

// TestScrapperWithEmptyResults tests behavior when selectors don't match anything
func TestScrapperWithEmptyResults(t *testing.T) {
	type EmptyData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[EmptyData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#nonexistent",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "", result.Title)
}

// TestScrapperWithMultipleURLs tests scraping multiple URLs
func TestScrapperWithMultipleURLs(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer1 := NewMockServer(`<html><body><h1>Title 1</h1></body></html>`)
	mockServer2 := NewMockServer(`<html><body><h1>Title 2</h1></body></html>`)
	defer mockServer1.Close()
	defer mockServer2.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer1.URL, mockServer2.URL},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "h1",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Check that both titles are present regardless of order
	titles := []string{results[0].Title, results[1].Title}
	assert.Contains(t, titles, "Title 1")
	assert.Contains(t, titles, "Title 2")
}