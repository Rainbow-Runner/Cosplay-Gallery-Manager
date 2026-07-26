//go:build !cgm_web_embed

// Package web exposes the Cosplay Gallery Manager frontend to the product
// server without importing the legacy Stash UI package.
package web

import "io/fs"

// FileSystem reports that no frontend was embedded in a developer build.
func FileSystem() (fs.FS, bool) {
	return nil, false
}
