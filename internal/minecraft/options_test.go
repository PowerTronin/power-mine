package minecraft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetPauseOnLostFocusCreatesOptions(t *testing.T) {
	gameDir := t.TempDir()
	changed, err := SetPauseOnLostFocus(gameDir, false)
	if err != nil {
		t.Fatalf("SetPauseOnLostFocus returned error: %v", err)
	}
	if !changed {
		t.Fatal("SetPauseOnLostFocus should report changed for a new options file")
	}
	raw, err := os.ReadFile(filepath.Join(gameDir, "options.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "pauseOnLostFocus:false\n" {
		t.Fatalf("options.txt = %q", string(raw))
	}
}

func TestSetPauseOnLostFocusUpdatesExistingOption(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "options.txt")
	if err := os.WriteFile(path, []byte("fov:0.5\npauseOnLostFocus:true\nlang:en_us\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := SetPauseOnLostFocus(gameDir, false)
	if err != nil {
		t.Fatalf("SetPauseOnLostFocus returned error: %v", err)
	}
	if !changed {
		t.Fatal("SetPauseOnLostFocus should report changed")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "pauseOnLostFocus:false\n") {
		t.Fatalf("options.txt missing updated pauseOnLostFocus: %q", got)
	}
	if !strings.Contains(got, "fov:0.5\n") || !strings.Contains(got, "lang:en_us\n") {
		t.Fatalf("options.txt did not preserve unrelated options: %q", got)
	}
}

func TestSetPauseOnLostFocusUnchanged(t *testing.T) {
	gameDir := t.TempDir()
	path := filepath.Join(gameDir, "options.txt")
	if err := os.WriteFile(path, []byte("pauseOnLostFocus:false\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := SetPauseOnLostFocus(gameDir, false)
	if err != nil {
		t.Fatalf("SetPauseOnLostFocus returned error: %v", err)
	}
	if changed {
		t.Fatal("SetPauseOnLostFocus should not report changed")
	}
}
