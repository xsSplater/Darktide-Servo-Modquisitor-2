// Servo-Modquisitor-2/bundle_patch.go
//
// Патчер bundle_database.data — собственная реализация логики
// dtkit-patch (manshanko, MIT) и _dt_mod_autopatch.dll (DMA, MIT),
// адаптированная под Go и работающая без внешних бинарников.
//
// Идея:
//  1. В bundle/bundle_database.data есть дескриптор boot-bundle'а
//     9ba626afa44a3aa3, начинающийся с 8-байтовой сигнатуры
//     0xA33A4AA4AF26A69B (big-endian).
//  2. Мы заменяем ровно 84 байта после сигнатуры на содержимое
//     assets/patch.bin (184 байта) — новый дескриптор, в котором
//     есть дополнительный Lua-чанк .patch_999.
//  3. Игра при загрузке boot-bundle'а вызывает этот Lua-чанк,
//     он подгружает binaries/mod_loader → читает mods/ → работаем.
//  4. Обратная операция — mv .bak → .data.
//
// Совместимость с DMA: patch.bin начинается с той же сигнатуры и
// содержит тег .patch_999, поэтому _dt_mod_autopatch.dll при запуске
// игры видит «AlreadyPatched» и не трогает файл. Конфликта нет.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	bundleDatabaseName = "bundle_database.data"
	bundleDatabaseBak  = "bundle_database.data.bak"

	// 8-байтовый якорь. Big-endian представление 0xA33A4AA4AF26A69B.
	// В исходниках dtkit-patch/DMA это u64::to_be_bytes(0xA33A4AA4AF26A69B),
	// что даёт ровно байты a3 3a 4a a4 af 26 a6 9b — они же стоят в начале
	// assets/patch.bin.
	modPatchStartingPoint = "\xa3\x3a\x4a\xa4\xaf\x26\xa6\x9b"

	// Тег «уже пропатчено нами». Есть только в новом дескрипторе.
	modPatchTag = ".patch_999"

	// Тег чужого (устаревшего) патча. Если встретим — не патчим.
	bootBundleNextPatch = "9ba626afa44a3aa3.patch_001"

	// Сколько байт заменяется в оригинальном database.
	// Проверено на patch.bin: ровно 84.
	bundleOldSize = 84
)

var (
	ErrBundleAlreadyPatched = errors.New("bundle is already patched")
	ErrBundleUnsupported    = errors.New("bundle contains unsupported (foreign) patch")
	ErrBundleForeignPatch   = errors.New("bundle is patched by another tool")
	ErrBundlePatchOffset    = errors.New("patch offset not found (game updated?)")
	ErrBundleBackupMissing  = errors.New("bundle_database.data.bak not found")
)

const (
	// Имя DLL-патчера (DMA, manshanko). Устанавливается как
	// binaries/plugins/_dt_mod_autopatch.dll.
	autopatcherDLLName = "_dt_mod_autopatch.dll"

	// Файл-флаг, который читает DLL при запуске игры. Если существует —
	// DLL не патчит bundle. Путь относительно корня игры: mods/DISABLE_AUTOPATCHER.
	autopatcherDisableFlag = "DISABLE_AUTOPATCHER"
)

func bundleDir(gameRoot string) string {
	return filepath.Join(gameRoot, "bundle")
}

// ─────────────────────────────────────────────────────────────────
// Autopatcher (DLL) — установка, флаг и совместное переключение
// ─────────────────────────────────────────────────────────────────

// autopatcherDLLPath возвращает путь к _dt_mod_autopatch.dll.
func autopatcherDLLPath(gameRoot string) string {
	return filepath.Join(gameRoot, "binaries", "plugins", autopatcherDLLName)
}

// autopatcherDisableFlagPath возвращает путь к файлу-флагу
// mods/DISABLE_AUTOPATCHER.
func autopatcherDisableFlagPath(gameRoot string) string {
	return filepath.Join(gameRoot, "mods", autopatcherDisableFlag)
}

// AutopatcherDLLInstalled сообщает, установлена ли _dt_mod_autopatch.dll.
// Наличие DLL означает, что игра будет автоматически пропатчена при
// следующем запуске (если флаг DISABLE_AUTOPATCHER отсутствует).
func AutopatcherDLLInstalled(gameRoot string) bool {
	if gameRoot == "" {
		return false
	}
	info, err := os.Stat(autopatcherDLLPath(gameRoot))
	return err == nil && !info.IsDir()
}

// AutopatcherDisabled сообщает, стоит ли флаг DISABLE_AUTOPATCHER.
// При наличии DLL именно это — источник истины «моды выключены»,
// а не тег в bundle_database.data.
func AutopatcherDisabled(gameRoot string) bool {
	if gameRoot == "" {
		return false
	}
	_, err := os.Stat(autopatcherDisableFlagPath(gameRoot))
	return err == nil
}

