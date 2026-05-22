package modpacks

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"power-mine/internal/domain"
)

func TestProfileInputFromMrpackDependencies(t *testing.T) {
	index := Index{
		FormatVersion: 1,
		Game:          "minecraft",
		Name:          "Pack",
		Dependencies: map[string]string{
			"minecraft":     "1.21.1",
			"fabric-loader": "0.16.14",
		},
	}

	input, err := ProfileInput(index, domain.MemorySettings{MinMB: 1024, MaxMB: 4096})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Pack" || input.MinecraftVersion != "1.21.1" {
		t.Fatalf("unexpected input %#v", input)
	}
	if input.Loader.Type != domain.LoaderFabric || input.Loader.Version != "0.16.14" {
		t.Fatalf("unexpected loader %#v", input.Loader)
	}
}

func TestInstallClientSkipsExistingFileAndAppliesClientOverrides(t *testing.T) {
	gameDir := t.TempDir()
	modData := []byte("mod jar")
	sha1Sum := sha1.Sum(modData)
	sha512Sum := sha512.Sum512(modData)
	modPath := filepath.Join(gameDir, "mods", "existing.jar")
	if err := os.MkdirAll(filepath.Dir(modPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modPath, modData, 0o644); err != nil {
		t.Fatal(err)
	}

	packPath := filepath.Join(t.TempDir(), "pack.mrpack")
	index := Index{
		FormatVersion: 1,
		Game:          "minecraft",
		Name:          "Pack",
		Dependencies:  map[string]string{"minecraft": "1.21.1"},
		Files: []IndexFile{{
			Path:     "mods/existing.jar",
			FileSize: int64(len(modData)),
			Hashes: map[string]string{
				"sha1":   hex.EncodeToString(sha1Sum[:]),
				"sha512": hex.EncodeToString(sha512Sum[:]),
			},
			Downloads: []string{"https://cdn.modrinth.com/data/project/versions/file/existing.jar"},
		}},
	}
	if err := writeTestMrpack(packPath, index, map[string]string{
		"overrides/config/base.toml":          "base",
		"client-overrides/config/client.toml": "client",
		"server-overrides/config/server.toml": "server",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := NewService().InstallClient(context.Background(), packPath, domain.Profile{
		ID:      "profile",
		GameDir: gameDir,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesInstalled != 0 || result.FilesSkipped != 1 || result.OverridesInstalled != 2 {
		t.Fatalf("unexpected result %#v", result)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "config", "base.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "config", "client.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(gameDir, "config", "server.toml")); !os.IsNotExist(err) {
		t.Fatalf("server override should not be applied, err=%v", err)
	}
}

func TestInstallClientRejectsUnsafePaths(t *testing.T) {
	packPath := filepath.Join(t.TempDir(), "pack.mrpack")
	index := Index{
		FormatVersion: 1,
		Game:          "minecraft",
		Dependencies:  map[string]string{"minecraft": "1.21.1"},
		Files: []IndexFile{{
			Path:      "../escape.jar",
			Downloads: []string{"https://cdn.modrinth.com/data/project/versions/file/escape.jar"},
		}},
	}
	if err := writeTestMrpack(packPath, index, nil); err != nil {
		t.Fatal(err)
	}

	_, err := NewService().InstallClient(context.Background(), packPath, domain.Profile{
		ID:      "profile",
		GameDir: t.TempDir(),
	}, nil)
	if err == nil {
		t.Fatal("expected unsafe path error")
	}
}

func TestExportWritesPortableMrpackOverrides(t *testing.T) {
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "mods", "mod-a.jar"), "jar a")
	writeTestFile(t, filepath.Join(gameDir, "mods", "mod-b.jar.disabled"), "jar b")
	writeTestFile(t, filepath.Join(gameDir, "mods", ".power-mine-modrinth.json"), "{}")
	writeTestFile(t, filepath.Join(gameDir, "mods", "readme.txt"), "skip")
	writeTestFile(t, filepath.Join(gameDir, "mods", "download.jar.part"), "skip")
	writeTestFile(t, filepath.Join(gameDir, "config", "base.toml"), "config")
	writeTestFile(t, filepath.Join(gameDir, "resourcepacks", "resources.zip"), "resources")
	writeTestFile(t, filepath.Join(gameDir, "saves", "world", "level.dat"), "skip")
	writeTestFile(t, filepath.Join(gameDir, "options.txt"), "options")

	targetPath := filepath.Join(t.TempDir(), "pack.mrpack")
	result, err := NewService().Export(domain.Profile{
		ID:               "profile",
		Name:             "Pack",
		MinecraftVersion: "1.20.1",
		Loader: domain.LoaderConfig{
			Type:    domain.LoaderFabric,
			Version: "0.16.14",
		},
		GameDir: gameDir,
	}, targetPath, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.VersionID == "" || result.OverridesExported != 5 {
		t.Fatalf("unexpected result %#v", result)
	}

	index, entries := readTestMrpack(t, targetPath)
	if index.FormatVersion != 1 || index.Game != "minecraft" || index.Name != "Pack" {
		t.Fatalf("unexpected index %#v", index)
	}
	if index.Dependencies["minecraft"] != "1.20.1" || index.Dependencies["fabric-loader"] != "0.16.14" {
		t.Fatalf("unexpected dependencies %#v", index.Dependencies)
	}
	if len(index.Files) != 0 {
		t.Fatalf("portable export should embed local files as overrides, got %#v", index.Files)
	}
	for _, name := range []string{
		"overrides/config/base.toml",
		"overrides/mods/mod-a.jar",
		"overrides/mods/mod-b.jar.disabled",
		"overrides/options.txt",
		"overrides/resourcepacks/resources.zip",
	} {
		if _, ok := entries[name]; !ok {
			t.Fatalf("missing exported file %s", name)
		}
	}
	for _, name := range []string{
		"overrides/mods/.power-mine-modrinth.json",
		"overrides/mods/readme.txt",
		"overrides/mods/download.jar.part",
		"overrides/saves/world/level.dat",
	} {
		if _, ok := entries[name]; ok {
			t.Fatalf("unexpected exported file %s", name)
		}
	}
}

func TestExportUsesIndexFilesForTrackedModrinthFiles(t *testing.T) {
	gameDir := t.TempDir()
	writeTestFile(t, filepath.Join(gameDir, "mods", "tracked.jar"), "tracked")
	writeTestFile(t, filepath.Join(gameDir, "mods", "local.jar"), "local")

	targetPath := filepath.Join(t.TempDir(), "pack.mrpack")
	result, err := NewService().Export(domain.Profile{
		ID:               "profile",
		Name:             "Pack",
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderVanilla},
		GameDir:          gameDir,
	}, targetPath, ExportOptions{Files: []ExportFile{{
		Path:      "mods/tracked.jar",
		Downloads: []string{"https://cdn.modrinth.com/data/project/versions/file/tracked.jar"},
		SHA1:      "4c54cc4d8b2e8d1062d92fad1ad9389fbe33c1ae",
		FileSize:  int64(len("tracked")),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesExported != 1 || result.OverridesExported != 1 {
		t.Fatalf("unexpected result %#v", result)
	}

	index, entries := readTestMrpack(t, targetPath)
	if len(index.Files) != 1 {
		t.Fatalf("expected one index file, got %#v", index.Files)
	}
	file := index.Files[0]
	if file.Path != "mods/tracked.jar" || file.Hashes["sha1"] != "4c54cc4d8b2e8d1062d92fad1ad9389fbe33c1ae" || file.FileSize != int64(len("tracked")) {
		t.Fatalf("unexpected index file %#v", file)
	}
	if len(file.Downloads) != 1 || file.Downloads[0] != "https://cdn.modrinth.com/data/project/versions/file/tracked.jar" {
		t.Fatalf("unexpected downloads %#v", file.Downloads)
	}
	if _, ok := entries["overrides/mods/tracked.jar"]; ok {
		t.Fatal("tracked Modrinth file should not be duplicated as an override")
	}
	if _, ok := entries["overrides/mods/local.jar"]; !ok {
		t.Fatal("local jar should remain embedded as an override")
	}
}

func writeTestMrpack(path string, index Index, files map[string]string) error {
	handle, err := os.Create(path)
	if err != nil {
		return err
	}
	defer handle.Close()

	writer := zip.NewWriter(handle)
	body, err := json.Marshal(index)
	if err != nil {
		return err
	}
	entry, err := writer.Create("modrinth.index.json")
	if err != nil {
		return err
	}
	if _, err := entry.Write(body); err != nil {
		return err
	}
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			return err
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			return err
		}
	}
	return writer.Close()
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

func readTestMrpack(t *testing.T, path string) (Index, map[string][]byte) {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	entries := map[string][]byte{}
	for _, file := range reader.File {
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(body)
		if closeErr := body.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = content
	}

	var index Index
	if err := json.Unmarshal(entries["modrinth.index.json"], &index); err != nil {
		t.Fatal(err)
	}
	return index, entries
}
