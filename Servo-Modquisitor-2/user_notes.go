// Servo-Modquisitor-2/user_notes.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// userNotesStore — хранилище пользовательских заметок к модам.
// Ключ — имя папки мода, значение — произвольный текст.
//
// Живёт рядом с config.json.
type userNotesStore struct {
	mu    sync.RWMutex
	notes map[string]string
}

func newUserNotesStore() *userNotesStore {
	return &userNotesStore{notes: make(map[string]string)}
}

func (s *userNotesStore) Get(folder string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.notes[folder]
}

func (s *userNotesStore) Set(folder, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if text == "" {
		delete(s.notes, folder)
	} else {
		s.notes[folder] = text
	}
}

func (s *userNotesStore) Snapshot() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.notes))
	for k, v := range s.notes {
		out[k] = v
	}
	return out
}

func userNotesFilePath() string {
	return filepath.Join(filepath.Dir(configFilePath()), "user_notes.json")
}

func (s *userNotesStore) Load() error {
	data, err := os.ReadFile(userNotesFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil // первый запуск — это норма
		}
		return err
	}
	var notes map[string]string
	if err := json.Unmarshal(data, &notes); err != nil {
		return fmt.Errorf("parse user_notes.json: %w", err)
	}
	s.mu.Lock()
	s.notes = notes
	s.mu.Unlock()
	return nil
}

func (s *userNotesStore) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.notes, "", "\t")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	return os.WriteFile(userNotesFilePath(), data, 0644)
}