// SetAutopatcherDisabled создаёт или удаляет файл-флаг
// mods/DISABLE_AUTOPATCHER. Содержимое файла неважно для DLL,
// но пишем ту же фразу, что и официальный toggle-скрипт.
func SetAutopatcherDisabled(gameRoot string, disabled bool) error {
	if gameRoot == "" {
		return fmt.Errorf("game root is empty")
	}
	path := autopatcherDisableFlagPath(gameRoot)

	if disabled {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path,
			[]byte("Darktide mod autopatching is disabled while this file exists\n"),
			0644)
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// IsModsEnabled — универсальный источник истины для UI. Учитывает,
// установлена ли DLL.
//
//   - DLL установлена  → enabled == !AutopatcherDisabled(gameRoot)
//   - DLL не установлена → enabled == BundleIsPatched(gameRoot)
//
// Это единственное место, где UI должен спрашивать «включены ли моды»,
// чтобы не разъезжаться с тем, что реально произойдёт при запуске игры.
func IsModsEnabled(gameRoot string) bool {
	if gameRoot == "" {
		return false
	}
	if AutopatcherDLLInstalled(gameRoot) {
		return !AutopatcherDisabled(gameRoot)
	}
	patched, err := BundleIsPatched(gameRoot)
	return err == nil && patched
}

// ToggleBundleWithAutopatcher — расширенная версия ToggleBundle, которая
// учитывает установленную DLL.
//
// Если DLL НЕ установлена — делегирует в ToggleBundle (прежнее поведение).
//
// Если DLL установлена:
//  1. Управляем через файл-флаг DISABLE_AUTOPATCHER (DLL читает именно его).
//  2. Дополнительно приводим bundle в согласованное состояние, чтобы
//     игра запускалась корректно СРАЗУ, не дожидаясь, пока DLL отработает
//     при следующем старте:
//     • при включении — если bundle не пропатчен, патчим;
//     • при выключении — если bundle пропатчен, раскатываем .bak.
//
// Возвращает новое состояние (true = моды включены).
func ToggleBundleWithAutopatcher(gameRoot string) (bool, error) {
	if gameRoot == "" {
		return false, fmt.Errorf("game root is empty")
	}

	// DLL нет — обычная логика через bundle.
	if !AutopatcherDLLInstalled(gameRoot) {
		return ToggleBundle(gameRoot)
	}

	// DLL есть — источник истины = флаг.
	wasDisabled := AutopatcherDisabled(gameRoot)
	willBeEnabled := wasDisabled // переключаем

	if willBeEnabled {
		// ВКЛЮЧАЕМ: снимаем флаг, потом (если нужно) патчим bundle.
		// Если не удастся снять флаг — не трогаем bundle вообще, чтобы
		// состояние не разъехалось.
		if err := SetAutopatcherDisabled(gameRoot, false); err != nil {
			return false, err
		}
		patched, _ := BundleIsPatched(gameRoot)
		if !patched {
			// Ошибка AlreadyPatched здесь невозможна (мы только что
			// проверили), но подстрахуемся.
			if err := PatchBundle(gameRoot); err != nil && !errors.Is(err, ErrBundleAlreadyPatched) {
				// Флаг уже снят, но bundle не пропатчен. DLL подхватит
				// при следующем запуске игры. Не откатываем — пользователь
				// выразил намерение «включить моды».
				return true, err
			}
		}
		return true, nil
	}

	// ВЫКЛЮЧАЕМ: ставим флаг, потом (если нужно) восстанавливаем bundle.
	if err := SetAutopatcherDisabled(gameRoot, true); err != nil {
		return true, err
	}
	patched, _ := BundleIsPatched(gameRoot)
	if patched {
		if err := UnpatchBundle(gameRoot); err != nil {
			// Чужой патч / отсутствует .bak — не критично: флаг стоит,
			// при следующем запуске DLL не будет патчить, а bundle
			// останется как есть до ручного разбора. Ошибку отдаём
			// наверх, но состояние считаем «выключено».
			if errors.Is(err, ErrBundleForeignPatch) ||
				errors.Is(err, ErrBundleBackupMissing) {
				return false, err
			}
			return false, err
		}
	}
	return false, nil
}

// Известные теги патчей от разных инструментов.
// Проверяем все — нам важно знать, пропатчен ли bundle ВООБЩЕ,
// а не только нашим патчем.
var knownPatchTags = [][]byte{
	[]byte(".patch_999"),                 // SMQ / dtkit-patch
	[]byte("9ba626afa44a3aa3.patch_001"), // DMA / _dt_mod_autopatch.dll
	[]byte(".patch_"),                    // любой другой .patch_XXX
}

// BundleIsPatched сообщает, пропатчен ли bundle_database.data
// ЛЮБЫМ известным инструментом (SMQ, DMA, dtkit-patch, DML).
//
// Раньше проверялся только наш собственный тег .patch_999, из-за
// чего игра, пропатченная DMA, показывалась как «моды выключены».
// Теперь мы честно говорим «да, патч есть» — и UI покажет, что
// моды активны, независимо от того, кто патчил.
func BundleIsPatched(gameRoot string) (bool, error) {
	db, err := os.ReadFile(filepath.Join(bundleDir(gameRoot), bundleDatabaseName))
	if err != nil {
		return false, err
	}
	for _, tag := range knownPatchTags {
		if bytes.Contains(db, tag) {
			return true, nil
		}
	}
	return false, nil
}

// BundlePatchedBy возвращает человекочитаемое имя инструмента, который
// пропатчил bundle, или "" если патча нет. Нужно для отображения в UI
// и для логирования.
func BundlePatchedBy(gameRoot string) string {
	db, err := os.ReadFile(filepath.Join(bundleDir(gameRoot), bundleDatabaseName))
	if err != nil {
		return ""
	}
	switch {
	case bytes.Contains(db, []byte(modPatchTag)):
		return "SMQ"
	case bytes.Contains(db, []byte(bootBundleNextPatch)):
		return "Autopatcher"
	default:
		// Любой другой .patch_XXX — неизвестный источник.
		if bytes.Contains(db, []byte(".patch_")) {
			return "unknown"
		}
		return ""
	}
}

// BundleHasBackup сообщает, лежит ли рядом .bak.
func BundleHasBackup(gameRoot string) bool {
	_, err := os.Stat(filepath.Join(bundleDir(gameRoot), bundleDatabaseBak))
	return err == nil
}

// PatchBundle применяет assets/patch.bin к bundle_database.data.
//
// Возвращает ErrBundleAlreadyPatched, если bundle уже пропатчен ЛЮБЫМ
// инструментом — не только нашим. Это не ошибка: вызывающий код должен
// трактовать это как «моды уже включены» и ничего не делать.
//
// Раньше PatchBundle возвращал ErrBundleUnsupported на чужой патч (DMA),
// и пользователь видел ошибку. Теперь мы просто говорим «уже пропатчено»:
// неважно, кто пропатчил, — важно, что патч есть и работает.
func PatchBundle(gameRoot string) error {
	dir := bundleDir(gameRoot)
	dbPath := filepath.Join(dir, bundleDatabaseName)
	bakPath := filepath.Join(dir, bundleDatabaseBak)

	db, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", dbPath, err)
	}

	// Уже пропатчено — любым инструментом. Не трогаем.
	if patched, _ := BundleIsPatched(gameRoot); patched {
		return ErrBundleAlreadyPatched
	}

	offset := bytes.Index(db, []byte(modPatchStartingPoint))
	if offset < 0 || offset+bundleOldSize > len(db) {
		return ErrBundlePatchOffset
	}

	// Бэкап делаем только если его ещё нет — возможно, файл .bak
	// оставлен от чужого патча, и мы не хотим затирать его.
	if _, err := os.Stat(bakPath); os.IsNotExist(err) {
		if err := os.WriteFile(bakPath, db, 0644); err != nil {
			return fmt.Errorf("write backup: %w", err)
		}
	}

	patched := make([]byte, 0, len(db)-bundleOldSize+len(bundlePatchBin))
	patched = append(patched, db[:offset]...)
	patched = append(patched, bundlePatchBin...)
	patched = append(patched, db[offset+bundleOldSize:]...)

	if err := os.WriteFile(dbPath, patched, 0644); err != nil {
		return fmt.Errorf("write patched %s: %w", dbPath, err)
	}
	return nil
}

