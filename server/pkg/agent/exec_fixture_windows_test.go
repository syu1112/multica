//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testExecutablePath(dir, name string) string {
	if filepath.Ext(name) == "" {
		name += ".cmd"
	}
	return filepath.Join(dir, name)
}

// writeTestExecutable is the Windows counterpart to the //go:build unix
// implementation in exec_fixture_unix_test.go. ETXTBSY is a Linux/Unix
// fork-exec race; Windows doesn't have that pathology, so a plain
// os.WriteFile is sufficient.
//
// The helper is referenced by claude_test.go / codex_test.go /
// kimi_test.go, so the absence of a Windows impl made
// `go test ./pkg/agent` fail to build on Windows. Lifted from #1719
// (Codex) with attribution.
func writeTestExecutable(tb testing.TB, path string, content []byte) {
	tb.Helper()
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".cmd" || ext == ".bat" {
		scriptPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".sh"
		if err := os.WriteFile(scriptPath, content, 0o755); err != nil {
			tb.Fatalf("write test executable script %s: %v", scriptPath, err)
		}
		wrapper := []byte("@echo off\r\nsh \"%~dpn0.sh\" %*\r\nexit /b %ERRORLEVEL%\r\n")
		if err := os.WriteFile(path, wrapper, 0o755); err != nil {
			tb.Fatalf("write test executable wrapper %s: %v", path, err)
		}
		return
	}
	if err := os.WriteFile(path, content, 0o755); err != nil {
		tb.Fatalf("write test executable %s: %v", path, err)
	}
}
