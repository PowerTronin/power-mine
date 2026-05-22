package storage

import (
	"os"
	"path/filepath"
	"testing"
)

type sampleConfig struct {
	Name string `json:"name"`
	Port int    `json:"port"`
}

func TestReadJSONReturnsFallbackWhenMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	fallback := sampleConfig{Name: "default", Port: 7}

	got, err := ReadJSON(path, fallback)
	if err != nil {
		t.Fatalf("ReadJSON returned error: %v", err)
	}
	if got != fallback {
		t.Fatalf("ReadJSON = %#v, want %#v", got, fallback)
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := sampleConfig{Name: "power", Port: 25565}

	if err := WriteJSON(path, want); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	got, err := ReadJSON(path, sampleConfig{})
	if err != nil {
		t.Fatalf("ReadJSON returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ReadJSON = %#v, want %#v", got, want)
	}
}

func TestWriteJSONDoesNotLeavePartFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	if err := WriteJSON(path, sampleConfig{Name: "power"}); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".part" {
			t.Fatalf("unexpected temporary file left behind: %s", entry.Name())
		}
	}
}
