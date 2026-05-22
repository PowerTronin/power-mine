package modpacks

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"power-mine/internal/domain"
)

var ErrInvalidModpack = errors.New("invalid modpack")

type ProgressFunc func(domain.InstallProgress)

type Service struct {
	client *http.Client
}

func NewService() *Service {
	return &Service{client: &http.Client{Timeout: 10 * time.Minute}}
}

type Index struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary"`
	Files         []IndexFile       `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type IndexFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       *FileEnv          `json:"env,omitempty"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

type FileEnv struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

type InstallResult struct {
	FilesInstalled     int
	FilesSkipped       int
	OverridesInstalled int
}

type ExportOptions struct {
	Files []ExportFile
}

type ExportFile struct {
	Path      string
	Downloads []string
	SHA1      string
	SHA512    string
	FileSize  int64
}

type ExportResult struct {
	VersionID         string
	FilesExported     int
	OverridesExported int
}

func (s *Service) ReadIndex(packPath string) (Index, error) {
	reader, err := zip.OpenReader(packPath)
	if err != nil {
		return Index{}, err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.Name != "modrinth.index.json" {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return Index{}, err
		}
		defer body.Close()

		var index Index
		if err := json.NewDecoder(body).Decode(&index); err != nil {
			return Index{}, err
		}
		if err := validateIndex(index); err != nil {
			return Index{}, err
		}
		return index, nil
	}
	return Index{}, fmt.Errorf("%w: modrinth.index.json was not found", ErrInvalidModpack)
}

func ProfileInput(index Index, memory domain.MemorySettings) (domain.ProfileInput, error) {
	if err := validateIndex(index); err != nil {
		return domain.ProfileInput{}, err
	}
	minecraftVersion := strings.TrimSpace(index.Dependencies["minecraft"])
	if minecraftVersion == "" {
		return domain.ProfileInput{}, fmt.Errorf("%w: minecraft dependency is required", ErrInvalidModpack)
	}

	loader := domain.LoaderConfig{Type: domain.LoaderVanilla}
	for _, candidate := range []struct {
		key        string
		loaderType domain.LoaderType
	}{
		{key: "fabric-loader", loaderType: domain.LoaderFabric},
		{key: "quilt-loader", loaderType: domain.LoaderQuilt},
		{key: "forge", loaderType: domain.LoaderForge},
		{key: "neoforge", loaderType: domain.LoaderNeoForge},
	} {
		version := strings.TrimSpace(index.Dependencies[candidate.key])
		if version == "" {
			continue
		}
		if loader.Type != domain.LoaderVanilla {
			return domain.ProfileInput{}, fmt.Errorf("%w: multiple mod loaders are not supported in one pack", ErrInvalidModpack)
		}
		loader = domain.LoaderConfig{Type: candidate.loaderType, Version: version}
	}

	name := strings.TrimSpace(index.Name)
	if name == "" {
		name = "Imported Modpack"
	}

	return domain.ProfileInput{
		Name:             name,
		MinecraftVersion: minecraftVersion,
		Loader:           loader,
		Memory:           memory,
	}, nil
}

func (s *Service) InstallClient(ctx context.Context, packPath string, profile domain.Profile, progress ProgressFunc) (InstallResult, error) {
	reader, err := zip.OpenReader(packPath)
	if err != nil {
		return InstallResult{}, err
	}
	defer reader.Close()

	index, err := readIndex(reader.File)
	if err != nil {
		return InstallResult{}, err
	}
	if err := validateIndex(index); err != nil {
		return InstallResult{}, err
	}

	files := clientFiles(index.Files)
	overrides := overrideFiles(reader.File)
	total := len(files) + len(overrides)
	current := 0
	result := InstallResult{}

	for _, file := range files {
		current++
		emit(progress, profile.ID, domain.InstallProgress{
			Stage:   "modpack-download",
			Message: "Downloading " + file.Path,
			Current: current,
			Total:   total,
			Percent: percent(current, total),
		})
		installed, err := s.installIndexFile(ctx, profile.GameDir, file)
		if err != nil {
			return result, err
		}
		if installed {
			result.FilesInstalled++
		} else {
			result.FilesSkipped++
		}
	}

	for _, file := range overrides {
		current++
		emit(progress, profile.ID, domain.InstallProgress{
			Stage:   "modpack-overrides",
			Message: "Applying " + file.targetPath,
			Current: current,
			Total:   total,
			Percent: percent(current, total),
		})
		if err := extractOverride(profile.GameDir, file); err != nil {
			return result, err
		}
		result.OverridesInstalled++
	}

	emit(progress, profile.ID, domain.InstallProgress{
		Stage:   "modpack-complete",
		Message: "Modpack files imported",
		Current: total,
		Total:   total,
		Percent: 100,
		Done:    true,
	})
	return result, nil
}

