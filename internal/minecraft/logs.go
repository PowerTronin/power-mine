package minecraft

import (
	"archive/zip"
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

func (s *Service) ExportGameLogs(profile domain.Profile, targetPath string, launcherEvents string) (domain.LogExportResult, error) {
	gameDir := strings.TrimSpace(profile.GameDir)
	if gameDir == "" {
		return domain.LogExportResult{}, fmt.Errorf("profile game directory is empty")
	}
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return domain.LogExportResult{}, fmt.Errorf("log export target is empty")
	}

	list, err := s.ListGameLogs(profile)
	if err != nil {
		return domain.LogExportResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return domain.LogExportResult{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".*.part")
	if err != nil {
		return domain.LogExportResult{}, err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	writer := zip.NewWriter(tmp)
	filesExported := 0
	for _, file := range list.Files {
		sourcePath := filepath.Join(gameDir, filepath.FromSlash(file.FileName))
		info, err := os.Stat(sourcePath)
		if err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return domain.LogExportResult{}, err
		}
		if info.IsDir() {
			continue
		}
		if err := writeLogZipFile(writer, sourcePath, "game/"+file.FileName, info); err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return domain.LogExportResult{}, err
		}
		filesExported++
	}

	launcherEvents = strings.TrimRight(launcherEvents, "\r\n")
	launcherEventsExported := launcherEvents != ""
	if launcherEventsExported {
		if err := writeLogZipBytes(writer, "launcher-events.log", []byte(launcherEvents+"\n")); err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return domain.LogExportResult{}, err
		}
	}

	manifest := fmt.Sprintf("Power Mine log export\nProfile: %s\nProfile ID: %s\nExported at: %s\nGame log files: %d\nLauncher events: %t\n",
		profile.Name,
		profile.ID,
		time.Now().UTC().Format(time.RFC3339),
		filesExported,
		launcherEventsExported,
	)
	if err := writeLogZipBytes(writer, "manifest.txt", []byte(manifest)); err != nil {
		_ = writer.Close()
		_ = tmp.Close()
		return domain.LogExportResult{}, err
	}

	if err := writer.Close(); err != nil {
		_ = tmp.Close()
		return domain.LogExportResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return domain.LogExportResult{}, err
	}
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Remove(targetPath); err != nil {
			return domain.LogExportResult{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.LogExportResult{}, err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return domain.LogExportResult{}, err
	}
	removeTemp = false

	return domain.LogExportResult{
		ProfileID:              profile.ID,
		Name:                   profile.Name,
		Path:                   targetPath,
		FilesExported:          filesExported,
		LauncherEventsExported: launcherEventsExported,
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

func writeLogZipFile(writer *zip.Writer, sourcePath string, archivePath string, info os.FileInfo) error {
	cleanPath, err := cleanLogArchivePath(archivePath)
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = cleanPath
	header.Method = zip.Deflate

	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(entry, file)
	return err
}

func writeLogZipBytes(writer *zip.Writer, archivePath string, body []byte) error {
	cleanPath, err := cleanLogArchivePath(archivePath)
	if err != nil {
		return err
	}
	header := &zip.FileHeader{Name: cleanPath, Method: zip.Deflate}
	header.SetModTime(time.Now().UTC())
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = entry.Write(body)
	return err
}

func cleanLogArchivePath(archivePath string) (string, error) {
	cleanPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(archivePath)))
	if cleanPath == "." || !filepath.IsLocal(filepath.FromSlash(cleanPath)) {
		return "", fmt.Errorf("invalid log archive path: %s", archivePath)
	}
	return cleanPath, nil
}
