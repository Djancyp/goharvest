package pkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPreScrapeActionTypeConstants tests the PreScrapeActionType constants
func TestPreScrapeActionTypeConstants(t *testing.T) {
	assert.Equal(t, PreScrapeActionType(0), ClickAction)
	assert.Equal(t, PreScrapeActionType(1), ScrollAction)
	assert.Equal(t, PreScrapeActionType(2), WaitAction)
}

// TestPreScrapeActionStructure tests the PreScrapeAction struct fields
func TestPreScrapeActionStructure(t *testing.T) {
	action := PreScrapeAction{
		Type:      ClickAction,
		Selector:  ".button-class",
		Duration:  2 * time.Second,
		WaitUntil: ".loaded-class",
	}

	assert.Equal(t, ClickAction, action.Type)
	assert.Equal(t, ".button-class", action.Selector)
	assert.Equal(t, 2*time.Second, action.Duration)
	assert.Equal(t, ".loaded-class", action.WaitUntil)
}

// TestPreScrapeActionTypes tests different action types
func TestPreScrapeActionTypes(t *testing.T) {
	actions := []PreScrapeAction{
		{Type: ClickAction, Selector: "#button"},
		{Type: ScrollAction, Selector: ".scroll-area"},
		{Type: WaitAction, Duration: 1 * time.Second},
	}

	assert.Equal(t, ClickAction, actions[0].Type)
	assert.Equal(t, ScrollAction, actions[1].Type)
	assert.Equal(t, WaitAction, actions[2].Type)
}

// TestScrapperWithPreScrapeActions tests the Scrapper with pre-scraping actions
func TestScrapperWithPreScrapeActions(t *testing.T) {
	type FormData struct {
		InputValue string `json:"input_value"`
		Result     string `json:"result"`
	}

	mockServer := NewMockServer(FormHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[FormData]{
		Urls: []string{mockServer.URL},
		PreScrapeActions: []PreScrapeAction{
			{
				Type:     ClickAction,
				Selector: "#search-form button",
			},
			{
				Type:     WaitAction,
				Duration: 100 * time.Millisecond,
			},
		},
		Selectors: []Selector{
			{
				Name:  "InputValue",
				Query: "#search-input",
				Attr:  "value",
			},
			{
				Name:  "Result",
				Query: ".result:first",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	// Note: Since this is a static HTML test, the form submission won't actually happen
	// This test verifies that the pre-scraping actions can be configured
	assert.Equal(t, "search term", result.InputValue)
	// The result might be empty since the form submission doesn't actually happen in static HTML
}

// TestScrapperWithMultiplePreScrapeActions tests multiple pre-scraping actions
func TestScrapperWithMultiplePreScrapeActions(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		PreScrapeActions: []PreScrapeAction{
			{
				Type:     ClickAction,
				Selector: "body",
			},
			{
				Type:     ScrollAction,
				Selector: "h1",
			},
			{
				Type:     WaitAction,
				Duration: 50 * time.Millisecond,
			},
		},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Test Title", result.Title)
}

// TestPreScrapeActionWithWaitUntil tests pre-scraping action with WaitUntil field
func TestPreScrapeActionWithWaitUntil(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls: []string{mockServer.URL},
		PreScrapeActions: []PreScrapeAction{
			{
				Type:      ClickAction,
				Selector:  "body",
				WaitUntil: "h1", // Wait for h1 to be visible after click
			},
		},
		Selectors: []Selector{
			{
				Name:  "Title",
				Query: "#title",
			},
		},
		RequestDelay: 100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Test Title", result.Title)
}

// TestEmptyPreScrapeActions tests behavior when no pre-scraping actions are defined
func TestEmptyPreScrapeActions(t *testing.T) {
	type SimpleData struct {
		Title string `json:"title"`
	}

	mockServer := NewMockServer(SimpleHTML)
	defer mockServer.Close()

	scrapper := &Scrapper[SimpleData]{
		Urls:               []string{mockServer.URL},
		PreScrapeActions:   []PreScrapeAction{}, // Empty slice
		Selectors:          []Selector{{Name: "Title", Query: "#title"}},
		RequestDelay:       100 * time.Millisecond,
	}

	results, err := scrapper.Scrape()
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	assert.Equal(t, "Test Title", result.Title)
}