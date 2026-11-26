// Package crew embeds rig-crew's content tree.
package crew

import (
	"embed"
	"io/fs"
)

//go:embed pool roles workflows harnesses schema
var content embed.FS

// Content returns the embedded content tree rooted at the repository layout.
func Content() fs.FS { return content }
