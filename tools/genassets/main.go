// genassets writes the SVG sources for the plugin's static images into a directory.
// Convert them to PNG with any SVG renderer (scripts/build.sh uses rsvg-convert).
//
//	go run ./tools/genassets <dir>
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Marco-caizzi/redragon-ss552-weather/internal/render"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: genassets <dir>")
		os.Exit(2)
	}
	dir := os.Args[1]
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}
	files := map[string]string{
		"icon.svg": render.AppIconSVG(),
		"key.svg":  render.KeyStatusSVG(render.StatusLoading),
		"knob.svg": render.KnobStatusSVG(render.StatusLoading),
	}
	for name, svg := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(svg), 0o644); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
