//go:build cgm_web_embed

// Package web exposes the Cosplay Gallery Manager frontend to the product
// server without importing the legacy Stash UI package.
package web

import (
	"embed"
	"io/fs"
)

//go:embed build
var assets embed.FS

// FileSystem returns the production React 19 build rooted at its public files.
func FileSystem() (fs.FS, bool) {
	root, err := fs.Sub(assets, "build")
	if err != nil {
		panic(err)
	}
	return root, true
}
