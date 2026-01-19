package pkg

import (
	"io"
	"net/http"
	"net/http/httptest"
)

// MockServer creates a test server with predefined HTML content
type MockServer struct {
	Server *httptest.Server
	URL    string
}

// NewMockServer creates a new mock server with the provided HTML content
func NewMockServer(htmlContent string) *MockServer {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, htmlContent)
	}))
	
	return &MockServer{
		Server: server,
		URL:    server.URL,
	}
}

// NewMultiPageMockServer creates a mock server with multiple pages for link hunting tests
func NewMultiPageMockServer() *MockServer {
	mux := http.NewServeMux()
	
	// Main page with links to other pages
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `
<!DOCTYPE html>
<html>
<head><title>Main Page</title></head>
<body>
	<h1>Main Page</h1>
	<div class="link-class">
		<a href="/page1">Page 1</a>
		<a href="/page2">Page 2</a>
	</div>
	<p>This is the main content</p>
</body>
</html>`
		io.WriteString(w, html)
	})
	
	// Page 1
	mux.HandleFunc("/page1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `
<!DOCTYPE html>
<html>
<head><title>Page 1</title></head>
<body>
	<h1>Page 1</h1>
	<p>This is content from page 1</p>
	<span class="data">Data from Page 1</span>
</body>
</html>`
		io.WriteString(w, html)
	})
	
	// Page 2
	mux.HandleFunc("/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		html := `
<!DOCTYPE html>
<html>
<head><title>Page 2</title></head>
<body>
	<h1>Page 2</h1>
	<p>This is content from page 2</p>
	<span class="data">Data from Page 2</span>
</body>
</html>`
		io.WriteString(w, html)
	})
	
	server := httptest.NewServer(mux)
	return &MockServer{
		Server: server,
		URL:    server.URL,
	}
}

// Close shuts down the mock server
func (ms *MockServer) Close() {
	ms.Server.Close()
}

// HTML content templates for testing
var (
	SimpleHTML = `
<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1 id="title">Test Title</h1>
	<p class="content">This is test content</p>
	<img src="image.jpg" alt="Test Image" />
	<a href="https://example.com" class="link">Example Link</a>
	<div class="repeated">Item 1</div>
	<div class="repeated">Item 2</div>
	<div class="repeated">Item 3</div>
</body>
</html>`

	FormHTML = `
<!DOCTYPE html>
<html>
<head><title>Form Page</title></head>
<body>
	<form id="search-form">
		<input type="text" id="search-input" value="search term" />
		<button type="submit">Submit</button>
	</form>
	<div class="result">Result 1</div>
	<div class="result">Result 2</div>
</body>
</html>`

	JavascriptHTML = `
<!DOCTYPE html>
<html>
<head><title>Javascript Page</title></head>
<body>
	<div id="dynamic-content">Loading...</div>
	<script>
		setTimeout(function() {
			document.getElementById('dynamic-content').innerHTML = 'Dynamic Content Loaded';
		}, 100);
	</script>
</body>
</html>`
)