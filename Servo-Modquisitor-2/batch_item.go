// Servo-Modquisitor-2/batch_item.go
package main

import (
	"fmt"
	"strings"
)

type BatchMode int

const (
	BatchModeInstall BatchMode = iota // wizard / collection
	BatchModeUpdate                   // update
)

type BatchItemStatus int

const (
	BatchPending BatchItemStatus = iota
	BatchWaiting
	BatchDownloading
	BatchInstalled
	BatchSkipped
	BatchFailed
)

func (s BatchItemStatus) Icon() string {
	switch s {
	case BatchPending:
		return "⚪"
	case BatchWaiting:
		return "⏳"
	case BatchDownloading:
		return "🔄"
	case BatchInstalled:
		return "✅"
	case BatchSkipped:
		return "⏭"
	case BatchFailed:
		return "❌"
	}
	return "?"
}

type BatchItem struct {
	// Идентификация
	ID         string // folder name для wizard/update; modID для collection
	NexusModID int
	FileID     int

	// Отображение (обогащается из mod_database.json)
	Name        string
	Author      string
	Category    string
	Description string
	Version     string
	Changelog   string
	PictureURL  string

	// Состояние
	Status   BatchItemStatus
	Error    string
	Selected bool
}

func (b *BatchItem) NexusURL() string {
	if b.NexusModID == 0 {
		return ""
	}
	return fmt.Sprintf("https://www.nexusmods.com/warhammer40kdarktide/mods/%d", b.NexusModID)
}

func (b *BatchItem) VersionLabel() string {
	if b.Version != "" {
		return b.Version
	}
	return "unknown"
}

func (b *BatchItem) ShortDesc() string {
	s := strings.TrimSpace(b.Description)
	if len(s) > 100 {
		return s[:97] + "..."
	}
	return s
}

// BatchSignal — сигнал о завершённой установке.
type BatchSignal struct {
	Folder     string
	NexusModID int
}
