// Servo-Modquisitor-2/trash.go
package main

// moveToTrash — единичное удаление. Оставлено для совместимости
// с checks.RemoveMod.
func moveToTrash(path string) error {
	return moveManyToTrash([]string{path})
}

// moveManyToTrash — пакетное удаление. На Windows один вызов
// IFileOperation удаляет N путей сразу — в разы быстрее, чем N
// вызовов SHFileOperationW (старый бэкенд wastebasket). На Linux/Mac
// используется wastebasket (FreeDesktop Trash = rename, там и так быстро).
func moveManyToTrash(paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	return trashPaths(paths)
}
