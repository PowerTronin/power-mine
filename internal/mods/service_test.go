package mods

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"power-mine/internal/domain"
)

func TestListSetEnabledAndDelete(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "zeta.jar"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "alpha.jar.disabled"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := service.List(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Mods) != 2 {
		t.Fatalf("expected 2 mods, got %d", len(list.Mods))
	}
	if list.Mods[0].FileName != "alpha.jar.disabled" || list.Mods[0].Enabled {
		t.Fatalf("unexpected first mod: %#v", list.Mods[0])
	}

	enabled, err := service.SetEnabled(profile, "alpha.jar.disabled", true)
	if err != nil {
		t.Fatal(err)
	}
	if enabled.FileName != "alpha.jar" || !enabled.Enabled {
		t.Fatalf("unexpected enabled mod: %#v", enabled)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "alpha.jar.disabled")); !os.IsNotExist(err) {
		t.Fatalf("disabled file still exists, err=%v", err)
	}

	disabled, err := service.SetEnabled(profile, "zeta.jar", false)
	if err != nil {
		t.Fatal(err)
	}
	if disabled.FileName != "zeta.jar.disabled" || disabled.Enabled {
		t.Fatalf("unexpected disabled mod: %#v", disabled)
	}

	if err := service.Delete(profile, "zeta.jar.disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "zeta.jar.disabled")); !os.IsNotExist(err) {
		t.Fatalf("deleted file still exists, err=%v", err)
	}
}

func TestImportCopiesJarAndRejectsTraversal(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "mod.jar")
	if err := os.WriteFile(sourcePath, []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	imported, err := service.Import(profile, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if imported.FileName != "mod.jar" || !imported.Enabled || imported.Size != 3 {
		t.Fatalf("unexpected imported mod: %#v", imported)
	}

	if _, err := service.SetEnabled(profile, "../mod.jar", false); err == nil {
		t.Fatal("expected traversal file name to fail")
	}

	textPath := filepath.Join(sourceDir, "mod.txt")
	if err := os.WriteFile(textPath, []byte("txt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Import(profile, textPath); err == nil {
		t.Fatal("expected non-jar import to fail")
	}
}

func TestInstallWritesDownloadedJar(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}

	installed, err := service.Install(profile, "downloaded.jar", strings.NewReader("jar"))
	if err != nil {
		t.Fatal(err)
	}
	if installed.FileName != "downloaded.jar" || installed.Size != 3 {
		t.Fatalf("unexpected installed mod: %#v", installed)
	}

	data, err := os.ReadFile(filepath.Join(profile.GameDir, "mods", "downloaded.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "jar" {
		t.Fatalf("unexpected jar contents %q", string(data))
	}
}

func TestExistingFindsEnabledAndDisabledJar(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "library.jar.disabled"), []byte("jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	existing, ok, err := service.Existing(profile, "library.jar")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || existing.FileName != "library.jar.disabled" || existing.Enabled {
		t.Fatalf("unexpected existing mod: ok=%t mod=%#v", ok, existing)
	}

	if _, ok, err := service.Existing(profile, "missing.jar"); err != nil || ok {
		t.Fatalf("expected missing mod, ok=%t err=%v", ok, err)
	}
}

func TestModrinthInstallRecordDeletePlanAndDelete(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "main.jar"), []byte("main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modsDir, "dep.jar"), []byte("dep"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "main-project",
		ProjectTitle:  "Main Mod",
		VersionID:     "main-version",
		VersionName:   "Main Release",
		VersionNumber: "1.0.0",
		FileName:      "main.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "main-project", VersionID: "main-version", FileName: "main.jar", DisplayName: "Main Mod"},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "dep.jar", DisplayName: "Dependency Mod", DependencyType: "required"},
		},
	}
	if err := service.RecordModrinthInstall(profile, result); err != nil {
		t.Fatal(err)
	}

	ids, err := service.ModrinthInstalledProjectIDs(profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "main-project" {
		t.Fatalf("unexpected installed ids %#v", ids)
	}

	plan, found, err := service.PlanModrinthDelete(profile, "main-project")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(plan.Files) != 2 || len(plan.SkippedFiles) != 0 {
		t.Fatalf("unexpected delete plan found=%t plan=%#v", found, plan)
	}

	deleteResult, found, err := service.DeleteModrinthInstall(profile, "main-project")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(deleteResult.DeletedFiles) != 2 {
		t.Fatalf("unexpected delete result found=%t result=%#v", found, deleteResult)
	}
	for _, fileName := range []string{"main.jar", "dep.jar"} {
		if _, err := os.Stat(filepath.Join(modsDir, fileName)); !os.IsNotExist(err) {
			t.Fatalf("%s should be deleted, err=%v", fileName, err)
		}
	}
}

