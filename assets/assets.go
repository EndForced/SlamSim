// Package assets встраивает картинки клеток в бинарник, чтобы релизу не нужна была папка assets/
package assets

import "embed"

//go:embed *.png
var FS embed.FS
