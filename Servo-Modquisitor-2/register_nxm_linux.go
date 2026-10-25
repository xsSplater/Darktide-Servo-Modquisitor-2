//go:build linux

// Servo-Modquisitor-2/register_nxm_linux.go

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// registerNXMProtocol регистрирует приложение в XDG-каталогах:
//
//  1. Кладёт встроенную иконку в ~/.local/share/icons/hicolor/<размер>/apps/.
//  2. Пишет .desktop-файл с именем, равным AppID — это ключ, по
//     которому KDE и GNOME находят ярлык для окна с данным app_id.
//  3. Прописывает поля Icon= и StartupWMClass=, чтобы компоновщик
//     подтянул иконку и в меню приложений, и в панели задач.
//  4. Обновляет MIME-регистрацию для nxm:// — можно кликать «Vortex»
//     на сайте Nexus и ссылка откроет SMQ.
//  5. Убирает устаревший .desktop со старым именем, если он остался
//     от предыдущих версий.
//
// Функция идемпотентна: каждый запуск перезаписывает файлы. Это
// нужно, чтобы после переноса программы в другую папку путь Exec=
// в .desktop обновился сам, без ручной правки.
//
// Вызывается из main.go при каждом старте, ошибки логируются, но не
// блокируют запуск — если XDG-каталог недоступен (live-USB, chroot),
// программа продолжит работать без ярлыка.
func registerNXMProtocol(exePath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return fmt.Errorf("cannot resolve home dir: %w", err)
	}

	// ─── 1. Иконка ─────────────────────────────────────────────────
	//
	// Имя файла должно совпадать с AppID: XDG-инструменты (KDE,
	// GNOME) ищут иконку приложения сначала по $AppID.png, потом
	// по $AppID в Icon=. Если положить рядом icon.png с другим
	// именем, DE его не найдёт и покажет дефолтную иконку Fyne.
	//
	// Размер 256×256 — стандарт hicolor. Если у вас PNG другого
	// размера, всё равно положим сюда: масштабирование сделает DE.
	iconDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor", "256x256", "apps")
	iconPath := filepath.Join(iconDir, AppID+".png")

	iconData, err := embeddedFiles.ReadFile(AppIcon)
	if err == nil && len(iconData) > 0 {
		if err := os.MkdirAll(iconDir, 0755); err != nil {
			return fmt.Errorf("mkdir icons: %w", err)
		}
		if err := os.WriteFile(iconPath, iconData, 0644); err != nil {
			return fmt.Errorf("write icon: %w", err)
		}
	} else {
		// Иконку не нашли в embed — не критично, но иконка не
		// появится. Логируем и идём дальше (поле Icon= будет
		// пустым, DE возьмёт иконку по WM_CLASS из темы).
		iconPath = ""
	}

	// ─── 2. .desktop ───────────────────────────────────────────────
	//
	// Имя файла = AppID. Это единственный надёжный способ, чтобы
	// KDE и GNOME связали ярлык с окном на Wayland: они матчат
	// app_id окна к базовому имени .desktop-файла.
	//
	// StartupWMClass — резервный механизм для X11 и некоторых
	// версий KDE: DE читает это поле и матчит его к WM_CLASS окна.
	// Ставим оба — тогда матчинг сработает в любом случае.
	desktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(desktopDir, 0755); err != nil {
		return fmt.Errorf("mkdir applications: %w", err)
	}

	desktopFileName := AppID + ".desktop"
	desktopPath := filepath.Join(desktopDir, desktopFileName)

	// execPath может содержать пробелы и специальные символы —
	// оборачиваем в двойные кавычки (валидно по спецификации
	// Desktop Entry).
	iconLine := ""
	if iconPath != "" {
		iconLine = "Icon=" + iconPath + "\n"
	}

	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Servo-Modquisitor
GenericName=Darktide Mod Manager
Comment=Manage mods for Warhammer 40,000: Darktide
Exec="%s" --nxm %%u
%sTerminal=false
Categories=Game;Utility;
MimeType=x-scheme-handler/nxm;
StartupNotify=true
StartupWMClass=%s
`, exePath, iconLine, AppID)

	if err := os.WriteFile(desktopPath, []byte(content), 0755); err != nil {
		return fmt.Errorf("write desktop: %w", err)
	}

	// ─── 3. Уборка старого .desktop ────────────────────────────────
	//
	// В предыдущих версиях файл назывался servo-modquisitor.desktop.
	// Если он остался — DE может найти два ярлыка и выбрать
	// «не тот» (без иконки и без StartupWMClass). Удаляем молча:
	// отсутствие файла — не ошибка.
	oldPath := filepath.Join(desktopDir, "servo-modquisitor.desktop")
	if oldPath != desktopPath {
		_ = os.Remove(oldPath)
	}

	// ─── 4. MIME для nxm:// ────────────────────────────────────────
	//
	// xdg-mime пишет в ~/.config/mimeapps.list ассоциацию
	// x-scheme-handler/nxm → наш .desktop. После этого клик
	// «Vortex» на Nexus Mods открывает SMQ, а не спрашивает
	// «чем открыть».
	_ = exec.Command("xdg-mime", "default", desktopFileName, "x-scheme-handler/nxm").Run()

	// ─── 5. Обновление кэша ────────────────────────────────────────
	//
	// KDE кэширует метаданные .desktop. Без update-desktop-database
	// изменение Icon= может не подхватиться до перелогина. Команда
	// необязательна — если её нет в PATH, просто ничего не произойдёт.
	_ = exec.Command("update-desktop-database", desktopDir).Run()

	// Некоторые дистрибутивы (в т.ч. SteamOS) используют этот
	// индекс вместо update-desktop-database.
	_ = exec.Command("gtk-update-icon-cache", "-f", "-t",
		filepath.Join(homeDir, ".local", "share", "icons", "hicolor")).Run()

	return nil
}
