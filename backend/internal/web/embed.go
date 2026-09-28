// Package web serves the server-rendered React frontend, which is compiled
// into the binary via embed.FS. Run `npm run build` in ../frontend first; it
// writes the bundles into dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// ServerBundle is the JS evaluated by the SSR renderer.
func ServerBundle() ([]byte, error) {
	return fs.ReadFile(dist, "dist/server.js")
}

// Assets is the browser-facing files (JS, CSS) served under /assets.
func Assets() fs.FS {
	sub, err := fs.Sub(dist, "dist/assets")
	if err != nil {
		panic(err) // fs.Sub only fails on an invalid path literal
	}
	return sub
}
