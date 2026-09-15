//go:build !windows

// Servo-Modquisitor-2/window_state_other.go

package main

func maximizeWindowByTitle(title string)  {}
func isWindowMaximized(title string) bool { return false }
