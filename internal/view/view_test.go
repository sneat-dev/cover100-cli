package view

import (
	"fmt"
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

func TestStandalone_NamesTheMissingAsset(t *testing.T) {
	// Each of the three reads has its own error return; deleting one asset at a
	// time proves every read is guarded and that the message names the file.
	for _, file := range []string{IndexFile, StyleFile, ScriptFile} {
		t.Run(file, func(t *testing.T) {
			fsys := assets(validIndex)
			delete(fsys, file)
			out, err := Standalone(fsys, []byte(`{}`))
			if err == nil {
				t.Fatalf("Standalone() with %s deleted must fail, but produced %d bytes of HTML", file, len(out))
			}
			if !strings.Contains(err.Error(), "reading "+file) {
				t.Errorf("error = %v, want it to name the unreadable %s", err, file)
			}
		})
	}
}

func TestStandalone_MarkerCountsAreReported(t *testing.T) {
	tests := map[string]struct {
		index string
		want  string
	}{
		"style marker twice": {
			index: `<html><head>` + styleMarker + styleMarker + `</head><body>` + scriptMarker + `</body></html>`,
			want:  fmt.Sprintf("%q exactly once, found 2", styleMarker),
		},
		"script marker twice": {
			index: `<html><head>` + styleMarker + `</head><body>` + scriptMarker + scriptMarker + `</body></html>`,
			want:  fmt.Sprintf("%q exactly once, found 2", scriptMarker),
		},
		"script marker missing": {
			index: `<html><head>` + styleMarker + `</head><body></body></html>`,
			want:  fmt.Sprintf("%q exactly once, found 0", scriptMarker),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := Standalone(assets(tc.index), []byte(`{}`))
			if err == nil {
				t.Fatalf("Standalone() accepted invalid markup, producing %d bytes", len(out))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestStandalone_NeutralisesClosingTagInsideScript(t *testing.T) {
	fsys := assets(validIndex)
	fsys[ScriptFile] = &fstest.MapFile{Data: []byte(`var html = "</script><script>alert(1)</script>";`)}

	out, err := Standalone(fsys, []byte(`{}`))
	if err != nil {
		t.Fatalf("Standalone() error = %v", err)
	}
	html := string(out)

	// Without the rewrite the first "</script>" would close the element that
	// carries app.js and leave the rest of the script as page markup.
	if strings.Contains(html, `var html = "</script><script>alert(1)</script>";`) {
		t.Error("inlined app.js still contains a raw </script>; it can escape the host script element")
	}
	if !strings.Contains(html, `var html = "<\/script><script>alert(1)<\/script>";`) {
		t.Errorf("inlined app.js did not rewrite every closing tag: %s", html)
	}
}

func TestNeutraliseClosingTag(t *testing.T) {
	t.Run("leaves script without a closing tag alone", func(t *testing.T) {
		const script = "const a = 1 < 2; // no closing tag here"
		if got := neutraliseClosingTag(script); got != script {
			t.Errorf("neutraliseClosingTag(%q) = %q, want it unchanged", script, got)
		}
	})
	t.Run("escapes every closing tag", func(t *testing.T) {
		const script = `document.write("</script>"); /* </script also */`
		want := `document.write("<\/script>"); /* <\/script also */`
		if got := neutraliseClosingTag(script); got != want {
			t.Errorf("neutraliseClosingTag() = %q, want %q", got, want)
		}
	})
}
