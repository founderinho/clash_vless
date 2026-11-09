//go:build android

package gui

import (
	"errors"

	"clashvless/internal/control"
)

func RunClient(*control.Client, bool) error {
	return errors.New("the Android GUI is provided by MainActivity")
}
