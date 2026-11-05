// Servo-Modquisitor-2/oauth_token_store.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
	keyExpiry       = "expiry"
	probeKey        = "smq-keyring-probe-do-not-use"
)

var (
	keyringChecked   atomic.Bool
	keyringAvailable atomic.Bool
)

// isKeyringAvailable проверяет, доступно ли системное хранилище секретов.
// Если оно недоступно (нет gnome-keyring, KWallet или секретов в системе),
// токены не сохраняются вообще.
func isKeyringAvailable() bool {
	if keyringChecked.Load() {
		return keyringAvailable.Load()
	}
	if err := keyring.Set(keyringService, probeKey, "1"); err == nil {
		_, _ = keyring.Get(keyringService, probeKey)
		_ = keyring.Delete(keyringService, probeKey)
		keyringAvailable.Store(true)
	} else {
		keyringAvailable.Store(false)
	}
	keyringChecked.Store(true)
	return keyringAvailable.Load()
}

func markKeyringUnavailable() {
	keyringAvailable.Store(false)
	keyringChecked.Store(true)
}

// saveTokensToStore сохраняет токены исключительно в системный keyring.
func saveTokensToStore(access, refresh string, expiry time.Time) error {
	if !isKeyringAvailable() {
		return fmt.Errorf("secure keyring is not available in this system. Cannot save tokens securely")
	}

	expiryStr := expiry.Format(time.RFC3339)

	if err := keyring.Set(keyringService, keyAccessToken, access); err != nil {
		markKeyringUnavailable()
		return fmt.Errorf("failed to save access token: %w", err)
	}
	if err := keyring.Set(keyringService, keyRefreshToken, refresh); err != nil {
		return fmt.Errorf("failed to save refresh token: %w", err)
	}
	if err := keyring.Set(keyringService, keyExpiry, expiryStr); err != nil {
		return fmt.Errorf("failed to save token expiry: %w", err)
	}

	return nil
}

// getStoredToken читает токен из системного keyring.
func getStoredToken(key string) (string, error) {
	if !isKeyringAvailable() {
		return "", fmt.Errorf("secure keyring is not available")
	}

	v, err := keyring.Get(keyringService, key)
	if err != nil {
		if err != keyring.ErrNotFound {
			markKeyringUnavailable()
		}
		return "", err
	}
	return v, nil
}

// deleteAllStoredTokens удаляет все токены из keyring.
func deleteAllStoredTokens() {
	if isKeyringAvailable() {
		_ = keyring.Delete(keyringService, keyAccessToken)
		_ = keyring.Delete(keyringService, keyRefreshToken)
		_ = keyring.Delete(keyringService, keyExpiry)
	}
}

// cleanupLegacyTokenFile удаляет файл oauth_tokens.json, если он остался
// от старых версий программы или по ошибке. Токены теперь хранятся только
// в безопасном системном keyring.
func (app *App) cleanupLegacyTokenFile() {
	path := filepath.Join(filepath.Dir(configFilePath()), "oauth_tokens.json")
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err == nil {
		}
	}
}
