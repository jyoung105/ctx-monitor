package renderer

import (
	"strings"
	"testing"
)

func TestGetHTMLTemplateNonEmpty(t *testing.T) {
	tmpl := GetHTMLTemplate()
	if len(tmpl) == 0 {
		t.Error("GetHTMLTemplate() returned empty string")
	}
}

func TestGetHTMLTemplateDoctype(t *testing.T) {
	tmpl := GetHTMLTemplate()
	if !strings.Contains(tmpl, "<!DOCTYPE html>") {
		t.Error("GetHTMLTemplate() does not contain '<!DOCTYPE html>'")
	}
}

func TestGetHTMLTemplateTitle(t *testing.T) {
	tmpl := GetHTMLTemplate()
	if !strings.Contains(tmpl, "ctx-monitor") {
		t.Error("GetHTMLTemplate() does not contain 'ctx-monitor' title")
	}
}

func TestGetHTMLTemplateAPIEndpoint(t *testing.T) {
	tmpl := GetHTMLTemplate()
	if !strings.Contains(tmpl, "/api/data") {
		t.Error("GetHTMLTemplate() does not contain '/api/data' fetch endpoint")
	}
}
