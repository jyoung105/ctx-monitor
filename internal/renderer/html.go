package renderer

import (
	_ "embed"
)

//go:embed dashboard.html
var dashboardHTML string

// GetHTMLTemplate returns the embedded HTML dashboard content.
func GetHTMLTemplate() string {
	return dashboardHTML
}
