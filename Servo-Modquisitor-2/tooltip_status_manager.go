package main

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type TooltipStatusManager struct {
	label     *widget.Label
	mu        sync.Mutex
	hideTimer *time.Timer
}

func NewTooltipStatusManager(label *widget.Label) *TooltipStatusManager {
	return &TooltipStatusManager{label: label}
}

// Show немедленно показывает тултип и останавливает таймер скрытия.
func (tm *TooltipStatusManager) Show(tip string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.hideTimer != nil {
		tm.hideTimer.Stop()
	}
	tm.label.SetText(tip)
	tm.label.Refresh()
}

// HideAfterDelay запускает таймер скрытия через 2 секунды (если не будет нового Show).
func (tm *TooltipStatusManager) HideAfterDelay() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.hideTimer != nil {
		tm.hideTimer.Stop()
	}
	tm.hideTimer = time.AfterFunc(TooltipHideDelay, func() {
		fyne.Do(func() {
			tm.mu.Lock()
			tm.label.SetText("")
			tm.label.Refresh()
			tm.mu.Unlock()
		})
	})
}