func (s *Service) Export(profile domain.Profile, targetPath string, options ExportOptions) (ExportResult, error) {
	gameDir := strings.TrimSpace(profile.GameDir)
	if gameDir == "" {
		return ExportResult{}, fmt.Errorf("%w: profile game directory is empty", ErrInvalidModpack)
	}
	if strings.TrimSpace(targetPath) == "" {
		return ExportResult{}, fmt.Errorf("%w: export target is empty", ErrInvalidModpack)
	}

	files, remotePaths, err := exportIndexFiles(options.Files)
	if err != nil {
		return ExportResult{}, err
	}
	overrides, err := exportOverridePaths(gameDir, remotePaths)
	if err != nil {
		return ExportResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return ExportResult{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".*.part")
	if err != nil {
		return ExportResult{}, err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	writer := zip.NewWriter(tmp)
	index := exportIndex(profile)
	index.Files = files
	indexBody, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		_ = writer.Close()
		_ = tmp.Close()
		return ExportResult{}, err
	}
	if err := writeZipBytes(writer, "modrinth.index.json", indexBody); err != nil {
		_ = writer.Close()
		_ = tmp.Close()
		return ExportResult{}, err
	}

	for _, override := range overrides {
		if err := writeZipFile(writer, override.sourcePath, "overrides/"+filepath.ToSlash(override.relativePath)); err != nil {
			_ = writer.Close()
			_ = tmp.Close()
			return ExportResult{}, err
		}
	}

	if err := writer.Close(); err != nil {
		_ = tmp.Close()
		return ExportResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return ExportResult{}, err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return ExportResult{}, err
	}
	removeTemp = false
	return ExportResult{VersionID: index.VersionID, FilesExported: len(files), OverridesExported: len(overrides)}, nil
}

func validateIndex(index Index) error {
	if index.FormatVersion != 1 {
		return fmt.Errorf("%w: unsupported format version %d", ErrInvalidModpack, index.FormatVersion)
	}
	if strings.TrimSpace(index.Game) != "minecraft" {
		return fmt.Errorf("%w: unsupported game %q", ErrInvalidModpack, index.Game)
	}
	return nil
}

func readIndex(files []*zip.File) (Index, error) {
	for _, file := range files {
		if file.Name != "modrinth.index.json" {
			continue
		}
		body, err := file.Open()
		if err != nil {
			return Index{}, err
		}
		defer body.Close()

		var index Index
		if err := json.NewDecoder(body).Decode(&index); err != nil {
			return Index{}, err
		}
		return index, nil
	}
	return Index{}, fmt.Errorf("%w: modrinth.index.json was not found", ErrInvalidModpack)
}

func clientFiles(files []IndexFile) []IndexFile {
	result := make([]IndexFile, 0, len(files))
	for _, file := range files {
		if file.Env != nil && strings.EqualFold(file.Env.Client, "unsupported") {
			continue
		}
		result = append(result, file)
	}
	return result
}

type overrideFile struct {
	file       *zip.File
	targetPath string
}

type exportOverridePath struct {
	sourcePath   string
	relativePath string
}

func overrideFiles(files []*zip.File) []overrideFile {
	var result []overrideFile
	for _, prefix := range []string{"overrides/", "client-overrides/"} {
		for _, file := range files {
			if file.FileInfo().IsDir() {
				continue
			}
			if relative, ok := strings.CutPrefix(file.Name, prefix); ok && relative != "" {
				result = append(result, overrideFile{file: file, targetPath: relative})
			}
		}
	}
	return result
}

func exportIndex(profile domain.Profile) Index {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = "Power Mine Modpack"
	}
	versionID := time.Now().UTC().Format("20060102-150405")
	dependencies := map[string]string{}
	if minecraftVersion := strings.TrimSpace(profile.MinecraftVersion); minecraftVersion != "" {
		dependencies["minecraft"] = minecraftVersion
	}
	if loaderVersion := strings.TrimSpace(profile.Loader.Version); loaderVersion != "" {
		switch profile.Loader.Type {
		case domain.LoaderFabric:
			dependencies["fabric-loader"] = loaderVersion
		case domain.LoaderQuilt:
			dependencies["quilt-loader"] = loaderVersion
		case domain.LoaderForge:
			dependencies["forge"] = loaderVersion
		case domain.LoaderNeoForge:
			dependencies["neoforge"] = loaderVersion
		}
	}

	return Index{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     versionID,
		Name:          name,
		Summary:       "Exported from Power Mine",
		Files:         []IndexFile{},
		Dependencies:  dependencies,
	}
}

