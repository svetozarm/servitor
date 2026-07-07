package server

import (
	"embed"
	"html/template"
)

//go:embed assets/index.html
var indexTemplateRaw string

//go:embed assets/hero.png
var heroPNG []byte

//go:embed assets
var assetsFS embed.FS

var indexTemplate = template.Must(template.New("index").Parse(indexTemplateRaw))
