package minecraft

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"power-mine/internal/domain"
)

func TestResolveArgumentList(t *testing.T) {
	raw := mustRawMessages(t, `"--username"`, `"${auth_player_name}"`, `{"rules":[{"action":"allow","os":{"name":"linux"}}],"value":"linux-only"}`, `{"rules":[{"action":"allow","features":{"is_demo_user":true}}],"value":"demo"}`)
	got := resolveArgumentList(raw, map[string]string{"auth_player_name": "Player"})

	if strings.Join(got, " ") != "--username Player" && minecraftOSName() != "linux" {
		t.Fatalf("resolved args = %#v", got)
	}
	if minecraftOSName() == "linux" && strings.Join(got, " ") != "--username Player linux-only" {
		t.Fatalf("resolved linux args = %#v", got)
	}
}

func TestBuildVanillaLaunchCommand(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)
	versionDir := filepath.Join(dataDir, "minecraft", "versions", "1.21.5")
	libraryDir := filepath.Join(dataDir, "minecraft", "libraries", "com", "example", "lib", "1.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, "1.21.5.jar"), []byte("client"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libraryDir, "lib-1.0.jar"), []byte("library"), 0o644); err != nil {
		t.Fatal(err)
	}
	nativePath := filepath.Join(libraryDir, "lib-1.0-natives.jar")
	if err := writeZip(nativePath, "native.dylib", []byte("native")); err != nil {
		t.Fatal(err)
	}
	metadata := `{
		"id":"1.21.5",
		"type":"release",
		"mainClass":"net.minecraft.client.main.Main",
		"assets":"19",
		"assetIndex":{"id":"19"},
		"arguments":{
			"jvm":["-Djava.library.path=${natives_directory}","-cp","${classpath}"],
			"game":["--username","${auth_player_name}","--uuid","${auth_uuid}","--accessToken","${auth_access_token}","--gameDir","${game_directory}","--assetsDir","${assets_root}","--assetIndex","${assets_index_name}","--userType","${user_type}"]
		},
		"libraries":[{
			"name":"com.example:lib:1.0",
			"downloads":{
				"artifact":{"path":"com/example/lib/1.0/lib-1.0.jar"},
				"classifiers":{"natives-` + nativeClassifierForTest() + `":{"path":"com/example/lib/1.0/lib-1.0-natives.jar"}}
			},
			"natives":{"` + minecraftOSName() + `":"natives-${arch}"}
		}],
		"logging":{"client":{"argument":"-Dlog4j.configurationFile=${path}","file":{"id":"client-1.21.xml"}}}
	}`
	if err := os.WriteFile(filepath.Join(versionDir, "1.21.5.json"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	command, err := service.BuildVanillaLaunchCommand(context.Background(), domain.Profile{
		ID:               "profile",
		Name:             "Profile",
		MinecraftVersion: "1.21.5",
		Loader:           domain.LoaderConfig{Type: domain.LoaderVanilla},
		GameDir:          filepath.Join(dataDir, "instance"),
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
		Account:          domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Player", OfflineUUID: "00000000-0000-3000-8000-000000000000"},
	}, LaunchOptions{
		JavaPath: "java",
		Account:  domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Player", OfflineUUID: "00000000-0000-3000-8000-000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildVanillaLaunchCommand returned error: %v", err)
	}
	if command.JavaPath != "java" {
		t.Fatalf("JavaPath = %q, want java", command.JavaPath)
	}
	joined := strings.Join(command.Args, " ")
	if !strings.Contains(joined, "net.minecraft.client.main.Main") {
		t.Fatalf("args missing main class: %s", joined)
	}
	if !strings.Contains(joined, "--username Player") {
		t.Fatalf("args missing offline username: %s", joined)
	}
	if !strings.Contains(joined, "-Xmx2048M") {
		t.Fatalf("args missing memory: %s", joined)
	}
	if !strings.Contains(joined, "-Dlog4j.configurationFile=") {
		t.Fatalf("args missing log4j config: %s", joined)
	}
}

func TestBuildFabricLaunchCommandUsesFabricMetadataAndVanillaClientJar(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)
	baseVersionDir := filepath.Join(dataDir, "minecraft", "versions", "1.20.1")
	fabricVersionID := FabricVersionID("1.20.1", "0.18.0")
	fabricVersionDir := filepath.Join(dataDir, "minecraft", "versions", fabricVersionID)
	libraryDir := filepath.Join(dataDir, "minecraft", "libraries", "net", "fabricmc", "fabric-loader", "0.18.0")
	if err := os.MkdirAll(baseVersionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fabricVersionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseVersionDir, "1.20.1.jar"), []byte("client"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libraryDir, "fabric-loader-0.18.0.jar"), []byte("fabric"), 0o644); err != nil {
		t.Fatal(err)
	}

	metadata := `{
		"id":"` + fabricVersionID + `",
		"inheritsFrom":"1.20.1",
		"type":"release",
		"mainClass":"net.fabricmc.loader.impl.launch.knot.KnotClient",
		"assets":"5",
		"assetIndex":{"id":"5"},
		"arguments":{
			"jvm":["-Djava.library.path=${natives_directory}","-cp","${classpath}","-DFabricMcEmu= net.minecraft.client.main.Main "],
			"game":["--username","${auth_player_name}","--uuid","${auth_uuid}","--accessToken","${auth_access_token}","--gameDir","${game_directory}","--assetsDir","${assets_root}","--assetIndex","${assets_index_name}","--userType","${user_type}"]
		},
		"libraries":[{
			"name":"net.fabricmc:fabric-loader:0.18.0",
			"downloads":{"artifact":{"path":"net/fabricmc/fabric-loader/0.18.0/fabric-loader-0.18.0.jar"}}
		}]
	}`
	if err := os.WriteFile(filepath.Join(fabricVersionDir, fabricVersionID+".json"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	command, err := service.BuildLaunchCommand(context.Background(), domain.Profile{
		ID:               "profile",
		Name:             "Fabric",
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric, Version: "0.18.0"},
		GameDir:          filepath.Join(dataDir, "instance"),
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
	}, LaunchOptions{
		JavaPath: "java",
		Account:  domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Player", OfflineUUID: "00000000-0000-3000-8000-000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildLaunchCommand returned error: %v", err)
	}

	joined := strings.Join(command.Args, " ")
	if !strings.Contains(joined, "net.fabricmc.loader.impl.launch.knot.KnotClient") {
		t.Fatalf("args missing Fabric main class: %s", joined)
	}
	if !strings.Contains(joined, filepath.Join(baseVersionDir, "1.20.1.jar")) {
		t.Fatalf("classpath missing inherited vanilla client jar: %s", joined)
	}
	if !strings.Contains(joined, "fabric-loader-0.18.0.jar") {
		t.Fatalf("classpath missing Fabric loader jar: %s", joined)
	}
}

func TestBuildForgeLaunchCommandSkipsInheritedVanillaClientJar(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)
	forgeVersionID := ForgeVersionID("1.20.1", "47.4.20")
	forgeVersionDir := filepath.Join(dataDir, "minecraft", "versions", forgeVersionID)
	libraryDir := filepath.Join(dataDir, "minecraft", "libraries", "cpw", "mods", "bootstraplauncher", "1.1.2")
	if err := os.MkdirAll(forgeVersionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libraryDir, "bootstraplauncher-1.1.2.jar"), []byte("forge"), 0o644); err != nil {
		t.Fatal(err)
	}

	metadata := `{
		"id":"` + forgeVersionID + `",
		"inheritsFrom":"1.20.1",
		"type":"release",
		"mainClass":"cpw.mods.bootstraplauncher.BootstrapLauncher",
		"assets":"5",
		"assetIndex":{"id":"5"},
		"arguments":{
			"jvm":["-cp","${classpath}"],
			"game":["--username","${auth_player_name}"]
		},
		"libraries":[{
			"name":"cpw.mods:bootstraplauncher:1.1.2",
			"downloads":{"artifact":{"path":"cpw/mods/bootstraplauncher/1.1.2/bootstraplauncher-1.1.2.jar"}}
		}]
	}`
	if err := os.WriteFile(filepath.Join(forgeVersionDir, forgeVersionID+".json"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	command, err := service.BuildLaunchCommand(context.Background(), domain.Profile{
		ID:               "profile",
		Name:             "Forge",
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderForge, Version: "47.4.20"},
		GameDir:          filepath.Join(dataDir, "instance"),
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
	}, LaunchOptions{
		JavaPath: "java",
		Account:  domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Player", OfflineUUID: "00000000-0000-3000-8000-000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildLaunchCommand returned error: %v", err)
	}

	joined := strings.Join(command.Args, " ")
	if strings.Contains(joined, filepath.Join("versions", "1.20.1", "1.20.1.jar")) {
		t.Fatalf("Forge classpath should not include inherited vanilla client jar: %s", joined)
	}
	if !strings.Contains(joined, "bootstraplauncher-1.1.2.jar") {
		t.Fatalf("classpath missing Forge launcher library: %s", joined)
	}
}

func TestBuildNeoForgeLaunchCommandSkipsInheritedVanillaClientJar(t *testing.T) {
	dataDir := t.TempDir()
	service := NewService(dataDir)
	neoForgeVersionID := NeoForgeVersionID("1.21.1", "21.1.207")
	neoForgeVersionDir := filepath.Join(dataDir, "minecraft", "versions", neoForgeVersionID)
	libraryDir := filepath.Join(dataDir, "minecraft", "libraries", "cpw", "mods", "bootstraplauncher", "2.0.2")
	if err := os.MkdirAll(neoForgeVersionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libraryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libraryDir, "bootstraplauncher-2.0.2.jar"), []byte("neoforge"), 0o644); err != nil {
		t.Fatal(err)
	}

	metadata := `{
		"id":"` + neoForgeVersionID + `",
		"inheritsFrom":"1.21.1",
		"type":"release",
		"mainClass":"cpw.mods.bootstraplauncher.BootstrapLauncher",
		"assets":"5",
		"assetIndex":{"id":"5"},
		"arguments":{
			"jvm":["-cp","${classpath}"],
			"game":["--username","${auth_player_name}"]
		},
		"libraries":[{
			"name":"cpw.mods:bootstraplauncher:2.0.2",
			"downloads":{"artifact":{"path":"cpw/mods/bootstraplauncher/2.0.2/bootstraplauncher-2.0.2.jar"}}
		},{
			"name":"cpw.mods:bootstraplauncher:2.0.2",
			"downloads":{"artifact":{"path":"cpw/mods/bootstraplauncher/2.0.2/bootstraplauncher-2.0.2.jar"}}
		}]
	}`
	if err := os.WriteFile(filepath.Join(neoForgeVersionDir, neoForgeVersionID+".json"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}

	command, err := service.BuildLaunchCommand(context.Background(), domain.Profile{
		ID:               "profile",
		Name:             "NeoForge",
		MinecraftVersion: "1.21.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderNeoForge, Version: "21.1.207"},
		GameDir:          filepath.Join(dataDir, "instance"),
		Memory:           domain.MemorySettings{MinMB: 1024, MaxMB: 2048},
	}, LaunchOptions{
		JavaPath: "java",
		Account:  domain.AccountConfig{Mode: domain.AccountOffline, OfflineName: "Player", OfflineUUID: "00000000-0000-3000-8000-000000000000"},
	})
	if err != nil {
		t.Fatalf("BuildLaunchCommand returned error: %v", err)
	}

	joined := strings.Join(command.Args, " ")
	if strings.Contains(joined, filepath.Join("versions", "1.21.1", "1.21.1.jar")) {
		t.Fatalf("NeoForge classpath should not include inherited vanilla client jar: %s", joined)
	}
	if !strings.Contains(joined, "bootstraplauncher-2.0.2.jar") {
		t.Fatalf("classpath missing NeoForge launcher library: %s", joined)
	}
	if strings.Count(joined, "bootstraplauncher-2.0.2.jar") != 1 {
		t.Fatalf("classpath should dedupe duplicate NeoForge libraries: %s", joined)
	}
}

func mustRawMessages(t *testing.T, values ...string) []json.RawMessage {
	t.Helper()
	raw := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		raw = append(raw, json.RawMessage(value))
	}
	return raw
}

func nativeClassifierForTest() string {
	if minecraftOSName() == "osx" {
		return "64"
	}
	return "64"
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
