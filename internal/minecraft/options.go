package minecraft

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetGameOption writes a Minecraft options.txt key in the profile game directory.
func SetGameOption(gameDir string, key string, value string) (bool, error) {
	gameDir = strings.TrimSpace(gameDir)
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if gameDir == "" {
		return false, fmt.Errorf("game directory is required")
	}
	if key == "" {
		return false, fmt.Errorf("option key is required")
	}
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		return false, err
	}

	path := filepath.Join(gameDir, "options.txt")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	prefix := key + ":"
	nextLine := prefix + value
	found := false
	changed := false
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			found = true
			if line != nextLine {
				lines[index] = nextLine
				changed = true
			}
		}
	}
	if !found {
		lines = append(lines, nextLine)
		changed = true
	}
	if !changed {
		return false, nil
	}

	nextContent := strings.Join(lines, "\n") + "\n"
	return true, os.WriteFile(path, []byte(nextContent), 0o644)
}

func SetPauseOnLostFocus(gameDir string, pause bool) (bool, error) {
	value := "false"
	if pause {
		value = "true"
	}
	return SetGameOption(gameDir, "pauseOnLostFocus", value)
}
