// Package branding exposes the shared Clashvless visual identity without a UI toolkit dependency.
package branding

import (
	"embed"
	"image/color"
)

const Name = "clashvless"

var (
	Background = color.NRGBA{R: 15, G: 19, B: 25, A: 255}
	Sidebar    = color.NRGBA{R: 19, G: 24, B: 31, A: 255}
	Surface    = color.NRGBA{R: 24, G: 30, B: 38, A: 255}
	Control    = color.NRGBA{R: 31, G: 39, B: 49, A: 255}
	Border     = color.NRGBA{R: 43, G: 53, B: 65, A: 255}
	Text       = color.NRGBA{R: 230, G: 235, B: 241, A: 255}
	Muted      = color.NRGBA{R: 145, G: 157, B: 171, A: 255}
	Accent     = color.NRGBA{R: 104, G: 213, B: 189, A: 255}
	AccentBlue = color.NRGBA{R: 105, G: 169, B: 255, A: 255}
	Danger     = color.NRGBA{R: 244, G: 126, B: 139, A: 255}
)

// Files contains the logo, tokens, SVG icons, and their Android-ready PNG versions.
//
//go:embed logo.svg tokens.json icons/*.svg png/*.png
var Files embed.FS

func Logo() ([]byte, error) {
	return Files.ReadFile("logo.svg")
}

func Tokens() ([]byte, error) {
	return Files.ReadFile("tokens.json")
}

func Icon(name string) ([]byte, error) {
	return Files.ReadFile("icons/" + name + ".svg")
}

func PNG(name string) ([]byte, error) {
	return Files.ReadFile("png/" + name + ".png")
}
