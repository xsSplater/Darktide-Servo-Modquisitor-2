package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func (app *App) buildMainMenu() *fyne.MainMenu {
	langEng := fyne.NewMenuItem(app.messages["menu_lang_en"], func() { app.changeLanguage("en") })
	langRu := fyne.NewMenuItem(app.messages["menu_lang_ru"], func() { app.changeLanguage("ru") })
	langMenu := fyne.NewMenuItem(app.messages["menu_language"], nil)
	langMenu.ChildMenu = fyne.NewMenu("", langEng, langRu)

	themeDark := fyne.NewMenuItem(app.messages["menu_theme_dark"], func() {
		app.cfg.Theme = "dark"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
	})
	themeLight := fyne.NewMenuItem(app.messages["menu_theme_light"], func() {
		app.cfg.Theme = "light"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
	})
	themeMenu := fyne.NewMenuItem(app.messages["menu_theme"], nil)
	themeMenu.ChildMenu = fyne.NewMenu("", themeDark, themeLight)

	dateYYYY := fyne.NewMenuItem(app.messages["menu_date_format_yyyy_mm_dd"], func() {
		app.cfg.DateFormat = "yyyy-mm-dd"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateMMDD := fyne.NewMenuItem(app.messages["menu_date_format_mm_dd_yyyy"], func() {
		app.cfg.DateFormat = "mm-dd-yyyy"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateDDMM := fyne.NewMenuItem(app.messages["menu_date_format_dd_mm_yyyy"], func() {
		app.cfg.DateFormat = "dd-mm-yyyy"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateMenu := fyne.NewMenuItem(app.messages["menu_date_format"], nil)
	dateMenu.ChildMenu = fyne.NewMenu("", dateYYYY, dateMMDD, dateDDMM)

	forceEnglishLabel := app.messages["setting_force_english_mod_names"]
	if app.cfg.ForceEnglishModNames {
		forceEnglishLabel = "✅ " + forceEnglishLabel
	}
	forceEnglishItem := fyne.NewMenuItem(forceEnglishLabel, func() {
		app.cfg.ForceEnglishModNames = !app.cfg.ForceEnglishModNames
		saveConfig(app.cfg)
		app.refreshModList()
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	updateItem := fyne.NewMenuItem(app.messages["check_updates"], func() {
		go app.checkForUpdates()
	})

	settingsMenu := fyne.NewMenu(app.messages["menu_settings"],
		langMenu, themeMenu, dateMenu, forceEnglishItem,
		fyne.NewMenuItemSeparator(),
		updateItem,
	)

	version := detectGameVersion(app.gameRoot)
	launchItems := []*fyne.MenuItem{}
	if version == VersionSteam || version == VersionXbox {
		launchItems = append(launchItems,
			fyne.NewMenuItem(app.messages["menu_launch_normal"], func() {
				go func() {
					err := app.launchGameFunc(version, app.gameRoot, false)
					if err != nil {
						app.appendLog(fmt.Sprintf(app.messages["launch_error"], err))
					}
				}()
			}),
			fyne.NewMenuItem(app.messages["menu_launch_nolauncher"], func() {
				go func() {
					err := app.launchGameFunc(version, app.gameRoot, true)
					if err != nil {
						app.appendLog(fmt.Sprintf(app.messages["launch_error"], err))
					}
				}()
			}),
		)
	}
	launchMenu := fyne.NewMenu(app.messages["menu_launch_game"], launchItems...)

	contactNexus := fyne.NewMenuItem(app.messages["menu_nexus"], func() {
		u, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/139")
		_ = app.myApp.OpenURL(u)
	})
	contactDiscordMy := fyne.NewMenuItem(app.messages["menu_discord_my"], func() {
		u, _ := url.Parse("https://discord.gg/BGZagw3xnz")
		_ = app.myApp.OpenURL(u)
	})
	contactDiscord := fyne.NewMenuItem(app.messages["menu_discord_dtmoddrs"], func() {
		u, _ := url.Parse("https://discord.com/channels/1048312349867646996/1165372223322869873")
		_ = app.myApp.OpenURL(u)
	})
	contactMenu := fyne.NewMenu(app.messages["menu_contact"], contactNexus, contactDiscord, contactDiscordMy)

	if len(launchItems) > 0 {
		return fyne.NewMainMenu(settingsMenu, contactMenu, launchMenu)
	}
	return fyne.NewMainMenu(settingsMenu, contactMenu)
}

func (app *App) changeLanguage(lang string) {
	if err := app.loadLanguage(lang); err != nil {
		return
	}
	app.cfg.Language = lang
	checks.SetLanguage(lang)
	saveConfig(app.cfg)
	app.mainWindow.SetTitle(app.messages["app_title_long"])
	app.mainWindow.SetMainMenu(app.buildMainMenu())
	sorter.SetSortMessages(app.messages["sort_ru_warning"], app.messages["sort_en_warning"])
	sorter.SetLogMessages(app.messages["log_create_mlot"], app.messages["log_mlot_created"])

	app.searchEntry.SetPlaceHolder(app.messages["search_placeholder"])
	app.filterSelect.Options = []string{
		app.messages["filter_all"], app.messages["filter_active"], app.messages["filter_inactive"],
		app.messages["filter_obsolete"], app.messages["filter_conflict"],
	}
	app.filterSelect.SetSelected(app.messages["filter_all"])
	app.updateToggleButtonText(app.btnToggle)
	app.refreshModList()

	app.modListTitle.SetText(app.messages["mod_list_title"])
	app.filterLabel.SetText(app.messages["filter_label"])
	app.btnSaveOrder.SetText(app.messages["btn_save_order"])
	app.btnSortChecks.SetText(app.messages["btn_sort_checks"])
	app.btnRefresh.SetText(app.messages["btn_refresh"])
	app.btnInstall.SetText(app.messages["btn_install"])
	app.btnRemove.SetText(app.messages["btn_remove"])
	app.btnExport.SetText(app.messages["btn_export"])
	app.btnImport.SetText(app.messages["btn_import"])
	app.btnUp.SetText(app.messages["btn_up"])
	app.btnDown.SetText(app.messages["btn_down"])
	app.updateDescriptionForMod(app.selectedModName)
}

const baseRawURL = "https://raw.githubusercontent.com/xsSplater/Servo-Modquisitor/main/mods/"

func (app *App) checkForUpdates() {
	files := []struct {
		local  string
		remote string
	}{
		{"mod_database.json", baseRawURL + "mod_database.json"},
		{"mandatory_obsolete_incompatible_dependencies.json", baseRawURL + "mandatory_obsolete_incompatible_dependencies.json"},
	}

	var updates []string
	for _, f := range files {
		localPath := filepath.Join(app.cfg.ModsPath, f.local)
		needUpdate, err := app.needFileUpdate(localPath, f.remote)
		if err != nil {
			app.appendLog(fmt.Sprintf("Update check error for %s: %v", f.local, err))
			continue
		}
		if needUpdate {
			updates = append(updates, f.local)
		}
	}

	if len(updates) == 0 {
		app.appendLog(app.messages["no_updates_found"])
		return
	}

	choice := app.showChoiceDialog(app.mainWindow,
		app.messages["update_title"],
		fmt.Sprintf(app.messages["update_message"], strings.Join(updates, ", ")),
		app.messages["yes"],
		app.messages["skip"],
	)
	if choice == 0 {
		var errFiles []string
		for _, f := range files {
			localPath := filepath.Join(app.cfg.ModsPath, f.local)
			if err := app.downloadFile(f.remote, localPath); err != nil {
				errMsg := fmt.Sprintf("Failed to update %s: %v", f.local, err)
				app.appendLog(errMsg)
				errFiles = append(errFiles, f.local)
			} else {
				app.appendLog(fmt.Sprintf(app.messages["update_success"], f.local))
			}
		}

		// Уведомление, если были ошибки
		if len(errFiles) > 0 {
			dialog.ShowInformation(
				app.messages["update_title"],
				"The following files could not be updated:\n"+strings.Join(errFiles, "\n"),
				app.mainWindow,
			)
		}

		// Перезагружаем базы (если файлы не были перезаписаны, останутся старые)
		app.loadModDatabase("mod_database.json")
		checks.SetModDatabase(app.modDatabase)
		checks.LoadExternalLists("mandatory_obsolete_incompatible_dependencies.json")
		app.refreshModList()
	}
}

func (app *App) needFileUpdate(localPath, url string) (bool, error) {
	localData, err := os.ReadFile(localPath)
	if err != nil {
		return true, nil // локального файла нет – нужно «обновить» (скачать)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Сервер вернул не 200 – считаем, что обновления нет, и возвращаем ошибку
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	remoteData, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	localHash := md5.Sum(localData)
	remoteHash := md5.Sum(remoteData)
	return hex.EncodeToString(localHash[:]) != hex.EncodeToString(remoteHash[:]), nil
}

func (app *App) downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}
