//go:build windows

// Servo-Modquisitor-2/trash_windows.go
package main

import (
	"context"
	"log"

	"github.com/xsSplater/xsstrashy"
)

func trashPaths(paths []string) error {
	log.Printf("[trash] попытка удалить %d путей: %v", len(paths), paths)
	err := xsstrashy.MoveContext(context.Background(), paths...)
	if err != nil {
		log.Printf("[trash] ошибка MoveContext: %v", err)
	} else {
		log.Printf("[trash] MoveContext вернул nil (успех)")
	}
	return err
}
