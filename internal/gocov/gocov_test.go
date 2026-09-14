package gocov

import (
	"strings"
	"testing"
)

const sampleProfile = `mode: set
example.com/sample/calc/calc.go:14.20,16.2 1 1
example.com/sample/calc/calc.go:19.16,21.2 1 1
example.com/sample/calc/calc.go:24.16,26.2 1 0
example.com/sample/calc/calc.go:29.26,31.20 1 1
example.com/sample/calc/calc.go:31.20,33.3 1 0
example.com/sample/calc/calc.go:34.2,34.20 1 1
example.com/sample/calc/calc.go:38.31,40.16 1 0
example.com/sample/calc/calc.go:40.16,42.3 1 0
example.com/sample/calc/calc.go:43.2,43.33 1 0
example.com/sample/calc/calc_test.go:5.14,7.2 1 1
`

func TestParseProfile(t *testing.T) {
	files, err := ParseProfile(strings.NewReader(sampleProfile))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}

	calc := files["example.com/sample/calc/calc.go"]
	if calc == nil {
		t.Fatalf("calc.go missing from parsed profile; got %d files", len(files))
	}

	// Blocks expand across their range: 14-16, 19-21, 24-26, 29-31, 31-33,
	// 34, 38-40, 40-42 and 43 give 21 distinct lines, of which the blocks with
	// a positive count cover 10. Zero-count blocks must still appear, or the
	// denominator silently shrinks and every file looks fully covered.
	got := metricOf(calc)
	if got.Covered != 10 || got.Total != 21 {
		t.Errorf("calc.go metric = %d/%d, want 10/21", got.Covered, got.Total)
	}
	if _, ok := files["example.com/sample/calc/calc_test.go"]; !ok {
		t.Error("test file should still be parsed here; Run() is what filters it")
	}
}

func TestParseProfile_ExpandsLineRangesAndKeepsHighestCount(t *testing.T) {
	// A multi-line block must count every line it spans, and an overlapping
	// block must not lower an already-covered line's count.
	profile := `mode: count
example.com/x/a.go:10.1,13.2 4 0
example.com/x/a.go:12.1,12.9 1 7
`
	files, err := ParseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	lines := files["example.com/x/a.go"]
	for _, want := range []int{10, 11, 12, 13} {
		if _, ok := lines[want]; !ok {
			t.Errorf("line %d missing: block range was not expanded", want)
		}
	}
	if lines[12] != 7 {
		t.Errorf("line 12 count = %d, want 7 (highest wins)", lines[12])
	}
	if got := metricOf(lines); got.Covered != 1 || got.Total != 4 {
		t.Errorf("metric = %d/%d, want 1/4", got.Covered, got.Total)
	}
}

func TestParseProfile_ToleratesUnknownLines(t *testing.T) {
	profile := "mode: set\nnot a profile line\n\nexample.com/x/a.go:1.1,1.2 1 1\n"
	files, err := ParseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatalf("ParseProfile() error = %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("parsed %d files, want 1", len(files))
	}
}

func TestResolveProfilePath(t *testing.T) {
	tests := []struct {
		name        string
		profilePath string
		modulePath  string
		want        string
		wantOK      bool
	}{
		{"nested file", "example.com/sample/calc/calc.go", "example.com/sample", "calc/calc.go", true},
		{"module root file", "example.com/sample/main.go", "example.com/sample", "main.go", true},
		{"module itself", "example.com/sample", "example.com/sample", ".", true},
		{"dependency", "example.com/other/x.go", "example.com/sample", "", false},
		{"prefix but not a path segment", "example.com/sampleX/x.go", "example.com/sample", "", false},
		{"no module path", "example.com/sample/x.go", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ResolveProfilePath(tc.profilePath, tc.modulePath)
			if ok != tc.wantOK || got != tc.want {
				t.Errorf("ResolveProfilePath(%q, %q) = (%q, %v), want (%q, %v)",
					tc.profilePath, tc.modulePath, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