func TestModrinthDeleteSkipsSharedDependency(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fileName := range []string{"main-a.jar", "main-b.jar", "dep.jar"} {
		if err := os.WriteFile(filepath.Join(modsDir, fileName), []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	recordA := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "project-a",
		ProjectTitle:  "Project A",
		VersionID:     "version-a",
		VersionName:   "A",
		VersionNumber: "1.0.0",
		FileName:      "main-a.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "project-a", VersionID: "version-a", FileName: "main-a.jar", DisplayName: "Project A"},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "dep.jar", DisplayName: "Shared Dependency", DependencyType: "required"},
		},
	}
	recordB := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "project-b",
		ProjectTitle:  "Project B",
		VersionID:     "version-b",
		VersionName:   "B",
		VersionNumber: "1.0.0",
		FileName:      "main-b.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "project-b", VersionID: "version-b", FileName: "main-b.jar", DisplayName: "Project B"},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "dep.jar", DisplayName: "Shared Dependency", DependencyType: "required", AlreadyPresent: true},
		},
	}
	if err := service.RecordModrinthInstall(profile, recordA); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordModrinthInstall(profile, recordB); err != nil {
		t.Fatal(err)
	}

	plan, found, err := service.PlanModrinthDelete(profile, "project-a")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(plan.Files) != 1 || len(plan.SkippedFiles) != 1 {
		t.Fatalf("unexpected delete plan found=%t plan=%#v", found, plan)
	}
	if plan.Files[0].FileName != "main-a.jar" || plan.SkippedFiles[0].FileName != "dep.jar" {
		t.Fatalf("unexpected delete files %#v skipped %#v", plan.Files, plan.SkippedFiles)
	}
	if !strings.Contains(plan.SkippedFiles[0].Reason, "Project B") {
		t.Fatalf("expected shared dependency reason, got %q", plan.SkippedFiles[0].Reason)
	}
}

func TestModrinthDeleteSelectedFilesKeepsUncheckedFiles(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fileName := range []string{"main.jar", "dep.jar"} {
		if err := os.WriteFile(filepath.Join(modsDir, fileName), []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	result := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "main-project",
		ProjectTitle:  "Main Mod",
		VersionID:     "main-version",
		VersionName:   "Main Release",
		VersionNumber: "1.0.0",
		FileName:      "main.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "main-project", VersionID: "main-version", FileName: "main.jar", DisplayName: "Main Mod"},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "dep.jar", DisplayName: "Dependency Mod", DependencyType: "required"},
		},
	}
	if err := service.RecordModrinthInstall(profile, result); err != nil {
		t.Fatal(err)
	}

	deleteResult, found, err := service.DeleteModrinthInstallFiles(profile, "main-project", []string{"dep.jar"})
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(deleteResult.DeletedFiles) != 1 || deleteResult.DeletedFiles[0].FileName != "dep.jar" {
		t.Fatalf("unexpected delete result found=%t result=%#v", found, deleteResult)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "dep.jar")); !os.IsNotExist(err) {
		t.Fatalf("dep.jar should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "main.jar")); err != nil {
		t.Fatalf("main.jar should be kept: %v", err)
	}

	plan, found, err := service.PlanModrinthDelete(profile, "main-project")
	if err != nil {
		t.Fatal(err)
	}
	if !found || len(plan.Files) != 1 || plan.Files[0].FileName != "main.jar" {
		t.Fatalf("expected manifest to keep main file only, found=%t plan=%#v", found, plan)
	}
}

