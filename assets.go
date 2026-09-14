// Package cover100 holds the browser assets compiled into the cover100 binary.
//
// The public directory is embedded rather than read from disk so an installed
// binary can serve the viewer anywhere, with no companion files to lose.
package cover100

import "embed"

// PublicFS contains the treemap viewer: index.html, styles.css and app.js.
//
//go:embed public
var PublicFS embed.FS
