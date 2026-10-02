// Servo-Modquisitor-2/bundle_patch_test.go
package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────────
// Независимая точка истины
//
// Это НЕ ссылка на modPatchStartingPoint и НЕ ссылка на содержимое
// patch.bin — это ровно то, что должно быть в обоих. Взято из hex-дампа
// реального bundle_database.data (u64::to_be_bytes(0xA33A4AA4AF26A69B)
// в исходниках dtkit-patch).
//
// Задаётся hex-строкой, чтобы при копипасте нельзя было перепутать
// escape-последовательности: "\x9b\xa3..." vs "\xa3\x3a..." визуально
// почти неотличимы, а "9b a3 ..." vs "a3 3a ..." — бросается в глаза.
// ─────────────────────────────────────────────────────────────────
const expectedSignatureHex = "a33a4aa4af26a69b"

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad hex fixture %q: %v", s, err)
	}
	return b
}

// ─────────────────────────────────────────────────────────────────
// Проверки самой сигнатуры и бинарника
// ─────────────────────────────────────────────────────────────────

// TestPatchSignature_Constant — константа modPatchStartingPoint
// содержит ровно ожидаемые байты. Ловит опечатки вида «перепутан
// первый и последний байт» — ровно та ошибка, из-за которой моды
// выключались, но не включались.
func TestPatchSignature_Constant(t *testing.T) {
	want := mustHex(t, expectedSignatureHex)
	got := []byte(modPatchStartingPoint)
	if !bytes.Equal(got, want) {
		t.Fatalf("modPatchStartingPoint = %x, want %x", got, want)
	}
}

// TestPatchSignature_EmbeddedBin — assets/patch.bin начинается с той
// же сигнатуры. Это независимая проверка: если кто-то заменит бинарник
// на патч от другой версии игры или на мусор, тест упадёт.
func TestPatchSignature_EmbeddedBin(t *testing.T) {
	want := mustHex(t, expectedSignatureHex)
	if len(bundlePatchBin) < len(want) {
		t.Fatalf("bundlePatchBin too short: %d bytes", len(bundlePatchBin))
	}
	if !bytes.HasPrefix(bundlePatchBin, want) {
		t.Fatalf("bundlePatchBin does not start with %x; first 8 bytes = %x",
			want, bundlePatchBin[:8])
	}
}

// TestPatchSizes_Documented — фиксируем размеры, которые были
// проверены вручную по dtkit-patch и по реальному
// bundle_database.data. Если кто-то случайно поменяет 84 на другое
// число — тест это поймает.
func TestPatchSizes_Documented(t *testing.T) {
	if bundleOldSize != 84 {
		t.Fatalf("bundleOldSize = %d, want 84 (verified against dtkit-patch)", bundleOldSize)
	}
	if len(bundlePatchBin) != 184 {
		t.Fatalf("len(bundlePatchBin) = %d, want 184 (verified from hex dump)", len(bundlePatchBin))
	}
}

// ─────────────────────────────────────────────────────────────────
// Fixture: минимальный «оригинальный» bundle_database.data
// ─────────────────────────────────────────────────────────────────

// newOriginalDB собирает файл из трёх частей:
//  1. сигнатура (из expectedSignatureHex — независимо от константы);
//  2. тело дескриптора длиной ровно bundleOldSize байт;
//  3. хвост, который не должен быть тронут при патче.
//
// Форма соответствует реальному bundle_database.data в той части,
// которая нужна PatchBundle: якорь, 84 байта дескриптора, потом ещё
// данные.
func newOriginalDB(t *testing.T) []byte {
	t.Helper()
	sig := mustHex(t, expectedSignatureHex)

	body := make([]byte, bundleOldSize)
	for i := range body {
		body[i] = byte(0x40 + i) // любой ненулевой мусор
	}
	tail := []byte("tail-bytes-after-descriptor-data")

	out := make([]byte, 0, len(sig)+len(body)+len(tail))
	out = append(out, sig...)
	out = append(out, body...)
	out = append(out, tail...)
	return out
}

func writeDB(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "bundle"), 0755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bundle", bundleDatabaseName), data, 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}
}

func readDB(t *testing.T, root string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "bundle", bundleDatabaseName))
	if err != nil {
		t.Fatalf("read db: %v", err)
	}
	return data
}

func backupPath(root string) string {
	return filepath.Join(root, "bundle", bundleDatabaseBak)
}

// ─────────────────────────────────────────────────────────────────
// Главный тест: полный roundtrip
// ─────────────────────────────────────────────────────────────────

