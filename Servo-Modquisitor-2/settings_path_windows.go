//go:build windows

// Servo-Modquisitor-2/settings_path_windows.go

package main

// findProtonSettingsPath — no-op на Windows. Настоящий поиск
// user_settings.config идёт через %APPDATA% прямо в getUserSettingsPath.
func findProtonSettingsPath() string {
	return ""
}
