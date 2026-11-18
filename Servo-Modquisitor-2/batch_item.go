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
	BatchModeDelete                   // delete
)

type BatchItemStatus int

// ProblemKind — тип проблемы у BatchItem в BatchModeDelete.
// В остальных режимах не используется.
type ProblemKind int

const (
	BatchPending BatchItemStatus = iota
	BatchWaiting
	BatchDownloading
	BatchInstalled
	BatchSkipped
	BatchFailed
)

const (
	ProblemNone ProblemKind = iota
	ProblemObsolete
	ProblemConflict
	ProblemBroken
	ProblemMissingFolder
)

// Icon возвращает имя файла иконки (без .png) в assets/buttons/.
// Используется в batch-диалоге для statusIcon().
func (s BatchItemStatus) Icon() string {
	switch s {
	case BatchPending:
		return "clock_sand"
	case BatchWaiting:
		return "timer"
	case BatchDownloading:
		return "upd_download_blue"
	case BatchInstalled:
		return "checked_box"
	case BatchSkipped:
		return "bottom"
	case BatchFailed:
		return "warn_red"
	}
	return "clock_sand"
}

// String возвращает ключ локализации с коротким названием типа проблемы.
// Используются существующие ключи из messages.json — новые дубликаты
// не заводим.
func (k ProblemKind) String() string {
	switch k {
	case ProblemObsolete:
		return "filter_obsolete" // "Устаревшие" / "Obsolete"
	case ProblemConflict:
		return "sort_conflict" // "Конфликт" / "Conflict"
	case ProblemBroken:
		return "desc_broken" // "нет .mod файла" / "missing .mod file"
	case ProblemMissingFolder:
		return "filter_missing" // "Нет папки мода" / "Missing folder"
	}
	return ""
}

// Icon — имя PNG в assets/buttons/ (та же логика, что BatchItemStatus.Icon).
func (k ProblemKind) Icon() string {
	switch k {
	case ProblemObsolete:
		return "skull"
	case ProblemConflict:
		return "conflict"
	case ProblemBroken:
		return "folder_broken"
	case ProblemMissingFolder:
		return "folder_missing"
	}
	return "warn_red"
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

	// Kind — только для BatchModeDelete. Для остальных режимов —
	// ProblemNone.
	Kind ProblemKind

	// ConflictFolder — первый партнёр (оставлен для совместимости).
	ConflictFolder string

	// ConflictPartners — все папки, с которыми конфликтует этот мод
	// (только «живые» пары). Заполняется для любого Kind: мод может
	// быть одновременно устаревшим и конфликтующим.
	ConflictPartners []string

	// ConflictGroup — номер группы связности конфликтов.
	// Моды одной группы конфликтуют напрямую или через цепочку —
	// оставить нужно один. -1 — не конфликт.
	ConflictGroup int
}

func (b *BatchItem) NexusURL() string {
	if b.NexusModID == 0 {
		return ""
	}
	return fmt.Sprintf(NexusModIDLink, b.NexusModID)
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
