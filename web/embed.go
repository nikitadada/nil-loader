package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html docs/nil-loader_full.pdf
var content embed.FS

func StaticFS() fs.FS {
	return content
}
