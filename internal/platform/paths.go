package platform

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const (
	AppName        = "Power Mine"
	LinuxAppDir    = "power-mine"
	macOSAppSubdir = "Library/Application Support/Power Mine"
)

func AppDataDir() (string, error) {
	if dataDir := os.Getenv("POWER_MINE_DATA_DIR"); dataDir != "" {
		return filepath.Clean(dataDir), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, macOSAppSubdir), nil
	case "linux":
		if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
			return filepath.Join(dataHome, LinuxAppDir), nil
		}
		return filepath.Join(home, ".local", "share", LinuxAppDir), nil
	default:
		return "", errors.New("unsupported platform for Power Mine MVP")
	}
}

func ProfileGameDir(dataDir string, profileID string) string {
	return filepath.Join(dataDir, "instances", profileID, "minecraft")
}
