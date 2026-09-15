// Servo-Modquisitor-2/update_test.go
package main

import (
	"runtime"
	"testing"
)

func TestGetProgramArchivePattern(t *testing.T) {
	pattern := getProgramArchivePattern()
	if runtime.GOOS == "windows" {
		if pattern != "Servo Modquisitor 2 Windows" {
			t.Errorf("getProgramArchivePattern() = %s, want 'Servo Modquisitor 2 Windows'", pattern)
		}
	} else if runtime.GOOS == "linux" {
		if pattern != "Servo Modquisitor 2 Linux" {
			t.Errorf("getProgramArchivePattern() = %s, want 'Servo Modquisitor 2 Linux'", pattern)
		}
	} else {
		if pattern != "Servo Modquisitor 2" {
			t.Errorf("getProgramArchivePattern() = %s, want 'Servo Modquisitor 2'", pattern)
		}
	}
}
