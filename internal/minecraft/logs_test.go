package minecraft

import (
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"power-mine/internal/domain"
)

func TestListGameLogsIncludesLogsCrashReportsAndJvmLogs(t *testing.T) {
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "logs", "latest.log"), "latest")
	writeTestFile(t, filepath.Join(gameDir, "logs", "old.log.gz"), "compressed")
	writeTestFile(t, filepath.Join(gameDir, "logs", "ignored.txt"), "ignored")
	writeTestFile(t, filepath.Join(gameDir, "crash-reports", "crash-2026-01-01.txt"), "crash")
	writeTestFile(t, filepath.Join(gameDir, "hs_err_pid123.log"), "jvm")

	service := NewService(t.TempDir())
	list, err := service.ListGameLogs(domain.Profile{ID: "profile-1", GameDir: gameDir})
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]string{}
	for _, file := range list.Files {
		found[file.FileName] = file.Kind
	}
	for fileName, kind := range map[string]string{
		"logs/latest.log":                    "log",
		"logs/old.log.gz":                    "log",
		"crash-reports/crash-2026-01-01.txt": "crash",
		"hs_err_pid123.log":                  "jvm",
	} {
		if found[fileName] != kind {
			t.Fatalf("missing %s kind %s in %#v", fileName, kind, list.Files)
		}
	}
	if _, ok := found["logs/ignored.txt"]; ok {
		t.Fatalf("unexpected ignored file in %#v", list.Files)
	}
}

func TestReadGameLogTailsLargePlainLog(t *testing.T) {
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "logs", "latest.log"), strings.Repeat("a", int(maxGameLogReadBytes)+32)+"tail")

	service := NewService(t.TempDir())
	content, err := service.ReadGameLog(domain.Profile{ID: "profile-1", GameDir: gameDir}, "logs/latest.log")
	if err != nil {
		t.Fatal(err)
	}
	if !content.Truncated {
		t.Fatal("expected truncated content")
	}
	if !strings.HasSuffix(content.Content, "tail") {
		t.Fatalf("expected tail content, got suffix %q", content.Content[len(content.Content)-16:])
	}
}

func TestReadGameLogReadsCompressedLog(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "logs", "old.log.gz")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := gzip.NewWriter(file)
	if _, err := writer.Write([]byte("compressed log body")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	service := NewService(t.TempDir())
	content, err := service.ReadGameLog(domain.Profile{ID: "profile-1", GameDir: gameDir}, "logs/old.log.gz")
	if err != nil {
		t.Fatal(err)
	}
	if content.Content != "compressed log body" {
		t.Fatalf("unexpected content %q", content.Content)
	}
}

func TestReadGameLogRejectsPathTraversal(t *testing.T) {
	service := NewService(t.TempDir())
	_, err := service.ReadGameLog(domain.Profile{ID: "profile-1", GameDir: t.TempDir()}, "../settings.json")
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

func TestExportGameLogsWritesFullArchiveAndLauncherEvents(t *testing.T) {
	gameDir := t.TempDir()
	fullLog := strings.Repeat("full", int(maxGameLogReadBytes/4)+1)
	writeTestFile(t, filepath.Join(gameDir, "logs", "latest.log"), fullLog)
	writeTestFile(t, filepath.Join(gameDir, "crash-reports", "crash-2026-01-01.txt"), "crash")
	writeTestFile(t, filepath.Join(gameDir, "hs_err_pid123.log"), "jvm")

	targetPath := filepath.Join(t.TempDir(), "logs.zip")
	service := NewService(t.TempDir())
	result, err := service.ExportGameLogs(
		domain.Profile{ID: "profile-1", Name: "Test Profile", GameDir: gameDir},
		targetPath,
		"[12:00:00] launcher event",
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesExported != 3 {
		t.Fatalf("FilesExported = %d, want 3", result.FilesExported)
	}
	if !result.LauncherEventsExported {
		t.Fatal("expected launcher events to be exported")
	}

	entries := readZipEntries(t, targetPath)
	if entries["game/logs/latest.log"] != fullLog {
		t.Fatal("expected full latest.log content in export")
	}
	if entries["game/crash-reports/crash-2026-01-01.txt"] != "crash" {
		t.Fatal("expected crash report in export")
	}
	if entries["game/hs_err_pid123.log"] != "jvm" {
		t.Fatal("expected JVM error log in export")
	}
	if entries["launcher-events.log"] != "[12:00:00] launcher event\n" {
		t.Fatalf("unexpected launcher events content %q", entries["launcher-events.log"])
	}
	if !strings.Contains(entries["manifest.txt"], "Test Profile") {
		t.Fatal("expected profile name in manifest")
	}
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readZipEntries(t *testing.T, path string) map[string]string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := map[string]string{}
	for _, file := range reader.File {
		handle, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(handle)
		if closeErr := handle.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = string(body)
	}
	return entries
}
