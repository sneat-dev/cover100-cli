//go:build windows

package tscov

// runnerExecutableSuffix is the extension npm installs its runner shim with on
// Windows: node_modules/.bin/jest is really jest.cmd there.
//
// It lives in a build-tagged file rather than in an `if runtime.GOOS ==
// "windows"` inside findRunnerBinary, because a GOOS comparison is a
// compile-time constant: on every platform but one the branch is dead, yet the
// coverage tool still instruments it and counts it as an uncovered statement.
// Splitting it per platform keeps each build's coverage honest, and CI's
// Windows job is what proves this file.
const runnerExecutableSuffix = ".cmd"
