package main

import (
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
