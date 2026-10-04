// Servo-Modquisitor-2/quick_launch.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// runQuickLaunch — путь для ярлыка «быстрый запуск игры».
//
// Никакого GUI: читаем config.json, разрешаем пути, при необходимости
// молча поднимаем Steam, запускаем игру через launchGame(skipLauncher=true),
// выходим.
//
// Логи пишем и в stderr, и в app.log — на Windows stderr обычно не виден
// из ярлыка, но лог рядом с exe помогает при разборе проблем.
//
// Коды выхода:
//
//	0 — успех (игра запущена, либо уже была запущена)
//	1 — внутренняя ошибка (не создали App, не прочитали переводы)
//	2 — не настроен game_root / неизвестная версия игры
//	3 — Steam не поднялся за отведённое время
//	4 — launchGame вернул ошибку
func runQuickLaunch() {
	var logFile *os.File
	if exePath, err := os.Executable(); err == nil {
		logPath := filepath.Join(filepath.Dir(exePath), FileNameLog)
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			logFile = f
			defer logFile.Close()
		}
	}

	logMsg := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintln(os.Stderr, msg)
		if logFile != nil {
			fmt.Fprintln(logFile, time.Now().Format(LogTimeFormat), "quick-launch: "+msg)
		}
	}

	cfg := loadConfig()

	// Минимальный App — только чтобы дотянуться до переводов
	// для сообщений launcher_*. Никакие UI-методы не вызываются.
	app := &App{cfg: cfg}
	app.setMessages(map[string]string{})
	if err := app.loadLanguage(cfg.Language); err != nil {
		logMsg("loadLanguage failed: %v", err)
	}

	SetLauncherMessages(
		app.msg("launcher_ver_unknown"),
		app.msg("launcher_exe_not_found"),
		app.msg("launcher_root_not_found"),
	)
	SetLinuxLauncherMessages(
		app.msg("linux_wine_not_found"),
		app.msg("linux_xbox_not_supported"),
	)

	root := cfg.GameRoot
	if root == "" {
		logMsg("game_root not set in config")
		os.Exit(2)
	}

	ver := detectGameVersion(root)
	if ver == VersionUnknown {
		logMsg("game version unknown at %q", root)
		os.Exit(2)
	}

	if isDarktideRunning() {
		logMsg("Darktide already running, nothing to do")
		os.Exit(0)
	}

	// Steam: молча поднимаем, если выключен. Диалогов быть не может —
	// это ярлык, интерактива нет. Дополнительная пауза после появления
	// процесса: сам клиент стартует быстро, но на логин / показ
	// селектора аккаунтов уходит ещё несколько секунд.
	if ver == VersionSteam && !isSteamRunning() {
		if err := startSteam(); err != nil {
			logMsg("startSteam failed: %v", err)
			os.Exit(3)
		}
		if !waitFor(isSteamRunning, 30*time.Second) {
			logMsg("Steam did not start in time")
			os.Exit(3)
		}
		time.Sleep(3 * time.Second)
	}

	if err := launchGame(ver, root, true); err != nil {
		logMsg("launchGame failed: %v", err)
		os.Exit(4)
	}

	logMsg("game launched")
}
