package javasvc

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseJavaVersion(t *testing.T) {
	tests := map[string]string{
		`java version "21.0.5" 2024-10-15 LTS`: "21.0.5",
		`openjdk version "17.0.12" 2024-07-16`: "17.0.12",
		`openjdk 23.0.1 2024-10-15`:            "23.0.1",
		`some unexpected version output`:       "",
	}

	for input, want := range tests {
		if got := parseJavaVersion(input); got != want {
			t.Fatalf("parseJavaVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMajorVersion(t *testing.T) {
	tests := map[string]int{
		"1.8.0_482":       8,
		"8.0.482":         8,
		"17.0.12":         17,
		"21.0.11+10-LTS":  21,
		"unexpected text": 0,
	}

	for input, want := range tests {
		if got := MajorVersion(input); got != want {
			t.Fatalf("MajorVersion(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestCompatibleMajor(t *testing.T) {
	if !CompatibleMajor(8, 8) {
		t.Fatal("Java 8 should satisfy Java 8 requirement")
	}
	if CompatibleMajor(21, 8) {
		t.Fatal("Java 21 should not satisfy legacy Java 8 requirement")
	}
	if !CompatibleMajor(21, 17) {
		t.Fatal("Java 21 should satisfy Java 17 requirement")
	}
}

func TestAdoptiumPlatformMapping(t *testing.T) {
	if got, err := adoptiumOS("darwin"); err != nil || got != "mac" {
		t.Fatalf("adoptiumOS(darwin) = %q, %v; want mac, nil", got, err)
	}
	if got, err := adoptiumOS("linux"); err != nil || got != "linux" {
		t.Fatalf("adoptiumOS(linux) = %q, %v; want linux, nil", got, err)
	}
	if got, err := adoptiumArch("amd64"); err != nil || got != "x64" {
		t.Fatalf("adoptiumArch(amd64) = %q, %v; want x64, nil", got, err)
	}
	if got, err := adoptiumArch("arm64"); err != nil || got != "aarch64" {
		t.Fatalf("adoptiumArch(arm64) = %q, %v; want aarch64, nil", got, err)
	}
}

func TestStripPathComponentsRejectsUnsafePaths(t *testing.T) {
	got, ok := stripPathComponents("jdk-21/bin/java", 1)
	if !ok || got != filepath.Join("bin", "java") {
		t.Fatalf("stripPathComponents returned %q, %v; want bin/java, true", got, ok)
	}
	if _, ok := stripPathComponents("../jdk/bin/java", 1); ok {
		t.Fatal("stripPathComponents should reject parent traversal")
	}
	if _, ok := stripPathComponents("/jdk/bin/java", 1); ok {
		t.Fatal("stripPathComponents should reject absolute paths")
	}
}

func TestFindJavaExecutableFindsNestedRuntime(t *testing.T) {
	root := t.TempDir()
	javaPath := filepath.Join(root, "Contents", "Home", "bin", "java")
	if err := os.MkdirAll(filepath.Dir(javaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(javaPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findJavaExecutable(root)
	if err != nil {
		t.Fatalf("findJavaExecutable returned error: %v", err)
	}
	if got != javaPath {
		t.Fatalf("findJavaExecutable = %q, want %q", got, javaPath)
	}
}

func TestScaledPercent(t *testing.T) {
	if got := scaledPercent(50, 100, 10, 72); got != 46 {
		t.Fatalf("scaledPercent = %d, want 46", got)
	}
	if got := scaledPercent(100, 100, 10, 72); got != 82 {
		t.Fatalf("scaledPercent complete = %d, want 82", got)
	}
}
