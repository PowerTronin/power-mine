package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"power-mine/internal/domain"
)

const maxLauncherLogs = 2000

func (a *App) IsLogsWindow() bool {
	return a.logsWindow
}

func (a *App) OpenLauncherLogsWindow() error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	if a.logsWindow {
		return nil
	}
	executable, err := launcherExecutable()
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	command := exec.Command(executable, "--logs-window")
	command.Dir = filepath.Dir(executable)
	if err := command.Start(); err != nil {
		return fmt.Errorf("opening logs window: %w", err)
	}
	return nil
}

func launcherExecutable() (string, error) {
	if appImagePath := strings.TrimSpace(os.Getenv("APPIMAGE")); appImagePath != "" {
		return appImagePath, nil
	}
	return os.Executable()
}

func (a *App) ListLauncherLogs() ([]domain.LauncherLog, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	a.launcherLogMu.Lock()
	defer a.launcherLogMu.Unlock()
	return readLauncherLogFile(a.launcherLogPath, maxLauncherLogs)
}

func (a *App) RecordLauncherLog(entry domain.LauncherLog) (domain.LauncherLog, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LauncherLog{}, err
	}
	entry = normalizeLauncherLog(entry)

	a.launcherLogMu.Lock()
	defer a.launcherLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(a.launcherLogPath), 0o755); err != nil {
		return domain.LauncherLog{}, fmt.Errorf("creating launcher log directory: %w", err)
	}
	file, err := os.OpenFile(a.launcherLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return domain.LauncherLog{}, fmt.Errorf("opening launcher log file: %w", err)
	}
	defer file.Close()

	encoded, err := json.Marshal(entry)
	if err != nil {
		return domain.LauncherLog{}, fmt.Errorf("encoding launcher log entry: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return domain.LauncherLog{}, fmt.Errorf("writing launcher log entry: %w", err)
	}
	return entry, nil
}

func (a *App) ClearLauncherLogs() error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	a.launcherLogMu.Lock()
	defer a.launcherLogMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(a.launcherLogPath), 0o755); err != nil {
		return fmt.Errorf("creating launcher log directory: %w", err)
	}
	if err := os.WriteFile(a.launcherLogPath, nil, 0o644); err != nil {
		return fmt.Errorf("clearing launcher logs: %w", err)
	}
	return nil
}

func readLauncherLogFile(path string, limit int) ([]domain.LauncherLog, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening launcher log file: %w", err)
	}
	defer file.Close()

	var entries []domain.LauncherLog
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry domain.LauncherLog
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		entry = normalizeLauncherLog(entry)
		entries = append(entries, entry)
		if limit > 0 && len(entries) > limit {
			entries = entries[len(entries)-limit:]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading launcher log file: %w", err)
	}

	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return entries, nil
}

func normalizeLauncherLog(entry domain.LauncherLog) domain.LauncherLog {
	now := time.Now()
	if strings.TrimSpace(entry.ID) == "" {
		entry.ID = newLauncherLogID(now)
	}
	if strings.TrimSpace(entry.Time) == "" {
		entry.Time = now.Format("15:04:05")
	}
	switch entry.Level {
	case "info", "success", "error":
	default:
		entry.Level = "info"
	}
	if strings.TrimSpace(entry.Source) == "" {
		entry.Source = "Launcher"
	}
	entry.Message = strings.TrimSpace(entry.Message)
	if entry.Message == "" {
		entry.Message = "No message"
	}
	return entry
}

func newLauncherLogID(now time.Time) string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("%d", now.UnixNano())
	}
	return fmt.Sprintf("%d-%s", now.UnixNano(), hex.EncodeToString(suffix[:]))
}
