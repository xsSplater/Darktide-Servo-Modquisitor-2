//go:build linux

// Servo-Modquisitor-2/trash_linux.go
package main

import "github.com/Bios-Marcel/wastebasket/v2"

func trashPaths(paths []string) error {
	return wastebasket.Trash(paths...)
}
