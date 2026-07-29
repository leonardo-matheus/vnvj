package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestChecksumAndSafeExtraction(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "runtime.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("runtime/bin/tool.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("binário fictício")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := validateSHA256(archive, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	if err := extractZIP(archive, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
}

func TestExtractionRejectsZipSlip(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "unsafe.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("não deve sair"))
	_ = writer.Close()
	_ = file.Close()
	if err := extractZIP(archive, filepath.Join(root, "out")); err == nil {
		t.Fatal("esperava rejeição de Zip Slip")
	}
}

func TestProfileBlockIsIdempotent(t *testing.T) {
	block := powerShellBlock()
	first := upsertBlock("Write-Host 'antes'\n", block)
	second := upsertBlock(first, block)
	if first != second || strings.Count(second, profileStart) != 1 {
		t.Fatal("bloco do perfil não é idempotente")
	}
	if !strings.Contains(block, ".ps1") || strings.Contains(block, "GetTempFileName") {
		t.Fatal("o PowerShell precisa gerar um script .ps1")
	}
}

func TestPathWithoutManagedEntries(t *testing.T) {
	path := strings.Join([]string{`C:\Node`, `C:\Windows`, `C:\Java\bin`}, ";")
	got := pathWithout(path, `C:\Node`, `C:\Java\bin`)
	if got != `C:\Windows` {
		t.Fatalf("esperava apenas o PATH-base, obteve %q", got)
	}
}

func TestCmdScriptUsesCurrentPathWithoutManagedEntries(t *testing.T) {
	file, err := os.CreateTemp(os.TempDir(), "vnvj-session-*.cmd")
	if err != nil {
		t.Fatal(err)
	}
	script := file.Name()
	file.Close()
	defer os.Remove(script)
	t.Setenv("VNVJ_SCRIPT", script)
	t.Setenv("PATH", `C:\Node;C:\Windows;C:\Java\bin`)
	t.Setenv("NODE_HOME", `C:\Node`)
	t.Setenv("JAVA_HOME", `C:\Java`)
	if err := writeEnvironmentScript("cmd", `C:\Node18`, ""); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	got := string(content)
	if strings.Contains(got, "VNVJ_BASE_PATH") || !strings.Contains(got, `C:\Windows`) {
		t.Fatalf("script CMD inválido: %s", got)
	}
}

func TestWithinDirectoryAcceptsShortAndLongWindowsPaths(t *testing.T) {
	shortTemp := shortPath(t, os.TempDir())
	longTemp := longPath(t, os.TempDir())
	if strings.EqualFold(shortTemp, longTemp) {
		t.Skip("o diretório temporário não possui alias 8.3")
	}
	file, err := os.CreateTemp(longTemp, "vnvj-test-*")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	defer os.Remove(file.Name())
	longFile := longPath(t, file.Name())
	if !withinDirectory(shortTemp, longFile) {
		t.Fatalf("%s e %s deveriam representar o mesmo diretório", shortTemp, filepath.Dir(longFile))
	}
}

func longPath(t *testing.T, path string) string {
	t.Helper()
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, syscall.MAX_PATH)
	length, err := syscall.GetLongPathName(pathPtr, &buffer[0], uint32(len(buffer)))
	if err != nil {
		t.Fatal(err)
	}
	return syscall.UTF16ToString(buffer[:length])
}

func shortPath(t *testing.T, path string) string {
	t.Helper()
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getShortPathName := kernel32.NewProc("GetShortPathNameW")
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, syscall.MAX_PATH)
	length, _, callErr := getShortPathName.Call(uintptr(unsafe.Pointer(pathPtr)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		t.Fatal(callErr)
	}
	return syscall.UTF16ToString(buffer[:length])
}