// TestPatchBundle_Roundtrip — патчим, распатчиваем, сверяем побайтово,
// патчим снова. Это ровно тот сценарий, в котором пользователь увидел
// баг «моды выключаются, но не включаются обратно»: UnpatchBundle (rename)
// всегда работал, а PatchBundle падал на поиске сигнатуры.
func TestPatchBundle_Roundtrip(t *testing.T) {
	root := t.TempDir()
	original := newOriginalDB(t)
	writeDB(t, root, original)

	// ── 1. Патч ────────────────────────────────────────────────
	if err := PatchBundle(root); err != nil {
		t.Fatalf("PatchBundle: %v", err)
	}

	patched := readDB(t, root)

	if !bytes.Contains(patched, []byte(modPatchTag)) {
		t.Fatalf(".patch_999 not found after patch")
	}

	// Длина выросла ровно на разницу между patch.bin и старым дескриптором.
	wantLen := len(original) - bundleOldSize + len(bundlePatchBin)
	if len(patched) != wantLen {
		t.Fatalf("patched length = %d, want %d", len(patched), wantLen)
	}

	// Сигнатура и всё, что до неё, не тронуты.
	sigLen := len(mustHex(t, expectedSignatureHex))
	if !bytes.Equal(patched[:sigLen], original[:sigLen]) {
		t.Fatalf("signature prefix changed: %x vs %x", patched[:sigLen], original[:sigLen])
	}

	// Хвост не тронут.
	if !bytes.HasSuffix(patched, original[len(original)-20:]) {
		t.Fatalf("tail changed")
	}

	// Бэкап создан и побайтово равен оригиналу.
	bak, err := os.ReadFile(backupPath(root))
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if !bytes.Equal(bak, original) {
		t.Fatalf("backup differs from original")
	}

	// ── 2. Распатч ─────────────────────────────────────────────
	if err := UnpatchBundle(root); err != nil {
		t.Fatalf("UnpatchBundle: %v", err)
	}

	restored := readDB(t, root)
	if !bytes.Equal(restored, original) {
		t.Fatalf("restored db differs from original")
	}

	// Бэкап после распатча исчез (rename сдвинул файл).
	if _, err := os.Stat(backupPath(root)); !os.IsNotExist(err) {
		t.Fatalf("backup should be gone after unpatch")
	}

	// ── 3. Второй цикл: патч снова ─────────────────────────────
	if err := PatchBundle(root); err != nil {
		t.Fatalf("second PatchBundle: %v", err)
	}
	patched2 := readDB(t, root)
	if !bytes.Contains(patched2, []byte(modPatchTag)) {
		t.Fatalf(".patch_999 not found after second patch")
	}
}

// ─────────────────────────────────────────────────────────────────
// Граничные случаи
// ─────────────────────────────────────────────────────────────────

// TestPatchBundle_AlreadyPatched — повторный вызов PatchBundle на
// уже пропатченном файле возвращает ErrBundleAlreadyPatched, а не
// портит файл.
func TestPatchBundle_AlreadyPatched(t *testing.T) {
	root := t.TempDir()
	writeDB(t, root, newOriginalDB(t))

	if err := PatchBundle(root); err != nil {
		t.Fatalf("first PatchBundle: %v", err)
	}
	patchedOnce := readDB(t, root)

	err := PatchBundle(root)
	if !errors.Is(err, ErrBundleAlreadyPatched) {
		t.Fatalf("second PatchBundle err = %v, want ErrBundleAlreadyPatched", err)
	}

	// Файл не изменился.
	if !bytes.Equal(patchedOnce, readDB(t, root)) {
		t.Fatalf("file was modified by rejected second PatchBundle")
	}
}

// TestPatchBundle_ForeignPatch — если в файле уже есть чужой
// устаревший патч (.patch_001), патчить нельзя.
func TestPatchBundle_ForeignPatch(t *testing.T) {
	root := t.TempDir()
	original := newOriginalDB(t)
	original = append(original, []byte(bootBundleNextPatch)...)
	writeDB(t, root, original)

	err := PatchBundle(root)
	if !errors.Is(err, ErrBundleUnsupported) {
		t.Fatalf("PatchBundle err = %v, want ErrBundleUnsupported", err)
	}

	// Файл не изменён.
	if !bytes.Equal(original, readDB(t, root)) {
		t.Fatalf("file was modified by rejected PatchBundle")
	}
}

// TestPatchBundle_NoSignature — файл без якорной сигнатуры (симуляция
// сильно обновлённой игры) → ErrBundlePatchOffset.
func TestPatchBundle_NoSignature(t *testing.T) {
	root := t.TempDir()
	writeDB(t, root, []byte("no signature here, just random bytes"))

	err := PatchBundle(root)
	if !errors.Is(err, ErrBundlePatchOffset) {
		t.Fatalf("PatchBundle err = %v, want ErrBundlePatchOffset", err)
	}
}

// TestPatchBundle_TruncatedAtSignature — сигнатура есть, но после неё
// меньше bundleOldSize байт (битый файл). Должно вернуть
// ErrBundlePatchOffset, а не паниковать на срезе.
func TestPatchBundle_TruncatedAtSignature(t *testing.T) {
	root := t.TempDir()
	sig := mustHex(t, expectedSignatureHex)
	// Только сигнатура + 10 байт — меньше 84.
	data := append(sig, make([]byte, 10)...)
	writeDB(t, root, data)

	err := PatchBundle(root)
	if !errors.Is(err, ErrBundlePatchOffset) {
		t.Fatalf("PatchBundle err = %v, want ErrBundlePatchOffset", err)
	}
}

// TestUnpatchBundle_NoBackup — распатч без .bak не ошибка, просто no-op.
func TestUnpatchBundle_NoBackup(t *testing.T) {
	root := t.TempDir()
	original := newOriginalDB(t)
	writeDB(t, root, original)

	if err := UnpatchBundle(root); err != nil {
		t.Fatalf("UnpatchBundle without backup: %v", err)
	}
	// Файл не тронут.
	if !bytes.Equal(original, readDB(t, root)) {
		t.Fatalf("file changed by no-op UnpatchBundle")
	}
}

// TestToggleBundle_States — ToggleBundle возвращает корректное новое
// состояние в обоих направлениях.
func TestToggleBundle_States(t *testing.T) {
	root := t.TempDir()
	writeDB(t, root, newOriginalDB(t))

	state, err := ToggleBundle(root)
	if err != nil {
		t.Fatalf("ToggleBundle → patch: %v", err)
	}
	if !state {
		t.Fatalf("after first toggle state = false, want true")
	}

	state, err = ToggleBundle(root)
	if err != nil {
		t.Fatalf("ToggleBundle → unpatch: %v", err)
	}
	if state {
		t.Fatalf("after second toggle state = true, want false")
	}
}
