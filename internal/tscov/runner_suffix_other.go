//go:build !windows

package tscov

// runnerExecutableSuffix is empty off Windows: the shim npm installs into
// node_modules/.bin is a plain executable file there. See the Windows file for
// why this is build-tagged rather than a runtime.GOOS comparison.
const runnerExecutableSuffix = ""
