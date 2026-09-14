// Package store has no test file on purpose, and the example relies on that.
//
// `go test -coverprofile ./...` still instruments a package with no tests, so
// this package is reported as 0% covered rather than omitted — which is
// exactly the shape a reader should expect to see in the treemap: a fully red
// box with a real denominator.
package store

// Memory is a trivial in-memory key/value store.
type Memory struct {
	values map[string]string
}

// New returns an empty store.
func New() *Memory {
	return &Memory{values: map[string]string{}}
}

// Set stores a value.
func (m *Memory) Set(key, value string) {
	m.values[key] = value
}

// Get returns the stored value and whether it was present.
func (m *Memory) Get(key string) (string, bool) {
	value, ok := m.values[key]
	return value, ok
}
