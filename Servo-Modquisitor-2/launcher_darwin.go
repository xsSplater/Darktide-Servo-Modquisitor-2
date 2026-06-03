//go:build darwin

// Заглушка для VSCode
package main

func launchGame(ver GameVersion, gameRoot string, skipLauncher bool) error {
	return nil
}

func registerNXMProtocol(exePath string) error {
	return nil
}
