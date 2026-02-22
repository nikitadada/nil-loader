package web

import (
	"embed"
	"io/fs"
)

//go:embed index.html
var content embed.FS

func StaticFS() fs.FS {
	return content
}
