package goharvest

import (
	"github.com/Djancyp/goharvest/pkg"
)

// Re-export types from the pkg module
type (
	Scrapper[T any]     = pkg.Scrapper[T]
	Selector            = pkg.Selector
	PreScrapeAction     = pkg.PreScrapeAction
	ExtractionFunc      = pkg.ExtractionFunc
	PreScrapeActionType = pkg.PreScrapeActionType
	Options             = pkg.Options
	BrowserOptions      = pkg.BrowserOptions
)

// Re-export constants from the pkg module
const (
	ClickAction pkg.PreScrapeActionType = iota
	ScrollAction
	WaitAction
)

// Re-export functions from the pkg module
var (
	Text                   = pkg.Text
	FirstText              = pkg.FirstText
	HTML                   = pkg.HTML
	Attr                   = pkg.Attr
	ExtractTextOrAttr      = pkg.ExtractTextOrAttr
	DefaultBrowserOptions  = pkg.DefaultBrowserOptions
	StealthBrowserOptions  = pkg.StealthBrowserOptions
	DefaultAntiDetectionOptions = pkg.DefaultAntiDetectionOptions
	DockerBrowserOptions   = pkg.DockerBrowserOptions
)
