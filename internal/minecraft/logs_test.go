package minecraft

import (
	"compress/gzip"
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

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
