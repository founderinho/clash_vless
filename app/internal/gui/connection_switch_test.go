//go:build !android

package gui

import (
	"testing"

	"fyne.io/fyne/v2/test"
)

func TestConnectionSwitchTapped(t *testing.T) {
	changed := false
	control := newConnectionSwitch(false, func(on bool) { changed = on })
	test.Tap(control)
	if !control.Checked || !changed {
		t.Fatal("tap must turn the connection switch on")
	}
	test.Tap(control)
	if control.Checked || changed {
		t.Fatal("second tap must turn the connection switch off")
	}
}