func TestPruneReplacedModrinthFilesRemovesOldMainAndKeepsSharedDependency(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, fileName := range []string{"old-main.jar", "new-main.jar", "shared-dep.jar"} {
		if err := os.WriteFile(filepath.Join(modsDir, fileName), []byte("jar"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	oldRecord := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "project-a",
		ProjectTitle:  "Project A",
		VersionID:     "old-version",
		VersionName:   "Old",
		VersionNumber: "1.0.0",
		FileName:      "old-main.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "project-a", VersionID: "old-version", FileName: "old-main.jar", DisplayName: "Project A"},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "shared-dep.jar", DisplayName: "Shared Dependency", DependencyType: "required"},
		},
	}
	otherRecord := domain.ModrinthInstallResult{
		ProfileID:     profile.ID,
		ProjectID:     "project-b",
		ProjectTitle:  "Project B",
		VersionID:     "version-b",
		VersionName:   "B",
		VersionNumber: "1.0.0",
		FileName:      "project-b.jar",
		InstalledFiles: []domain.ModrinthInstalledFile{
			{ProjectID: "project-b", VersionID: "version-b", FileName: "project-b.jar", DisplayName: "Project B", AlreadyPresent: true},
			{ProjectID: "dep-project", VersionID: "dep-version", FileName: "shared-dep.jar", DisplayName: "Shared Dependency", DependencyType: "required", AlreadyPresent: true},
		},
	}
	if err := service.RecordModrinthInstall(profile, oldRecord); err != nil {
		t.Fatal(err)
	}
	if err := service.RecordModrinthInstall(profile, otherRecord); err != nil {
		t.Fatal(err)
	}

	deleted, skipped, err := service.PruneReplacedModrinthFiles(profile, "project-a", []domain.ModrinthInstalledFile{
		{ProjectID: "project-a", VersionID: "new-version", FileName: "new-main.jar", DisplayName: "Project A"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || deleted[0].FileName != "old-main.jar" {
		t.Fatalf("unexpected deleted files %#v", deleted)
	}
	if len(skipped) != 1 || skipped[0].FileName != "shared-dep.jar" || !strings.Contains(skipped[0].Reason, "Project B") {
		t.Fatalf("unexpected skipped files %#v", skipped)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "old-main.jar")); !os.IsNotExist(err) {
		t.Fatalf("old-main.jar should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(modsDir, "shared-dep.jar")); err != nil {
		t.Fatalf("shared dependency should be kept: %v", err)
	}
}

func TestListUsesModMetadataDisplayName(t *testing.T) {
	service := NewService()
	profile := domain.Profile{ID: "profile-1", GameDir: t.TempDir()}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeZip(filepath.Join(modsDir, "fabric-api.jar"), "fabric.mod.json", []byte(`{"id":"fabric-api","name":"Fabric API"}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeZip(filepath.Join(modsDir, "forge-lib.jar"), "META-INF/mods.toml", []byte("modId=\"forge_lib\"\ndisplayName=\"Forge Library\"\n")); err != nil {
		t.Fatal(err)
	}
	if err := writeZip(filepath.Join(modsDir, "quilt-lib.jar.disabled"), "quilt.mod.json", []byte(`{"quilt_loader":{"id":"quilt_lib","metadata":{"name":"Quilt Library"}}}`)); err != nil {
		t.Fatal(err)
	}

	list, err := service.List(profile)
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]string{}
	for _, mod := range list.Mods {
		names[mod.FileName] = mod.DisplayName
	}
	if names["fabric-api.jar"] != "Fabric API" {
		t.Fatalf("unexpected Fabric display name: %q", names["fabric-api.jar"])
	}
	if names["forge-lib.jar"] != "Forge Library" {
		t.Fatalf("unexpected Forge display name: %q", names["forge-lib.jar"])
	}
	if names["quilt-lib.jar.disabled"] != "Quilt Library" {
		t.Fatalf("unexpected Quilt display name: %q", names["quilt-lib.jar.disabled"])
	}
}

func writeZip(path string, name string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := entry.Write(data); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
