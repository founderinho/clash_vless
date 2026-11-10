//go:build (android || !cgo) && !windows

package gui

import "clashvless/internal/control"

func Preflight() error { return ErrUnavailable }

func RunClient(*control.Client, bool) error { return ErrUnavailable }
