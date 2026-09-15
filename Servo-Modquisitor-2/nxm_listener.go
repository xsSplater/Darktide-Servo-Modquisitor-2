// Servo-Modquisitor-2/nxm_listener.go
package main

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

// NXMListener — управляет TCP-слушателем для приёма nxm://-ссылок от
// браузера и защищает от повторной обработки одной и той же ссылки
// в течение короткого окна.
//
// Заменяет три поля App (nxmListener, lastNxmURL, lastNxmTime).
type NXMListener struct {
	mu        sync.Mutex
	listener  net.Listener
	lastURL   string
	lastTime  time.Time

	// onLink вызывается в UI-потоке при получении новой ссылки.
	// Может быть nil.
	onLink func(string)
}

// NewNXMListener создаёт неактивный слушатель. onLink может быть nil.
func NewNXMListener(onLink func(string)) *NXMListener {
	return &NXMListener{onLink: onLink}
}

// Start пытается поднять TCP-слушатель. Если порт уже занят —
// возвращает ошибку, ничего не меняя.
//
// Если слушатель уже запущен — no-op.
func (l *NXMListener) Start() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener != nil {
		return nil
	}
	ln, err := net.Listen(NXMProtocol, NXMAddress)
	if err != nil {
		return err
	}
	l.listener = ln
	go l.acceptLoop(ln)
	return nil
}

// Stop закрывает слушатель (если он открыт). Безопасно вызывать
// повторно и при незапущенном слушателе.
func (l *NXMListener) Stop() {
	l.mu.Lock()
	ln := l.listener
	l.listener = nil
	l.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
}

// IsDuplicate сообщает, была ли эта же ссылка обработана недавно
// (в пределах 5 секунд). Обновляет состояние при не-дубликате.
func (l *NXMListener) IsDuplicate(url string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if url == l.lastURL && now.Sub(l.lastTime) < 5*time.Second {
		return true
	}
	l.lastURL = url
	l.lastTime = now
	return false
}

// acceptLoop принимает входящие соединения и передаёт ссылки в onLink
// через fyne.Do.
func (l *NXMListener) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // слушатель закрыт
		}
		link, _ := bufio.NewReader(conn).ReadString('\n')
		conn.Close()
		if l.onLink != nil {
			fyne.Do(func() { l.onLink(strings.TrimSpace(link)) })
		}
	}
}
