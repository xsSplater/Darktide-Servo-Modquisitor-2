// Servo-Modquisitor-2/oauth_token_store.go
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keyAccessToken   = "access_token"
	keyRefreshToken  = "refresh_token"
	keyExpiry        = "expiry"
	probeKey         = "smq-keyring-probe-do-not-use"
	fallbackFileName = ".smq-tokens.enc"
)

var (
	keyringChecked   atomic.Bool
	keyringAvailable atomic.Bool

	// Резервное хранилище в памяти (если ни keyring, ни файл не сработали)
	memoryStoreMu sync.Mutex
	memoryStore   = make(map[string]string)
)

// isKeyringAvailable проверяет, доступно ли системное хранилище.
func isKeyringAvailable() bool {
	if keyringAvailable.Load() {
		return true
	}
	if err := keyring.Set(keyringService, probeKey, "1"); err == nil {
		_, _ = keyring.Get(keyringService, probeKey)
		_ = keyring.Delete(keyringService, probeKey)
		keyringAvailable.Store(true)
		return true
	}
	return false
}

func markKeyringUnavailable() {
	keyringAvailable.Store(false)
}

// getEncryptionKey генерирует или читает ключ шифрования для fallback-файла.
func getEncryptionKey() ([]byte, error) {
	keyPath := filepath.Join(filepath.Dir(configFilePath()), ".smq.key")
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		// Генерируем новый ключ
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := os.WriteFile(keyPath, key, 0600); err != nil {
			return nil, err
		}
		return key, nil
	}
	return os.ReadFile(keyPath)
}

// saveTokensToStore — умное сохранение.
func saveTokensToStore(access, refresh string, expiry time.Time) error {
	// 1. ВСЕГДА сохраняем в память. Это позволит пользоваться программой прямо сейчас.
	memoryStoreMu.Lock()
	memoryStore[keyAccessToken] = access
	memoryStore[keyRefreshToken] = refresh
	memoryStore[keyExpiry] = expiry.Format(time.RFC3339)
	memoryStoreMu.Unlock()

	// 2. Пробуем системный Keyring.
	if isKeyringAvailable() {
		errA := keyring.Set(keyringService, keyAccessToken, access)
		if errA == nil {
			errR := keyring.Set(keyringService, keyRefreshToken, refresh)
			errE := keyring.Set(keyringService, keyExpiry, expiry.Format(time.RFC3339))
			if errR == nil && errE == nil {
				return nil
			}
			// Частичный сбой — стираем всё, что успели, чтобы не
			// остаться в полу-состоянии. Fallback-файл ниже запишет
			// полную тройку. Логировать некуда (нет App-контекста
			// в этой функции); главное, что fallback сработает.
			_ = keyring.Delete(keyringService, keyAccessToken)
			_ = keyring.Delete(keyringService, keyRefreshToken)
			_ = keyring.Delete(keyringService, keyExpiry)
		} else {
			markKeyringUnavailable()
		}
	}

	// 3. Если Keyring недоступен — пишем в зашифрованный файл.
	key, err := getEncryptionKey()
	if err != nil {
		return fmt.Errorf("cannot generate key: %w", err)
	}

	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)

	payload := map[string]string{
		"access":  access,
		"refresh": refresh,
		"expiry":  expiry.Format(time.RFC3339),
	}
	raw, _ := json.Marshal(payload)
	encrypted := gcm.Seal(nonce, nonce, raw, nil)

	filePath := filepath.Join(filepath.Dir(configFilePath()), fallbackFileName)
	if err := os.WriteFile(filePath, encrypted, 0600); err != nil {
		return fmt.Errorf("cannot write fallback file: %w", err)
	}

	return nil
}

// getStoredToken читает токен (сначала из памяти, потом из Keyring, потом из файла).
func getStoredToken(key string) (string, error) {
	// 1. Сначала память (самое быстрое)
	memoryStoreMu.Lock()
	if val, ok := memoryStore[key]; ok {
		memoryStoreMu.Unlock()
		return val, nil
	}
	memoryStoreMu.Unlock()

	// 2. Потом Keyring
	if isKeyringAvailable() {
		if val, err := keyring.Get(keyringService, key); err == nil {
			return val, nil
		}
	}

	// 3. Потом зашифрованный файл
	filePath := filepath.Join(filepath.Dir(configFilePath()), fallbackFileName)
	data, err := os.ReadFile(filePath)
	if err == nil {
		keyBytes, err := getEncryptionKey()
		if err == nil {
			block, _ := aes.NewCipher(keyBytes)
			gcm, _ := cipher.NewGCM(block)
			nonceSize := gcm.NonceSize()
			if len(data) > nonceSize {
				raw, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
				if err == nil {
					var payload map[string]string
					json.Unmarshal(raw, &payload)
					switch key {
					case keyAccessToken:
						return payload["access"], nil
					case keyRefreshToken:
						return payload["refresh"], nil
					case keyExpiry:
						return payload["expiry"], nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("token not found")
}

// deleteAllStoredTokens удаляет токены отовсюду.
func deleteAllStoredTokens() {
	memoryStoreMu.Lock()
	memoryStore = make(map[string]string)
	memoryStoreMu.Unlock()

	if isKeyringAvailable() {
		_ = keyring.Delete(keyringService, keyAccessToken)
		_ = keyring.Delete(keyringService, keyRefreshToken)
		_ = keyring.Delete(keyringService, keyExpiry)
	}

	_ = os.Remove(filepath.Join(filepath.Dir(configFilePath()), fallbackFileName))
}

// cleanupLegacyTokenFile удаляет старый файл токенов.
func (app *App) cleanupLegacyTokenFile() {
	path := filepath.Join(filepath.Dir(configFilePath()), "oauth_tokens.json")
	_ = os.Remove(path)
}
