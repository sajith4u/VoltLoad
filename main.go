package main

import (
	"embed"

	"voltload/cmd"
)

// embed must live next to frontend/, so it is declared here and handed to cmd.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	cmd.Execute(assets)
}
