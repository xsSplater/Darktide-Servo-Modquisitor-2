// Servo-Modquisitor-2/oauth_token_store.go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/zalando/go-keyring"
)

// Хранилище OAuth-токенов с резервным файлом.
//
// go-keyring на Linux требует запущенного Secret Service (gnome-keyring,
// KWallet или совместимого). На дистрибутивах без него (CachyOS, чистый
// Arch, минимальные WM-сессии) вызовы keyring.Set/Get падают с ошибкой
// "The name org.freedesktop.secrets was not provided by any .service
// files". Раньше это молча терялось: saveOAuthTokens возвращал ошибку,
// она писалась только в лог, а пользователю показывалось «успешно».
// Меню при этом оставалось в состоянии «не залогинен», и обновления
// модов падали с "no URL or API key".
//
// Теперь: если keyring не работает, токены кладутся в файл
// oauth_tokens.json в конфиг-директории (права 0600). keyring остаётся
// основным хранилищем, файл используется как fallback и как дубль на
// случай недоступности keyring при следующем запуске.
const (
	keyAccessToken  = "access_token"
	keyRefreshToken = "refresh_token"
	keyExpiry       = "expiry"

	// probeKey — служебный ключ для проверки работоспособности keyring.
	probeKey = "smq-keyring-probe-do-not-use"
)

type oauthTokenFile struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
}

var (
	keyringChecked   atomic.Bool
	keyringAvailable atomic.Bool
)

func oauthTokenFilePath() string {
	return filepath.Join(filepath.Dir(configFilePath()), "oauth_tokens.json")
}

// isKeyringAvailable делает одну реальную проверку keyring (write+read+
// delete служебного ключа) и кэширует результат. Без этой проверки
// каждый вызов getAuthToken() на недоступном keyring ждал бы таймаута
// DBus — что заметно тормозит UI.
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

// markKeyringUnavailable сбрасывает кэш, если keyring сломался уже после
// успешной проверки (например, пользователь закрыл gnome-keyring).
func markKeyringUnavailable() {
	keyringAvailable.Store(false)
	keyringChecked.Store(true)
}

func readTokenFile() (*oauthTokenFile, error) {
	data, err := os.ReadFile(oauthTokenFilePath())
	if err != nil {
		return nil, err
	}
	var tf oauthTokenFile
	if err := json.Unmarshal(data, &tf); err != nil {
		return nil, err
	}
	return &tf, nil
}

func writeTokenFile(tf *oauthTokenFile) error {
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	// 0600 — читает только владелец. Токены — фактически пароль.
	return os.WriteFile(oauthTokenFilePath(), data, 0600)
}

// saveTokensToStore пишет три ключа. Если keyring доступен — пишем в
// него. В файл пишем всегда: во-первых, как fallback, если keyring
// сломается позже; во-вторых, чтобы после перезапуска при недоступном
// keyring пользователю не пришлось логиниться заново.
func saveTokensToStore(access, refresh string, expiry time.Time) error {
	expiryStr := expiry.Format(time.RFC3339)

	if isKeyringAvailable() {
		if err := keyring.Set(keyringService, keyAccessToken, access); err != nil {
			markKeyringUnavailable()
		} else {
			_ = keyring.Set(keyringService, keyRefreshToken, refresh)
			_ = keyring.Set(keyringService, keyExpiry, expiryStr)
		}
	}

	return writeTokenFile(&oauthTokenFile{
		AccessToken:  access,
		RefreshToken: refresh,
		Expiry:       expiryStr,
	})
}

// getStoredToken возвращает одно из трёх значений. Сначала пробует
// keyring (если он доступен), потом файл.
func getStoredToken(key string) (string, error) {
	if isKeyringAvailable() {
		v, err := keyring.Get(keyringService, key)
		if err == nil {
			return v, nil
		}
		if !errors.Is(err, keyring.ErrNotFound) {
			// keyring вернул не «нет ключа», а что-то более серьёзное —
			// переключаемся на файл до конца сессии.
			markKeyringUnavailable()
		}
	}

	tf, err := readTokenFile()
	if err != nil {
		return "", err
	}
	var v string
	switch key {
	case keyAccessToken:
		v = tf.AccessToken
	case keyRefreshToken:
		v = tf.RefreshToken
	case keyExpiry:
		v = tf.Expiry
	default:
		return "", fmt.Errorf("unknown token key: %s", key)
	}
	if v == "" {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

// deleteAllStoredTokens чистит и keyring, и файл.
func deleteAllStoredTokens() {
	if isKeyringAvailable() {
		_ = keyring.Delete(keyringService, keyAccessToken)
		_ = keyring.Delete(keyringService, keyRefreshToken)
		_ = keyring.Delete(keyringService, keyExpiry)
	}
	_ = os.Remove(oauthTokenFilePath())
}
