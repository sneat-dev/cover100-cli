package view

import (
	"strings"
	"testing"
	"testing/fstest"
)

func assets(index string) fstest.MapFS {
	return fstest.MapFS{
		IndexFile:  &fstest.MapFile{Data: []byte(index)},
		StyleFile:  &fstest.MapFile{Data: []byte("body { color: red }")},
		ScriptFile: &fstest.MapFile{Data: []byte("const x = 1;")},
	}
}

const validIndex = `<!DOCTYPE html><html><head>` + styleMarker +
	`</head><body><div id="treemap"></div>` + scriptMarker + `</body></html>`

func TestStandalone_InlinesAssetsAndData(t *testing.T) {
	out, err := Standalone(assets(validIndex), []byte(`{"root":"/tmp/app","tree":{"id":"root"}}`))
	if err != nil {
		t.Fatalf("Standalone() error = %v", err)
	}
	html := string(out)

	for _, want := range []string{
		"<style>\nbody { color: red }\n</style>",
		"const x = 1;",
		`window.__COVER100_DATA__ = {"root":"/tmp/app","tree":{"id":"root"}};`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("standalone HTML missing %q", want)
		}
	}
	for _, unwanted := range []string{styleMarker, scriptMarker} {
		if strings.Contains(html, unwanted) {
			t.Errorf("standalone HTML still references an external asset: %q", unwanted)
		}
	}
}

func TestStandalone_EscapesClosingScriptTagInData(t *testing.T) {
	// A file named "</script>" must not be able to close the host script
	// element and turn the report into markup.
	out, err := Standalone(assets(validIndex), []byte(`{"name":"</script><script>alert(1)</script>"}`))
	if err != nil {
		t.Fatalf("Standalone() error = %v", err)
	}
	if strings.Contains(string(out), "</script><script>alert(1)") {
		t.Fatal("data was not escaped: a </script> in the report can break out of the script element")
	}
	if !strings.Contains(string(out), `<\/script>`) {
		t.Error("expected the escaped form \\/script in the output")
	}
}

func TestStandalone_RejectsMalformedJSON(t *testing.T) {
	if _, err := Standalone(assets(validIndex), []byte("{not json")); err == nil {
		t.Fatal("Standalone() must reject a report that is not valid JSON")
	}
}

func TestStandalone_RequiresBothIntegrationMarkers(t *testing.T) {
	tests := map[string]string{
		"missing style marker":  `<html><head></head><body>` + scriptMarker + `</body></html>`,
		"missing script marker": `<html><head>` + styleMarker + `</head><body></body></html>`,
		"duplicated marker":     validIndex + styleMarker,
	}
	for name, index := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Standalone(assets(index), []byte(`{}`)); err == nil {
				t.Fatal("Standalone() must fail loudly when the markup contract changes")
			}
		})
	}
}

func TestStandalone_MissingAsset(t *testing.T) {
	fsys := assets(validIndex)
	delete(fsys, ScriptFile)
	if _, err := Standalone(fsys, []byte(`{}`)); err == nil {
		t.Fatal("Standalone() must fail when an asset is missing")
	}
}
