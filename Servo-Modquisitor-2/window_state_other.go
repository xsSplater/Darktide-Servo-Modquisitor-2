//go:build !windows

// window_state_other.go
package main

func maximizeWindowByTitle(title string)  {}
func isWindowMaximized(title string) bool { return false }
