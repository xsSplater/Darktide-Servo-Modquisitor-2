// Servo-Modquisitor-2/oauth_state.go
package main

import "sync"

// OAuthState — потокобезопасное хранилище state/verifier для OAuth-flow.
// Заменяет два поля App (oauthState, oauthVerifier) с мьютексом.
//
// Эти значения заполняются в startOAuthFlow и читаются в HTTP-колбэке
// на /callback. Обе точки в разных горутинах, поэтому мьютекс нужен.
type OAuthState struct {
	mu       sync.Mutex
	state    string
	verifier string
}

// NewOAuthState создаёт пустое хранилище.
func NewOAuthState() *OAuthState {
	return &OAuthState{}
}

// Set сохраняет пару (state, verifier).
func (s *OAuthState) Set(state, verifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.verifier = verifier
}

// Get возвращает сохранённую пару (state, verifier).
func (s *OAuthState) Get() (state, verifier string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.verifier
}
