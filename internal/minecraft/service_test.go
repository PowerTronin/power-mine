package minecraft

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLibraryAllowedWithoutRules(t *testing.T) {
	if !libraryAllowed(nil) {
		t.Fatal("library without rules should be allowed")
	}
}

func TestLibraryAllowedHonorsCurrentOSRules(t *testing.T) {
	if !libraryAllowed([]rule{{Action: "allow", OS: &ruleOS{Name: minecraftOSName()}}}) {
		t.Fatal("matching allow rule should allow library")
	}
	if libraryAllowed([]rule{{Action: "allow", OS: &ruleOS{Name: "windows"}}}) {
		t.Fatal("non-matching allow rule should not allow library")
	}
}

func TestNativeClassifierUsesMinecraftOSName(t *testing.T) {
	library := libraryMetadata{
		Natives: map[string]string{
			minecraftOSName(): "natives-${arch}",
		},
	}
	got, ok := nativeClassifier(library)
	if !ok {
		t.Fatal("expected native classifier")
	}
	if runtime.GOARCH == "amd64" && got != "natives-64" {
		t.Fatalf("native classifier = %q, want natives-64", got)
	}
}

func TestInferRequiredJavaVersion(t *testing.T) {
	tests := map[string]int{
		"1.5.2":  8,
		"1.16.5": 8,
		"1.17.1": 16,
		"1.18.2": 17,
		"1.20.4": 17,
		"1.20.5": 21,
		"1.21.5": 21,
	}

	for versionID, want := range tests {
		if got := InferRequiredJavaVersion(versionID); got != want {
			t.Fatalf("InferRequiredJavaVersion(%q) = %d, want %d", versionID, got, want)
		}
	}
}

func TestMavenArtifactPath(t *testing.T) {
	got, ok := mavenArtifactPath("net.fabricmc:fabric-loader:0.18.0")
	if !ok {
		t.Fatal("mavenArtifactPath should parse Fabric loader coordinate")
	}
	want := "net/fabricmc/fabric-loader/0.18.0/fabric-loader-0.18.0.jar"
	if got != want {
		t.Fatalf("mavenArtifactPath = %q, want %q", got, want)
	}

	got, ok = mavenArtifactPath("group.example:artifact:1.0:client")
	if !ok {
		t.Fatal("mavenArtifactPath should parse classifier coordinate")
	}
	want = "group/example/artifact/1.0/artifact-1.0-client.jar"
	if got != want {
		t.Fatalf("mavenArtifactPath classifier = %q, want %q", got, want)
	}
}

func TestMavenArtifactPathWithExtension(t *testing.T) {
	got, ok := mavenArtifactPathFromSpec("net.minecraftforge:forge:1.20.1-47.4.20:installer")
	if !ok {
		t.Fatal("mavenArtifactPathFromSpec should parse Forge installer coordinate")
	}
	want := "net/minecraftforge/forge/1.20.1-47.4.20/forge-1.20.1-47.4.20-installer.jar"
	if got != want {
		t.Fatalf("Forge installer path = %q, want %q", got, want)
	}

	got, ok = mavenArtifactPathFromSpec("net.minecraft:client:1.20.1-20230612.114412:mappings@txt")
	if !ok {
		t.Fatal("mavenArtifactPathFromSpec should parse non-jar classifier coordinate")
	}
	want = "net/minecraft/client/1.20.1-20230612.114412/client-1.20.1-20230612.114412-mappings.txt"
	if got != want {
		t.Fatalf("mapping path = %q, want %q", got, want)
	}
}

func TestForgeVersionID(t *testing.T) {
	if got := ForgeVersionID("1.20.1", "1.20.1-47.4.20"); got != "1.20.1-forge-47.4.20" {
		t.Fatalf("ForgeVersionID full version = %q", got)
	}
	if got := ForgeVersionID("1.20.1", "latest"); got != "1.20.1-forge-latest" {
		t.Fatalf("ForgeVersionID latest = %q", got)
	}
}

func TestNeoForgeVersionID(t *testing.T) {
	if got := NeoForgeVersionID("1.21.1", "21.1.207"); got != "1.21.1-neoforge-21.1.207" {
		t.Fatalf("NeoForgeVersionID version = %q", got)
	}
	if got := NeoForgeVersionID("1.21.1", "latest"); got != "1.21.1-neoforge-latest" {
		t.Fatalf("NeoForgeVersionID latest = %q", got)
	}
}

func TestForgeProcessorResolverKeepsAbsolutePaths(t *testing.T) {
	service := NewService(t.TempDir())
	minecraftJar := filepath.Join(service.minecraftDir(), "versions", "1.20.1", "1.20.1.jar")
	got := service.resolveForgeProcessorValue("{MINECRAFT_JAR}", "1.20.1-47.4.20", map[string]string{
		"MINECRAFT_JAR": minecraftJar,
	})
	if got != minecraftJar {
		t.Fatalf("processor path = %q, want %q", got, minecraftJar)
	}

	dataPath := service.resolveForgeDataValue("1.20.1-47.4.20", "/data/client.lzma")
	if !strings.Contains(dataPath, "installer-data") || !strings.HasSuffix(dataPath, filepath.Join("data", "client.lzma")) {
		t.Fatalf("installer data path = %q", dataPath)
	}

	neoForgeDataPath := service.resolveForgeLikeDataValue("21.1.207", "/data/client.lzma", neoForgeInstallerProvider)
	if !strings.Contains(neoForgeDataPath, filepath.Join("net", "neoforged", "neoforge", "21.1.207", "installer-data")) {
		t.Fatalf("NeoForge installer data path = %q", neoForgeDataPath)
	}
}

func TestNormalizeLibraryDownloads(t *testing.T) {
	libraries := []libraryMetadata{{
		Name: "net.fabricmc:fabric-loader:0.18.0",
		URL:  "https://maven.fabricmc.net/",
		SHA1: "abc123",
		Size: 42,
	}}

	normalizeLibraryDownloads(libraries)
	artifact := libraries[0].Downloads.Artifact
	if artifact.Path != "net/fabricmc/fabric-loader/0.18.0/fabric-loader-0.18.0.jar" {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
	if artifact.URL != "https://maven.fabricmc.net/net/fabricmc/fabric-loader/0.18.0/fabric-loader-0.18.0.jar" {
		t.Fatalf("artifact url = %q", artifact.URL)
	}
	if artifact.SHA1 != "abc123" || artifact.Size != 42 {
		t.Fatalf("artifact verification = %#v", artifact)
	}
}
