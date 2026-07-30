package main

import (
	"context"
	"testing"

	"power-mine/internal/domain"
)

func TestReserveLaunchBlocksDuplicateUntilReleased(t *testing.T) {
	app := NewApp()

	if err := app.reserveLaunch("profile"); err != nil {
		t.Fatalf("reserveLaunch returned error: %v", err)
	}
	if err := app.reserveLaunch("profile"); err == nil {
		t.Fatal("reserveLaunch should reject duplicate launch")
	}

	app.releaseLaunch("profile")
	if err := app.reserveLaunch("profile"); err != nil {
		t.Fatalf("reserveLaunch after release returned error: %v", err)
	}
}

func TestReleaseLaunchReportsUserStop(t *testing.T) {
	app := NewApp()

	if err := app.reserveLaunch("profile"); err != nil {
		t.Fatalf("reserveLaunch returned error: %v", err)
	}
	app.stopping["profile"] = true

	if stopping := app.releaseLaunch("profile"); !stopping {
		t.Fatal("releaseLaunch should report an intentional stop")
	}
	if _, ok := app.running["profile"]; ok {
		t.Fatal("releaseLaunch should remove running command")
	}
	if _, ok := app.stopping["profile"]; ok {
		t.Fatal("releaseLaunch should remove stopping marker")
	}
}

func TestModrinthDependencySelectionMatchesVersionProjectOrFile(t *testing.T) {
	selected := selectedModrinthDependencyMap([]string{"version-1", "project-2", "mod-3.jar"})

	cases := []domain.ModrinthVersion{
		{ID: "version-1"},
		{ProjectID: "project-2"},
		{File: domain.ModrinthVersionFile{FileName: "mod-3.jar"}},
	}
	for _, version := range cases {
		if !modrinthDependencySelected(selected, version) {
			t.Fatalf("expected version to be selected: %#v", version)
		}
	}
	if modrinthDependencySelected(selected, domain.ModrinthVersion{ID: "missing"}) {
		t.Fatal("unexpected selected dependency")
	}
}

func TestModrinthUpdateAvailableComparesVersionThenFile(t *testing.T) {
	installed := domain.ModrinthInstalledProject{
		VersionID: "version-1",
		FileName:  "old.jar",
	}
	if modrinthUpdateAvailable(installed, domain.ModrinthVersion{ID: "version-1", File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("same version id should be up to date")
	}
	if !modrinthUpdateAvailable(installed, domain.ModrinthVersion{ID: "version-2", File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("different version id should need update")
	}
	installed.VersionID = ""
	if !modrinthUpdateAvailable(installed, domain.ModrinthVersion{File: domain.ModrinthVersionFile{FileName: "new.jar"}}) {
		t.Fatal("different file should need update when version id is missing")
	}
}

func TestLauncherLogsRecordAndListNewestFirst(t *testing.T) {
	app := NewApp()
	app.initServices(context.Background(), t.TempDir())

	first, err := app.RecordLauncherLog(domain.LauncherLog{
		Level:   "info",
		Source:  "Test",
		Message: "first",
	})
	if err != nil {
		t.Fatalf("record first launcher log: %v", err)
	}
	second, err := app.RecordLauncherLog(domain.LauncherLog{
		Level:   "success",
		Source:  "Test",
		Message: "second",
	})
	if err != nil {
		t.Fatalf("record second launcher log: %v", err)
	}

	logs, err := app.ListLauncherLogs()
	if err != nil {
		t.Fatalf("list launcher logs: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].ID != second.ID || logs[1].ID != first.ID {
		t.Fatalf("logs are not newest-first: %#v", logs)
	}
	if logs[0].Time == "" || logs[0].Level != "success" {
		t.Fatalf("log was not normalized: %#v", logs[0])
	}
}

func TestLauncherLogsClear(t *testing.T) {
	app := NewApp()
	app.initServices(context.Background(), t.TempDir())

	if _, err := app.RecordLauncherLog(domain.LauncherLog{Message: "entry"}); err != nil {
		t.Fatalf("record launcher log: %v", err)
	}
	if err := app.ClearLauncherLogs(); err != nil {
		t.Fatalf("clear launcher logs: %v", err)
	}
	logs, err := app.ListLauncherLogs()
	if err != nil {
		t.Fatalf("list launcher logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected empty logs after clear, got %#v", logs)
	}
}

func TestLauncherExecutablePrefersAppImagePath(t *testing.T) {
	t.Setenv("APPIMAGE", "/tmp/power-mine.AppImage")

	path, err := launcherExecutable()
	if err != nil {
		t.Fatalf("launcherExecutable returned error: %v", err)
	}
	if path != "/tmp/power-mine.AppImage" {
		t.Fatalf("expected APPIMAGE path, got %q", path)
	}
}
