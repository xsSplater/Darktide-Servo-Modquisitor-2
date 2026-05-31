// theme_colors.go
package themes

// Имена собственных цветов, используемых в интерфейсе.
// Их значения задаются в тёмной и светлой теме.
const (
	// Статусы модов
	ColorStatusSystem    = "color-status-system"
	ColorStatusBroken    = "color-status-broken"
	ColorStatusConflict  = "color-status-conflict"
	ColorStatusObsolete  = "color-status-obsolete"
	ColorStatusMandatory = "color-status-mandatory"
	ColorStatusActive    = "color-status-active"
	ColorStatusInactive  = "color-status-inactive"
	ColorStatusVortex    = "color-status-vortex"

	// Таблица
	ColorTableRowEven     = "color-table-row-even"
	ColorTableRowOdd      = "color-table-row-odd"
	ColorTableRowSelected = "color-table-row-selected"
	ColorTableRowConflict = "color-table-row-conflict"
	ColorTableBorderDirty = "color-table-border-dirty"
	ColorTableHeaderBg    = "color-table-header-bg"
	ColorSystemTableBg    = "color-system-table-bg"

	// CRT-консоль
	ColorConsoleText     = "color-console-text"
	ColorCRTScreenFill   = "color-crt-screen-fill"
	ColorCRTScreenStroke = "color-crt-screen-stroke"
	ColorCRTHeaderBg     = "color-crt-header-bg"

	// Панели и карточки
	ColorDescCardStroke = "color-desc-card-stroke"
	ColorDescCardBg     = "color-desc-card-bg"
	ColorManagePanelBg  = "color-manage-panel-bg"
	ColorTopPanelBg     = "color-top-panel-bg"
	ColorTipBg          = "color-tip-bg"

	// Кастомная кнопка
	ColorButtonShadow         = "color-button-shadow"
	ColorButtonShadowDisabled = "color-button-shadow-disabled"
	ColorButtonStroke         = "color-button-stroke"
	ColorButtonStrokeImage    = "color-button-stroke-image"
)
