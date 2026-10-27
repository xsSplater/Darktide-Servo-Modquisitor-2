//go:build windows

// Servo-Modquisitor-2/trash_windows.go
package main

import (
	"context"

	"github.com/xsSplater/xsstrashy"
)

func trashPaths(paths []string) error {
	// Используем MoveContext, чтобы можно было в будущем добавить таймаут.
	return xsstrashy.MoveContext(context.Background(), paths...)
}
