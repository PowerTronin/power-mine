package servers

import (
	"path/filepath"
	"testing"

	"power-mine/internal/domain"
)

func TestCreateLocalServerUsesDefaultDirectoryAndPort(t *testing.T) {
	service := NewService(t.TempDir())

	server, err := service.Create(domain.LocalServerInput{
		Name:             "Local Test Server",
		MinecraftVersion: "1.20.1",
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
		EulaAccepted:     true,
	}, domain.MemorySettings{MinMB: 512, MaxMB: 1024})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if server.Port != 25565 {
		t.Fatalf("Port = %d, want 25565", server.Port)
	}
	if filepath.Base(server.ServerDir) != "local-test-server" {
		t.Fatalf("ServerDir = %q, want slug suffix local-test-server", server.ServerDir)
	}
	if server.Install.Status != "not-installed" {
		t.Fatalf("Install status = %q, want not-installed", server.Install.Status)
	}
}

func TestCreateLocalServerAvoidsExistingDirectory(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)

	first, err := service.Create(domain.LocalServerInput{
		Name:             "Local Server",
		MinecraftVersion: "1.20.1",
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
		Port:             25565,
	}, domain.MemorySettings{})
	if err != nil {
		t.Fatalf("Create first returned error: %v", err)
	}
	second, err := service.Create(domain.LocalServerInput{
		Name:             "Local Server",
		MinecraftVersion: "1.20.1",
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
		Port:             25566,
	}, domain.MemorySettings{})
	if err != nil {
		t.Fatalf("Create second returned error: %v", err)
	}

	if first.ServerDir == second.ServerDir {
		t.Fatal("expected unique server directories")
	}
	if filepath.Base(second.ServerDir) != "local-server-2" {
		t.Fatalf("second ServerDir = %q, want suffix local-server-2", second.ServerDir)
	}
}

func TestCreateLocalServerValidatesPort(t *testing.T) {
	service := NewService(t.TempDir())

	_, err := service.Create(domain.LocalServerInput{
		Name:             "Bad Server",
		MinecraftVersion: "1.20.1",
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
		Port:             70000,
	}, domain.MemorySettings{})
	if err == nil {
		t.Fatal("expected invalid port error")
	}
}
