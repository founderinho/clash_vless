//go:build !android

package main

import "clashvless/internal/store"

func platformInit(*store.State) {}

func platformTunFd(*store.State) error { return nil }
