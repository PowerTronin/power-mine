package settings

import (
	"path/filepath"
	"testing"

	"power-mine/internal/domain"
)

func TestSettingsDefaultAndSave(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)

	initial, err := service.Get()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if initial.DataDir != dataDir {
		t.Fatalf("DataDir = %q, want %q", initial.DataDir, dataDir)
	}
	if initial.JavaPath != "java" {
		t.Fatalf("JavaPath = %q, want java", initial.JavaPath)
	}
	if initial.Account.Mode != domain.AccountOffline {
		t.Fatalf("Account.Mode = %q, want offline", initial.Account.Mode)
	}

	want := domain.Settings{
		DataDir:  dataDir,
		JavaPath: filepath.Join(dataDir, "bin", "java"),
		Account: domain.AccountConfig{
			Mode:        domain.AccountOffline,
			OfflineName: "Local_Player",
			OfflineUUID: "0",
		},
		DefaultMemory: domain.MemorySettings{
			MinMB: 2048,
			MaxMB: 6144,
		},
		Network: domain.NetworkSettings{
			RetryCount:       5,
			MetadataTTLHours: 12,
		},
	}

	saved, err := service.Save(want)
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	if saved.Account.OfflineName != "Local_Player" {
		t.Fatalf("OfflineName = %q, want Local_Player", saved.Account.OfflineName)
	}

	got, err := NewService(dataDir).Get()
	if err != nil {
		t.Fatalf("Get after save returned error: %v", err)
	}
	if got.Account != saved.Account {
		t.Fatalf("Account = %#v, want %#v", got.Account, saved.Account)
	}
}