// UnpatchBundle восстанавливает bundle_database.data из .bak.
//
// ВАЖНО: работает только если патч НАШ (SMQ). Если bundle пропатчен
// чужим инструментом (DMA, dtkit-patch), мы не можем его распатчить:
//   - наш .bak может не существовать;
//   - чужой патчер может завязан на свой формат;
//   - попытка восстановления может сломать игру.
//
// В этом случае возвращаем ErrBundleForeignPatch, и UI покажет
// пользователю, что нужно откатывать через тот инструмент, который
// патчил.
func UnpatchBundle(gameRoot string) error {
	dir := bundleDir(gameRoot)
	dbPath := filepath.Join(dir, bundleDatabaseName)
	bakPath := filepath.Join(dir, bundleDatabaseBak)

	db, err := os.ReadFile(dbPath)
	if err != nil {
		return err
	}

	// Чужой патч — не трогаем.
	if bytes.Contains(db, []byte(bootBundleNextPatch)) {
		return ErrBundleForeignPatch
	}
	// Не наш патч, но и не чужой известный — вообще не понимаем,
	// что это. Тоже не трогаем.
	if !bytes.Contains(db, []byte(modPatchTag)) {
		// Возможно, уже распатчено.
		if _, err := os.Stat(bakPath); os.IsNotExist(err) {
			return nil
		}
		// Есть .bak, но патча нет — состояние уже не наше.
		return ErrBundleForeignPatch
	}

	// Наш патч: восстанавливаем.
	if _, err := os.Stat(bakPath); err != nil {
		if os.IsNotExist(err) {
			return ErrBundleBackupMissing
		}
		return err
	}
	return os.Rename(bakPath, dbPath)
}

// ToggleBundle — кнопка «включить/выключить моды».
// Возвращает новое состояние (true = пропатчено).
func ToggleBundle(gameRoot string) (bool, error) {
	patched, err := BundleIsPatched(gameRoot)
	if err != nil {
		return false, err
	}
	if patched {
		return false, UnpatchBundle(gameRoot)
	}
	return true, PatchBundle(gameRoot)
}
