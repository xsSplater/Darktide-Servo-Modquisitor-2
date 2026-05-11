//go:build !windows

package main

func maximizeWindowByTitle(title string) {}
func isWindowMaximized(title string) bool { return false }
