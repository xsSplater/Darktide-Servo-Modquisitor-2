// Servo-Modquisitor-2/trash.go
package main

import (
	"github.com/Bios-Marcel/wastebasket/v2"
)

// moveToTrash перемещает указанный путь в системную корзину.
//
// Использует библиотеку wastebasket, которая под капотом:
//   - Windows: вызывает современный COM-интерфейс IFileOperation,
//     который поддерживает длинные пути (> 260 символов) и не падает
//     с ошибкой SHFileOperationW 124.
//   - Linux/Unix: реализует спецификацию FreeDesktop Trash напрямую,
//     без необходимости в установленных утилитах gio или trash-cli.
//
// CGO не требуется.
func moveToTrash(path string) error {
	return wastebasket.Trash(path)
}
