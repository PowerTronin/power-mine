package minecraft

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"power-mine/internal/domain"
)

const maxGameLogReadBytes int64 = 512 * 1024

func (s *Service) ListGameLogs(profile domain.Profile) (domain.GameLogList, error) {
	files := make([]domain.GameLogFile, 0)
	logsDir := filepath.Join(profile.GameDir, "logs")
	entries := []struct {
		dir  string
		kind string
	}{
		{dir: logsDir, kind: "log"},
		{dir: filepath.Join(profile.GameDir, "crash-reports"), kind: "crash"},
	}

	for _, entry := range entries {
		items, err := os.ReadDir(entry.dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return domain.GameLogList{}, err
		}
		for _, item := range items {
			if item.IsDir() {
				continue
			}
			info, err := item.Info()
			if err != nil {
				continue
			}
			name := item.Name()
			if entry.kind == "log" && !isRegularLogName(name) {
				continue
			}
			if entry.kind == "crash" && !strings.HasSuffix(strings.ToLower(name), ".txt") {
				continue
			}
			relative := filepath.ToSlash(filepath.Join(filepath.Base(entry.dir), name))
			files = append(files, gameLogFile(relative, name, entry.kind, info))
		}
	}

	rootItems, err := os.ReadDir(profile.GameDir)
	if err == nil {
		for _, item := range rootItems {
			if item.IsDir() {
				continue
			}
			name := item.Name()
			if !strings.HasPrefix(name, "hs_err_pid") || !strings.HasSuffix(strings.ToLower(name), ".log") {
				continue
			}
			info, err := item.Info()
			if err != nil {
				continue
			}
			files = append(files, gameLogFile(name, name, "jvm", info))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.GameLogList{}, err
	}

	sort.SliceStable(files, func(i, j int) bool {
		return files[i].UpdatedAt > files[j].UpdatedAt
	})

	return domain.GameLogList{
		ProfileID: profile.ID,
		LogsDir:   logsDir,
		Files:     files,
	}, nil
}

func (s *Service) ReadGameLog(profile domain.Profile, fileName string) (domain.GameLogContent, error) {
	relative, kind, err := cleanGameLogName(fileName)
	if err != nil {
		return domain.GameLogContent{}, err
	}
	path := filepath.Join(profile.GameDir, filepath.FromSlash(relative))
	info, err := os.Stat(path)
	if err != nil {
		return domain.GameLogContent{}, err
	}
	if info.IsDir() {
		return domain.GameLogContent{}, fmt.Errorf("game log is a directory")
	}

	content, truncated, err := readGameLogContent(path, info.Size())
	if err != nil {
		return domain.GameLogContent{}, err
	}

	return domain.GameLogContent{
		ProfileID:   profile.ID,
		FileName:    relative,
		DisplayName: filepath.Base(relative),
		Kind:        kind,
		Content:     content,
		Size:        info.Size(),
		Truncated:   truncated,
		MaxBytes:    maxGameLogReadBytes,
	}, nil
}

func isRegularLogName(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".log") || strings.HasSuffix(lower, ".log.gz")
}

func gameLogFile(relative string, name string, kind string, info os.FileInfo) domain.GameLogFile {
	return domain.GameLogFile{
		FileName:    relative,
		DisplayName: name,
		Kind:        kind,
		Size:        info.Size(),
		UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339),
		Compressed:  strings.HasSuffix(strings.ToLower(name), ".gz"),
	}
}

func cleanGameLogName(fileName string) (string, string, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(fileName))
	if normalized == "" {
		return "", "", fmt.Errorf("game log file is required")
	}
	if filepath.IsAbs(normalized) {
		return "", "", fmt.Errorf("absolute game log paths are not allowed")
	}
	cleaned := filepath.ToSlash(filepath.Clean(normalized))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", "", fmt.Errorf("invalid game log path")
	}
	if strings.HasPrefix(cleaned, "logs/") && isRegularLogName(filepath.Base(cleaned)) {
		return cleaned, "log", nil
	}
	if strings.HasPrefix(cleaned, "crash-reports/") && strings.HasSuffix(strings.ToLower(filepath.Base(cleaned)), ".txt") {
		return cleaned, "crash", nil
	}
	if !strings.Contains(cleaned, "/") && strings.HasPrefix(cleaned, "hs_err_pid") && strings.HasSuffix(strings.ToLower(cleaned), ".log") {
		return cleaned, "jvm", nil
	}
	return "", "", fmt.Errorf("unsupported game log path")
}

func readGameLogContent(path string, size int64) (string, bool, error) {
	if strings.HasSuffix(strings.ToLower(path), ".gz") {
		return readCompressedGameLog(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	truncated := size > maxGameLogReadBytes
	if truncated {
		if _, err := file.Seek(-maxGameLogReadBytes, io.SeekEnd); err != nil {
			return "", false, err
		}
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxGameLogReadBytes+1))
	if err != nil {
		return "", false, err
	}
	if int64(len(raw)) > maxGameLogReadBytes {
		raw = raw[len(raw)-int(maxGameLogReadBytes):]
		truncated = true
	}
	return string(raw), truncated, nil
}

func readCompressedGameLog(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		return "", false, err
	}
	defer reader.Close()

	raw, err := io.ReadAll(io.LimitReader(reader, maxGameLogReadBytes+1))
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(raw)) > maxGameLogReadBytes
	if truncated {
		raw = raw[:maxGameLogReadBytes]
	}
	return string(raw), truncated, nil
}
