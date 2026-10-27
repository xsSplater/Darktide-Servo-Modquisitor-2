//go:build linux

// Servo-Modquisitor-2/trash_linux.go
package main

import (
	"context"

	"github.com/xsSplater/xsstrashy"
)

func trashPaths(paths []string) error {
	return xsstrashy.MoveContext(context.Background(), paths...)
}
