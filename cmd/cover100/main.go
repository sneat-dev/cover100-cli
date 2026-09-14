// Command cover100 collects Go and TypeScript/JavaScript test coverage,
// normalizes it into one JSON document, and opens an interactive treemap of it.
package main

import (
	"io/fs"
	"os"

	cover100 "github.com/sneat-dev/cover100-cli"
	"github.com/sneat-dev/cover100-cli/internal/cli"
)

func main() {
	run(os.Args, cli.Run, cli.Fatal)
}

// run is the testable core of main: it calls runFn and hands any error to
// fatalFn, which owns the process exit code.
func run(args []string, runFn func([]string, fs.FS) error, fatalFn func(error)) {
	if err := runFn(args, cover100.PublicFS); err != nil {
		fatalFn(err)
	}
}
