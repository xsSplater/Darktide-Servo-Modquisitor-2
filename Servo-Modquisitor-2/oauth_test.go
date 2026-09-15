// Servo-Modquisitor-2/oauth_test.go

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestRandomString(t *testing.T) {
	s := randomString(16)
	if len(s) != 22 { // base64.RawURLEncoding encodes 16 bytes to 22 chars
		t.Errorf("randomString(16) length = %d, want 22", len(s))
	}
	// Проверяем, что строка содержит только допустимые символы
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			t.Errorf("randomString contains invalid char: %c", r)
		}
	}
}

func TestGenerateCodeVerifier(t *testing.T) {
	v, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("generateCodeVerifier error: %v", err)
	}
	if len(v) != 43 { // 32 bytes -> 43 chars
		t.Errorf("verifier length = %d, want 43", len(v))
	}
}

func TestGenerateCodeChallenge(t *testing.T) {
	verifier := "test_verifier"
	challenge := generateCodeChallenge(verifier)
	// Проверяем, что это Base64URL-encoded SHA256
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	if err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("decoded challenge length = %d, want 32", len(decoded))
	}
	// Проверяем, что хеш соответствует
	hash := sha256.Sum256([]byte(verifier))
	if string(decoded) != string(hash[:]) {
		t.Error("challenge does not match SHA256 of verifier")
	}
}
