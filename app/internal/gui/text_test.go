//go:build !android && cgo

package gui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

func TestExitNamesWithParagraphSeparators(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	for name, separator := range map[string]string{
		"CR": "\r", "LF": "\n", "CRLF": "\r\n", "NEL": "\u0085",
		"line separator": "\u2028", "paragraph separator": "\u2029",
	} {
		t.Run(name, func(t *testing.T) {
			label := ellipsized("🇳🇱 Выход " + separator + strings.Repeat("Example exit ", 20))
			if label.Text != "🇳🇱 Выход "+strings.TrimSpace(strings.Repeat("Example exit ", 20)) {
				t.Fatal("display label did not normalize whitespace or preserve Unicode text")
			}
			for _, width := range []float32{630, 200, 30, 0, 500} {
				label.Resize(fyne.NewSize(width, 30))
				label.Refresh()
			}
		})
	}
}
