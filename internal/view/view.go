// Package view renders the self-contained HTML document used when the report
// is opened over file://.
//
// A browser blocks fetch() against file:// URLs, so the served page's fetch of
// ?data= cannot work there. The standalone document therefore inlines the same
// stylesheet and script the server serves, plus the report itself as
// window.__COVER100_DATA__, which the page prefers over any fetch. Both forms
// are generated from the same source files, so they cannot drift.
package view

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// Source file names inside the public asset filesystem.
const (
	IndexFile  = "index.html"
	StyleFile  = "styles.css"
	ScriptFile = "app.js"
)

// Markers the CLI replaces. They must appear exactly once in index.html; a
// missing marker is an error rather than a silent partial inlining, because
// the result would look fine until the moment it was opened offline.
const (
	styleMarker  = `<link rel="stylesheet" href="styles.css">`
	scriptMarker = `<script src="app.js"></script>`
	dataVar      = "window.__COVER100_DATA__"
)

// Standalone renders index.html with the stylesheet, script and report inlined.
func Standalone(assets fs.FS, report []byte) ([]byte, error) {
	index, err := fs.ReadFile(assets, IndexFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", IndexFile, err)
	}
	css, err := fs.ReadFile(assets, StyleFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", StyleFile, err)
	}
	script, err := fs.ReadFile(assets, ScriptFile)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ScriptFile, err)
	}

	data, err := embedJSON(report)
	if err != nil {
		return nil, err
	}

	html := string(index)
	if n := strings.Count(html, styleMarker); n != 1 {
		return nil, fmt.Errorf("%s must contain %q exactly once, found %d", IndexFile, styleMarker, n)
	}
	if n := strings.Count(html, scriptMarker); n != 1 {
		return nil, fmt.Errorf("%s must contain %q exactly once, found %d", IndexFile, scriptMarker, n)
	}

	html = strings.Replace(html, styleMarker, "<style>\n"+string(css)+"\n</style>", 1)
	inlined := "<script>" + dataVar + " = " + string(data) + ";</script>\n" +
		"<script>\n" + neutraliseClosingTag(string(script)) + "\n</script>"
	html = strings.Replace(html, scriptMarker, inlined, 1)

	return []byte(html), nil
}

// embedJSON validates the report and returns it as a script-safe JSON literal.
func embedJSON(report []byte) ([]byte, error) {
	var probe map[string]any
	if err := json.Unmarshal(report, &probe); err != nil {
		return nil, fmt.Errorf("report is not valid JSON: %w", err)
	}
	// A JSON string may legally escape "/" as "\/", which is what stops a
	// "</script>" inside any string value from closing the host script element.
	return bytes.ReplaceAll(report, []byte("</"), []byte(`<\/`)), nil
}

// neutraliseClosingTag escapes any "</script" inside the inlined script.
func neutraliseClosingTag(script string) string {
	if !strings.Contains(script, "</script") {
		return script
	}
	return strings.ReplaceAll(script, "</script", `<\/script`)
}
