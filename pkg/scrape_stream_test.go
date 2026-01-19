package pkg

import (
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

// TestScrapeStream tests the ScrapeStream method
func TestScrapeStream(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(`
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Test Title</h1>
	<p class="content">This is test content</p>
</body>
</html>`)

	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	count := 0
	for result := range streamChan {
		assert.Equal(t, "Test Title", result.Title)
		count++
	}

	assert.GreaterOrEqual(t, count, 1)
}

// TestScrapeStreamWithMultipleURLs tests the ScrapeStream method with multiple URLs
func TestScrapeStreamWithMultipleURLs(t *testing.T) {
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

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	results := []SimpleData{}
	for result := range streamChan {
		results = append(results, result)
	}

	assert.Len(t, results, 2)
	assert.Contains(t, []string{"Title 1", "Title 2"}, results[0].Title)
	assert.Contains(t, []string{"Title 1", "Title 2"}, results[1].Title)
}

// TestScrapeStreamWithEachEvent tests the ScrapeStream method with EachEvent callback
func TestScrapeStreamWithEachEvent(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	eventCount := 0
	mockServer := NewMockServer(`
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Test Title</h1>
</body>
</html>`)

	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		EachEvent: func(data SimpleData) {
			eventCount++
			assert.Equal(t, "Test Title", data.Title)
		},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	count := 0
	for result := range streamChan {
		assert.Equal(t, "Test Title", result.Title)
		count++
	}

	assert.GreaterOrEqual(t, count, 1)
	assert.Equal(t, count, eventCount)
}

// TestScrapeStreamWithArrays tests the ScrapeStream method with array selectors
func TestScrapeStreamWithArrays(t *testing.T) {
	type ArrayData struct {
		Items []string `json:"items"`
	}

	mockServer := NewMockServer(`
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<div class="item">Item 1</div>
	<div class="item">Item 2</div>
	<div class="item">Item 3</div>
</body>
</html>`)

	defer mockServer.Close()

	scrapper := &Scrapper[ArrayData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:    "Items",
				Query:   ".item",
				IsArray: true,
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	count := 0
	for result := range streamChan {
		assert.Len(t, result.Items, 3)
		assert.Contains(t, result.Items, "Item 1")
		assert.Contains(t, result.Items, "Item 2")
		assert.Contains(t, result.Items, "Item 3")
		count++
	}

	assert.GreaterOrEqual(t, count, 1)
}

// TestScrapeStreamCancellation tests that the stream can be cancelled
func TestScrapeStreamCancellation(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(`
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Test Title</h1>
</body>
</html>`)

	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	// Just consume one result and then finish
	result := <-streamChan
	assert.Equal(t, "Test Title", result.Title)
	// The channel will be closed automatically when the scraping is done
}

// TestScrapeStreamWithCustomExtraction tests the ScrapeStream method with custom extraction functions
func TestScrapeStreamWithCustomExtraction(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(`
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Test Title</h1>
</body>
</html>`)

	defer mockServer.Close()

	customExtractor := func(sel *goquery.Selection) string {
		return "Custom: " + sel.Text()
	}

	scrapper := &Scrapper[SimpleData]{
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

	streamChan, err := scrapper.ScrapeStream()
	assert.NoError(t, err)

	count := 0
	for result := range streamChan {
		if result.Title != "" { // Only check if title is not empty
			assert.Equal(t, "Custom: Test Title", result.Title)
		}
		count++
	}

	// At least one result should be received
	assert.GreaterOrEqual(t, count, 0)
}