func exportIndexFiles(files []ExportFile) ([]IndexFile, map[string]bool, error) {
	result := make([]IndexFile, 0, len(files))
	remotePaths := make(map[string]bool, len(files))
	seen := make(map[string]bool, len(files))

	for _, file := range files {
		path, err := cleanExportPath(file.Path)
		if err != nil {
			return nil, nil, err
		}
		if seen[path] {
			continue
		}
		downloads := exportDownloads(file.Downloads)
		if len(downloads) == 0 {
			return nil, nil, fmt.Errorf("%w: %s has no downloads", ErrInvalidModpack, path)
		}
		for _, download := range downloads {
			if err := validateDownloadURL(download); err != nil {
				return nil, nil, err
			}
		}
		hashes := map[string]string{}
		if sha1 := strings.ToLower(strings.TrimSpace(file.SHA1)); sha1 != "" {
			hashes["sha1"] = sha1
		}
		if sha512 := strings.ToLower(strings.TrimSpace(file.SHA512)); sha512 != "" {
			hashes["sha512"] = sha512
		}
		if len(hashes) == 0 {
			return nil, nil, fmt.Errorf("%w: %s has no hashes", ErrInvalidModpack, path)
		}
		result = append(result, IndexFile{
			Path:      path,
			Hashes:    hashes,
			Downloads: downloads,
			FileSize:  file.FileSize,
		})
		remotePaths[path] = true
		seen[path] = true
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	return result, remotePaths, nil
}

func exportDownloads(downloads []string) []string {
	result := make([]string, 0, len(downloads))
	seen := make(map[string]bool, len(downloads))
	for _, download := range downloads {
		download = strings.TrimSpace(download)
		if download == "" || seen[download] {
			continue
		}
		result = append(result, download)
		seen[download] = true
	}
	return result
}

func cleanExportPath(relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidModpack)
	}
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("%w: absolute path %q", ErrInvalidModpack, relativePath)
	}
	cleanPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(relativePath)))
	if cleanPath == "." || !filepath.IsLocal(filepath.FromSlash(cleanPath)) {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidModpack, relativePath)
	}
	return cleanPath, nil
}

