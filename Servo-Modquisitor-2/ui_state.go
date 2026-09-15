// Servo-Modquisitor-2/ui_state.go
package main

import (
	"sync/atomic"
	"time"
)

// UIState — контейнер для изменяемого UI-состояния (не виджетов!).
// Виджеты остаются в App: они привязаны к главному потоку Fyne, а эти
// поля могут читаться и из фоновых горутин.
//
// Встраивается в App: поля доступны как app.selectedModIndex,
// app.selectedModName, app.orderDirty и т.д. Никакие call-sites менять
// не требуется.
type UIState struct {
	selectedModIndex        atomic.Int32
	selectedModName         string
	showSelectColumn        bool
	orderDirty              bool
	blinkSaveOrderActive    bool
	suppressSelectionEvents bool
	pathsInitialized        bool
	amlDetected             atomic.Bool
	enrichDebounce          *time.Timer
}
