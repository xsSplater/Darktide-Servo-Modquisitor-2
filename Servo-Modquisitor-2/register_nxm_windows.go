//go:build windows

// register_nxm_windows.go
package main

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
)

func registerNXMProtocol(exePath string) error {
	key := `Software\Classes\nxm`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	k.SetStringValue("", "URL:Nexus Mod Manager Protocol")
	k.SetStringValue("URL Protocol", "")

	iconKey, _, err := registry.CreateKey(registry.CURRENT_USER, key+`\DefaultIcon`, registry.SET_VALUE)
	if err == nil {
		iconKey.SetStringValue("", exePath+",0")
		iconKey.Close()
	}

	cmdKey, _, err := registry.CreateKey(registry.CURRENT_USER, key+`\shell\open\command`, registry.SET_VALUE)
	if err == nil {
		cmdKey.SetStringValue("", fmt.Sprintf(`"%s" --nxm "%%1"`, exePath))
		cmdKey.Close()
	}
	return nil
}
