// Package model defines the normalized coverage model that cover100 collects
// from every supported language and hands to the treemap UI.
//
// The model is deliberately language-neutral: collectors (internal/gocov,
// internal/tscov) produce FileCoverage values, and aggregation
// (internal/aggregate) rolls them up into a single Node tree whose shape is
// repository -> package -> file -> function.
package model

// Metric is a covered/total pair for a single coverage dimension.
type Metric struct {
	Covered int `json:"covered"`
	Total   int `json:"total"`
}

// Add returns the element-wise sum of two metrics.
func (m Metric) Add(o Metric) Metric {
	return Metric{Covered: m.Covered + o.Covered, Total: m.Total + o.Total}
}

// Percent returns coverage as a fraction in [0,1]. ok is false when the
// dimension was not measured (Total == 0), which callers must render as
// "not measured" rather than as 0%.
func (m Metric) Percent() (float64, bool) {
	if m.Total <= 0 {
		return 0, false
	}
	return float64(m.Covered) / float64(m.Total), true
}

// Any returns 1 when at least one unit in the dimension is covered, else 0.
// It implements the model's "a package/file counts as covered when any line
// inside it is covered" rule.
func (m Metric) Any() int {
	if m.Total > 0 && m.Covered > 0 {
		return 1
	}
	return 0
}

// Metrics groups the five coverage dimensions reported for every tree node.
type Metrics struct {
	Lines        Metric `json:"lines"`
	Functions    Metric `json:"functions"`
	Files        Metric `json:"files"`
	Packages     Metric `json:"packages"`
	Repositories Metric `json:"repositories"`
}

// Node types. The tree is always repository -> package -> file -> function;
// the root node is the synthetic parent of every repository.
const (
	TypeRoot       = "root"
	TypeRepository = "repository"
	TypePackage    = "package"
	TypeFile       = "file"
	TypeFunction   = "function"
)

// Languages reported per node and used by the UI's language filter.
const (
	LangGo         = "go"
	LangTypeScript = "typescript"
	LangJavaScript = "javascript"
	LangMixed      = "mixed"
)

// MetricNames is the canonical metric order exposed in the report and UI.
var MetricNames = []string{"lines", "functions", "files", "packages", "repositories"}

// Node is one node of the aggregated coverage tree. Metrics is embedded so the
// five metric objects are serialized inline, exactly as the UI expects.
type Node struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Language is nil on the root node and "mixed" for a repository that holds
	// more than one language.
	Language *string `json:"language"`
	// Path is relative to the repository root for packages and files, and
	// relative to the scan root for repositories.
	Path string `json:"path,omitempty"`

	// Function-only detail.
	Line    *int  `json:"line,omitempty"`
	Hits    *int  `json:"hits,omitempty"`
	Covered *bool `json:"covered,omitempty"`

	// FunctionsAvailable is present (and false) on Go files: Go's cover
	// profiles carry no function-level data, so functions are reported 0/0.
	FunctionsAvailable *bool   `json:"functionsAvailable,omitempty"`
	Branches           *Metric `json:"branches,omitempty"`

	Metrics

	Children []*Node `json:"children,omitempty"`
}

// Report is the document written to --out and fetched by the treemap page.
type Report struct {
	GeneratedAt string   `json:"generatedAt"`
	Root        string   `json:"root"`
	Metrics     []string `json:"metrics"`
	Languages   []string `json:"languages"`
	Tree        *Node    `json:"tree"`

	// Additive fields. Consumers that only understand the contract above can
	// ignore them; they exist so the CLI can report partial failures in-band.
	Warnings []string `json:"warnings,omitempty"`
	Tool     string   `json:"tool,omitempty"`
}

// FuncCoverage is per-function detail collected from a language that can
// supply it (currently Istanbul-format JavaScript/TypeScript reports).
type FuncCoverage struct {
	Name    string
	Line    int
	Hits    int
	Lines   Metric
	Covered bool
}

// FileCoverage is a collector's result for a single source file. Path is
// always relative to the scan root and slash-separated, which is what makes
// node IDs globally unique without a per-collector naming scheme.
type FileCoverage struct {
	Path     string
	Language string

	Lines    Metric
	Branches *Metric

	Functions          Metric
	FunctionsAvailable bool
	FunctionList       []FuncCoverage
}

// StrPtr returns a pointer to s, for the optional JSON fields above.
func StrPtr(s string) *string { return &s }

// IntPtr returns a pointer to i.
func IntPtr(i int) *int { return &i }

// BoolPtr returns a pointer to b.
func BoolPtr(b bool) *bool { return &b }
