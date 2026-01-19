package pkg

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
)

// TestTextExtractionFunction tests the Text extraction function
func TestTextExtractionFunction(t *testing.T) {
	html := `<div><p>Hello   World</p></div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	selection := doc.Find("p")
	result := Text(selection)
	// The Text function trims whitespace but doesn't normalize internal spaces
	assert.Contains(t, result, "Hello")  // Contains "Hello"
	assert.Contains(t, result, "World") // Contains "World"
	// Just verify it's not empty and contains expected content
	assert.NotEmpty(t, result)
}

// TestFirstTextExtractionFunction tests the FirstText extraction function
func TestFirstTextExtractionFunction(t *testing.T) {
	html := `<div><p>First</p><p>Second</p></div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	selection := doc.Find("p")
	result := FirstText(selection)
	assert.Equal(t, "First", result)
}

// TestHTMLExtractionFunction tests the HTML extraction function
func TestHTMLExtractionFunction(t *testing.T) {
	html := `<div><p>Content</p></div>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	selection := doc.Find("div")
	result := HTML(selection)
	assert.Contains(t, result, "<p>Content</p>")
}

// TestAttrExtractionFunction tests the Attr extraction function
func TestAttrExtractionFunction(t *testing.T) {
	html := `<img src="image.jpg" alt="Test Image" />`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	selection := doc.Find("img")
	attrFunc := Attr("src")
	result := attrFunc(selection)
	assert.Equal(t, "image.jpg", result)

	attrFuncAlt := Attr("alt")
	resultAlt := attrFuncAlt(selection)
	assert.Equal(t, "Test Image", resultAlt)

	attrFuncMissing := Attr("missing")
	resultMissing := attrFuncMissing(selection)
	assert.Equal(t, "", resultMissing)
}

// TestExtractTextOrAttrFunction tests the ExtractTextOrAttr function
func TestExtractTextOrAttrFunction(t *testing.T) {
	html := `<a href="https://example.com">Example Link</a>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	selection := doc.Find("a")

	// Test with attribute
	result := ExtractTextOrAttr(selection, "href")
	assert.Equal(t, "https://example.com", result)

	// Test with empty attribute (should return text)
	result = ExtractTextOrAttr(selection, "")
	assert.Equal(t, "Example Link", result)

	// Test with non-existent attribute
	result = ExtractTextOrAttr(selection, "nonexistent")
	assert.Equal(t, "", result)
}

// TestSelectorStructure tests the Selector struct fields
func TestSelectorStructure(t *testing.T) {
	extractor := func(sel *goquery.Selection) string {
		return sel.Text()
	}

	selector := Selector{
		Name:        "TestField",
		Query:       ".test-class",
		Attr:        "href",
		IsArray:     true,
		ExtractFunc: extractor,
	}

	assert.Equal(t, "TestField", selector.Name)
	assert.Equal(t, ".test-class", selector.Query)
	assert.Equal(t, "href", selector.Attr)
	assert.Equal(t, true, selector.IsArray)
	assert.NotNil(t, selector.ExtractFunc)
}

// TestSelectorWithDifferentConfigurations tests different selector configurations
func TestSelectorWithDifferentConfigurations(t *testing.T) {
	html := `<div class="container">
		<span class="item" data-id="1">Item 1</span>
		<span class="item" data-id="2">Item 2</span>
		<span class="item" data-id="3">Item 3</span>
	</div>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	assert.NoError(t, err)

	// Test selector for text content
	selectorText := Selector{
		Name:  "Content",
		Query: ".item",
		Attr:  "",
	}
	selections := doc.Find(selectorText.Query)
	var results []string
	selections.Each(func(i int, selection *goquery.Selection) {
		data := ExtractTextOrAttr(selection, selectorText.Attr)
		if data != "" {
			results = append(results, data)
		}
	})
	assert.Len(t, results, 3)
	assert.Contains(t, results, "Item 1")
	assert.Contains(t, results, "Item 2")
	assert.Contains(t, results, "Item 3")

	// Test selector for attribute
	selectorAttr := Selector{
		Name:  "ID",
		Query: ".item",
		Attr:  "data-id",
	}
	var attrResults []string
	selections = doc.Find(selectorAttr.Query)
	selections.Each(func(i int, selection *goquery.Selection) {
		data := ExtractTextOrAttr(selection, selectorAttr.Attr)
		if data != "" {
			attrResults = append(attrResults, data)
		}
	})
	assert.Len(t, attrResults, 3)
	assert.Contains(t, attrResults, "1")
	assert.Contains(t, attrResults, "2")
	assert.Contains(t, attrResults, "3")
}