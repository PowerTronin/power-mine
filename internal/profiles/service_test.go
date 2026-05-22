package profiles

import (
	"errors"
	"path/filepath"
	"testing"

	"power-mine/internal/domain"
)

func TestCreateProfileUsesDefaultsAndPersists(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)
	defaults := domain.MemorySettings{MinMB: 1024, MaxMB: 4096}

	profile, err := service.Create(domain.ProfileInput{
		Name:             "Fabric Survival",
		MinecraftVersion: "1.21.5",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric, Version: "0.16.14"},
	}, defaults)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if profile.ID == "" {
		t.Fatal("profile ID is empty")
	}
	if profile.Memory != defaults {
		t.Fatalf("profile memory = %#v, want %#v", profile.Memory, defaults)
	}
	wantGameDir := filepath.Join(dataDir, "instances", profile.ID, "minecraft")
	if profile.GameDir != wantGameDir {
		t.Fatalf("profile GameDir = %q, want %q", profile.GameDir, wantGameDir)
	}

	list, err := NewService(dataDir).List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if list.SelectedProfileID != profile.ID {
		t.Fatalf("selected profile = %q, want %q", list.SelectedProfileID, profile.ID)
	}
	if len(list.Profiles) != 1 {
		t.Fatalf("profile count = %d, want 1", len(list.Profiles))
	}
}

func TestCreateProfileValidatesRequiredFields(t *testing.T) {
	service := NewService(t.TempDir())

	_, err := service.Create(domain.ProfileInput{
		Name:             " ",
		MinecraftVersion: "1.21.5",
		Loader:           domain.LoaderConfig{Type: domain.LoaderVanilla},
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 4096},
	}, domain.MemorySettings{})
	if !errors.Is(err, ErrInvalidProfile) {
		t.Fatalf("Create error = %v, want ErrInvalidProfile", err)
	}
}

func TestDeleteSelectedProfileSelectsNext(t *testing.T) {
	service := NewService(t.TempDir())
	defaults := domain.MemorySettings{MinMB: 1024, MaxMB: 4096}

	first, err := service.Create(domain.ProfileInput{
		Name:             "One",
		MinecraftVersion: "1.21.5",
		Loader:           domain.LoaderConfig{Type: domain.LoaderVanilla},
	}, defaults)
	if err != nil {
		t.Fatalf("Create first returned error: %v", err)
	}
	second, err := service.Create(domain.ProfileInput{
		Name:             "Two",
		MinecraftVersion: "1.21.5",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric, Version: "0.16.14"},
	}, defaults)
	if err != nil {
		t.Fatalf("Create second returned error: %v", err)
	}

	if err := service.Delete(first.ID); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	list, err := service.List()
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if list.SelectedProfileID != second.ID {
		t.Fatalf("selected profile = %q, want %q", list.SelectedProfileID, second.ID)
	}
}