func exportOverridePaths(gameDir string, remotePaths map[string]bool) ([]exportOverridePath, error) {
	root, err := filepath.Abs(gameDir)
	if err != nil {
		return nil, err
	}

	var result []exportOverridePath
	for _, dir := range []string{"mods", "config", "resourcepacks", "shaderpacks", "datapacks"} {
		dirPath := filepath.Join(root, dir)
		info, err := os.Stat(dirPath)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(dirPath, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if shouldSkipExportFile(entry.Name()) && path != dirPath {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if shouldSkipExportFile(entry.Name()) {
				return nil
			}
			if dir == "mods" && !isExportableModFile(entry.Name()) {
				return nil
			}
			relativePath, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if relativePath == "." || !filepath.IsLocal(relativePath) {
				return fmt.Errorf("%w: unsafe export path %q", ErrInvalidModpack, relativePath)
			}
			if remotePaths[filepath.ToSlash(relativePath)] {
				return nil
			}
			result = append(result, exportOverridePath{
				sourcePath:   path,
				relativePath: relativePath,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	for _, name := range []string{"options.txt", "optionsof.txt", "servers.dat"} {
		path := filepath.Join(root, name)
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || shouldSkipExportFile(name) {
			continue
		}
		if remotePaths[filepath.ToSlash(name)] {
			continue
		}
		result = append(result, exportOverridePath{sourcePath: path, relativePath: name})
	}

	sort.Slice(result, func(i, j int) bool {
		return filepath.ToSlash(result[i].relativePath) < filepath.ToSlash(result[j].relativePath)
	})
	return result, nil
}

func shouldSkipExportFile(name string) bool {
	if name == ".DS_Store" || name == ".power-mine-modrinth.json" {
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".part") || strings.HasSuffix(lower, ".tmp")
}

func isExportableModFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".jar.disabled")
}

func writeZipBytes(writer *zip.Writer, name string, body []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	header.SetMode(0o644)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = entry.Write(body)
	return err
}

func writeZipFile(writer *zip.Writer, sourcePath string, archivePath string) error {
	if strings.TrimSpace(archivePath) == "" || filepath.IsAbs(archivePath) {
		return fmt.Errorf("%w: unsafe archive path %q", ErrInvalidModpack, archivePath)
	}
	cleanArchivePath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(archivePath)))
	if cleanArchivePath == "." || !filepath.IsLocal(filepath.FromSlash(cleanArchivePath)) {
		return fmt.Errorf("%w: unsafe archive path %q", ErrInvalidModpack, archivePath)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = cleanArchivePath
	header.Method = zip.Deflate
	writerEntry, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(writerEntry, source)
	return err
}

func (s *Service) installIndexFile(ctx context.Context, gameDir string, file IndexFile) (bool, error) {
	targetPath, err := safeTargetPath(gameDir, file.Path)
	if err != nil {
		return false, err
	}
	if valid, err := validPackFile(targetPath, file); err != nil {
		return false, err
	} else if valid {
		return false, nil
	}
	if len(file.Downloads) == 0 {
		return false, fmt.Errorf("%w: %s has no downloads", ErrInvalidModpack, file.Path)
	}

	var lastErr error
	for _, rawURL := range file.Downloads {
		if err := validateDownloadURL(rawURL); err != nil {
			lastErr = err
			continue
		}
		if err := s.downloadFile(ctx, rawURL, targetPath, file); err != nil {
			lastErr = err
			continue
		}
		return true, nil
	}
	if lastErr != nil {
		return false, fmt.Errorf("%s: %w", file.Path, lastErr)
	}
	return false, fmt.Errorf("%w: %s has no usable downloads", ErrInvalidModpack, file.Path)
}

func (s *Service) downloadFile(ctx context.Context, rawURL string, targetPath string, file IndexFile) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "Power-Mine-Launcher/0.1")

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("download request failed: %s", response.Status)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".*.part")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	sha1Hash := sha1.New()
	sha512Hash := sha512.New()
	written, err := io.Copy(io.MultiWriter(tmp, sha1Hash, sha512Hash), response.Body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := validatePackHashAndSize(hex.EncodeToString(sha1Hash.Sum(nil)), hex.EncodeToString(sha512Hash.Sum(nil)), written, file); err != nil {
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func extractOverride(gameDir string, override overrideFile) error {
	targetPath, err := safeTargetPath(gameDir, override.targetPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	source, err := override.file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	tmp, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".*.part")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, source); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func validPackFile(path string, file IndexFile) (bool, error) {
	handle, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer handle.Close()

	info, err := handle.Stat()
	if err != nil {
		return false, err
	}
	sha1Hash := sha1.New()
	sha512Hash := sha512.New()
	if _, err := io.Copy(io.MultiWriter(sha1Hash, sha512Hash), handle); err != nil {
		return false, err
	}
	if err := validatePackHashAndSize(hex.EncodeToString(sha1Hash.Sum(nil)), hex.EncodeToString(sha512Hash.Sum(nil)), info.Size(), file); err != nil {
		return false, nil
	}
	return true, nil
}

func validatePackHashAndSize(actualSHA1 string, actualSHA512 string, actualSize int64, file IndexFile) error {
	if file.FileSize > 0 && actualSize != file.FileSize {
		return fmt.Errorf("size mismatch: got %d, want %d", actualSize, file.FileSize)
	}
	if expected := strings.TrimSpace(file.Hashes["sha1"]); expected != "" && !strings.EqualFold(actualSHA1, expected) {
		return fmt.Errorf("sha1 mismatch: got %s, want %s", actualSHA1, expected)
	}
	if expected := strings.TrimSpace(file.Hashes["sha512"]); expected != "" && !strings.EqualFold(actualSHA512, expected) {
		return fmt.Errorf("sha512 mismatch: got %s, want %s", actualSHA512, expected)
	}
	return nil
}

func safeTargetPath(root string, relativePath string) (string, error) {
	if strings.TrimSpace(relativePath) == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidModpack)
	}
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("%w: absolute path %q", ErrInvalidModpack, relativePath)
	}
	cleanPath := filepath.Clean(filepath.FromSlash(relativePath))
	if cleanPath == "." || !filepath.IsLocal(cleanPath) {
		return "", fmt.Errorf("%w: unsafe path %q", ErrInvalidModpack, relativePath)
	}
	target := filepath.Join(root, cleanPath)
	relative, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return "", fmt.Errorf("%w: path escapes instance %q", ErrInvalidModpack, relativePath)
	}
	return target, nil
}

func validateDownloadURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%w: non-https download URL %q", ErrInvalidModpack, rawURL)
	}
	host := strings.ToLower(parsed.Hostname())
	switch host {
	case "cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com":
		return nil
	default:
		return fmt.Errorf("%w: download host %q is not allowed", ErrInvalidModpack, host)
	}
}

func emit(progress ProgressFunc, profileID string, event domain.InstallProgress) {
	if progress == nil {
		return
	}
	event.ProfileID = profileID
	progress(event)
}

func percent(current int, total int) int {
	if total <= 0 {
		return 100
	}
	return current * 100 / total
}
