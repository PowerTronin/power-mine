package javasvc

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"power-mine/internal/domain"
)

const adoptiumAssetsURL = "https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=%s&heap_size=normal&image_type=jre&jvm_impl=hotspot&os=%s&project=jdk&vendor=eclipse"

type ProgressFunc func(domain.JavaInstallProgress)

type Service struct {
	dataDir string
	client  *http.Client
}

func NewService(dataDir string) *Service {
	return &Service{
		dataDir: dataDir,
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (s *Service) Validate(ctx context.Context, javaPath string) domain.JavaStatus {
	if strings.TrimSpace(javaPath) == "" {
		javaPath = "java"
	}

	status := domain.JavaStatus{
		Path:      javaPath,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}

	validateCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	command := exec.CommandContext(validateCtx, javaPath, "-version")
	output, err := command.CombinedOutput()
	text := normalizeJavaOutput(output)
	if validateCtx.Err() != nil {
		status.Message = "Java validation timed out"
		return status
	}
	if err != nil {
		status.Message = "Java validation failed"
		if text != "" {
			status.Message += ": " + text
		} else {
			status.Message += ": " + err.Error()
		}
		return status
	}

	status.OK = true
	status.Version = parseJavaVersion(text)
	if status.Version == "" {
		status.Message = "Java executable is available"
	} else {
		status.Message = "Java " + status.Version + " is available"
	}
	return status
}

func (s *Service) InstallTemurin(ctx context.Context, version int, progress ProgressFunc) (string, error) {
	if version <= 0 {
		version = 21
	}
	if strings.TrimSpace(s.dataDir) == "" {
		return "", fmt.Errorf("java installer requires a data directory")
	}

	emit := func(event domain.JavaInstallProgress) {
		event.Version = fmt.Sprintf("%d", version)
		if progress != nil {
			progress(event)
		}
	}

	runtimeRoot := filepath.Join(s.dataDir, "runtimes", fmt.Sprintf("temurin-%d", version))
	currentDir := filepath.Join(runtimeRoot, "current")
	if javaPath, ok := s.findUsableJava(ctx, currentDir); ok {
		emit(domain.JavaInstallProgress{
			Stage:    "complete",
			Message:  "Java is already installed",
			Percent:  100,
			Done:     true,
			JavaPath: javaPath,
		})
		return javaPath, nil
	}

	emit(domain.JavaInstallProgress{
		Stage:   "metadata",
		Message: "Resolving Eclipse Temurin runtime",
		Percent: 5,
	})

	asset, err := s.fetchTemurinAsset(ctx, version)
	if err != nil {
		emitError(emit, "metadata", "Could not resolve Java runtime", err)
		return "", err
	}

	if err := os.MkdirAll(filepath.Join(runtimeRoot, "downloads"), 0o755); err != nil {
		emitError(emit, "prepare", "Could not create Java runtime directory", err)
		return "", err
	}

	archivePath := filepath.Join(runtimeRoot, "downloads", asset.Name)
	if err := s.ensureArchive(ctx, archivePath, asset, emit); err != nil {
		emitError(emit, "download", "Could not download Java runtime", err)
		return "", err
	}

	emit(domain.JavaInstallProgress{
		Stage:   "extract",
		Message: "Extracting Java runtime",
		Percent: 88,
	})

	tmpDir := filepath.Join(runtimeRoot, fmt.Sprintf(".install-%d", time.Now().UnixNano()))
	if err := os.RemoveAll(tmpDir); err != nil {
		emitError(emit, "extract", "Could not prepare Java extraction directory", err)
		return "", err
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		emitError(emit, "extract", "Could not prepare Java extraction directory", err)
		return "", err
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := extractTarGz(ctx, archivePath, tmpDir, 1); err != nil {
		emitError(emit, "extract", "Could not extract Java runtime", err)
		return "", err
	}

	tmpJavaPath, err := findJavaExecutable(tmpDir)
	if err != nil {
		emitError(emit, "configure", "Could not locate java executable", err)
		return "", err
	}
	relativeJavaPath, err := filepath.Rel(tmpDir, tmpJavaPath)
	if err != nil {
		emitError(emit, "configure", "Could not configure Java runtime", err)
		return "", err
	}

	emit(domain.JavaInstallProgress{
		Stage:   "configure",
		Message: "Configuring launcher Java path",
		Percent: 96,
	})

	if err := os.RemoveAll(currentDir); err != nil {
		emitError(emit, "configure", "Could not replace previous Java runtime", err)
		return "", err
	}
	if err := os.Rename(tmpDir, currentDir); err != nil {
		emitError(emit, "configure", "Could not activate Java runtime", err)
		return "", err
	}
	cleanupTmp = false

	javaPath := filepath.Join(currentDir, relativeJavaPath)
	if status := s.Validate(ctx, javaPath); !status.OK {
		err := fmt.Errorf("%s", status.Message)
		emitError(emit, "validate", "Installed Java failed validation", err)
		return "", err
	}

	emit(domain.JavaInstallProgress{
		Stage:    "complete",
		Message:  "Java runtime installed",
		Percent:  100,
		Done:     true,
		JavaPath: javaPath,
	})
	return javaPath, nil
}

func (s *Service) InstalledTemurin(ctx context.Context, version int) (string, bool) {
	if version <= 0 || strings.TrimSpace(s.dataDir) == "" {
		return "", false
	}
	return s.findUsableJava(ctx, filepath.Join(s.dataDir, "runtimes", fmt.Sprintf("temurin-%d", version), "current"))
}

func normalizeJavaOutput(output []byte) string {
	lines := bytes.Split(bytes.TrimSpace(output), []byte{'\n'})
	if len(lines) == 0 || len(lines[0]) == 0 {
		return ""
	}
	return strings.TrimSpace(string(lines[0]))
}

func parseJavaVersion(output string) string {
	match := regexp.MustCompile(`version "([^"]+)"`).FindStringSubmatch(output)
	if len(match) == 2 {
		return match[1]
	}
	match = regexp.MustCompile(`openjdk ([^ ]+)`).FindStringSubmatch(strings.ToLower(output))
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func MajorVersion(version string) int {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0
	}
	if strings.HasPrefix(version, "1.") {
		version = strings.TrimPrefix(version, "1.")
	}
	match := regexp.MustCompile(`^(\d+)`).FindStringSubmatch(version)
	if len(match) != 2 {
		return 0
	}
	var major int
	for _, digit := range match[1] {
		major = major*10 + int(digit-'0')
	}
	return major
}

func CompatibleMajor(actual int, required int) bool {
	if required <= 0 {
		return actual > 0
	}
	if required <= 8 {
		return actual == 8
	}
	return actual >= required
}

func (s *Service) findUsableJava(ctx context.Context, root string) (string, bool) {
	javaPath, err := findJavaExecutable(root)
	if err != nil {
		return "", false
	}
	status := s.Validate(ctx, javaPath)
	return javaPath, status.OK
}

func (s *Service) fetchTemurinAsset(ctx context.Context, version int) (temurinPackage, error) {
	osName, err := adoptiumOS(runtime.GOOS)
	if err != nil {
		return temurinPackage{}, err
	}
	arch, err := adoptiumArch(runtime.GOARCH)
	if err != nil {
		return temurinPackage{}, err
	}

	url := fmt.Sprintf(adoptiumAssetsURL, version, arch, osName)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return temurinPackage{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Power-Mine-Launcher/0.1")

	response, err := s.client.Do(request)
	if err != nil {
		return temurinPackage{}, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return temurinPackage{}, fmt.Errorf("adoptium metadata request failed: %s", response.Status)
	}

	var assets []temurinAsset
	if err := json.NewDecoder(response.Body).Decode(&assets); err != nil {
		return temurinPackage{}, err
	}
	if len(assets) == 0 || assets[0].Binary.Package.Link == "" || assets[0].Binary.Package.Name == "" {
		return temurinPackage{}, fmt.Errorf("temurin java %d jre is unavailable for %s/%s", version, osName, arch)
	}
	return assets[0].Binary.Package, nil
}

func (s *Service) ensureArchive(ctx context.Context, archivePath string, asset temurinPackage, emit ProgressFunc) error {
	if ok, err := validArchive(archivePath, asset); err != nil {
		return err
	} else if ok {
		emit(domain.JavaInstallProgress{
			Stage:   "download",
			Message: "Using cached Java archive",
			Current: asset.Size,
			Total:   asset.Size,
			Percent: 82,
		})
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.Link, nil)
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
		return fmt.Errorf("java download request failed: %s", response.Status)
	}

	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(archivePath), filepath.Base(archivePath)+".*.part")
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

	total := asset.Size
	if total <= 0 {
		total = response.ContentLength
	}
	hash := sha256.New()
	reader := &progressReader{
		reader: response.Body,
		total:  total,
		onProgress: func(current, total int64) {
			emit(domain.JavaInstallProgress{
				Stage:   "download",
				Message: "Downloading Java runtime",
				Current: current,
				Total:   total,
				Percent: scaledPercent(current, total, 10, 72),
			})
		},
	}

	written, err := io.Copy(io.MultiWriter(tmp, hash), reader)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := validateArchiveHashAndSize(hex.EncodeToString(hash.Sum(nil)), written, asset); err != nil {
		return err
	}
	if err := os.Rename(tmpName, archivePath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func validArchive(path string, asset temurinPackage) (bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return false, err
	}

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	if err := validateArchiveHashAndSize(hex.EncodeToString(hash.Sum(nil)), info.Size(), asset); err != nil {
		return false, nil
	}
	return true, nil
}

func validateArchiveHashAndSize(actualHash string, actualSize int64, asset temurinPackage) error {
	if asset.Size > 0 && actualSize != asset.Size {
		return fmt.Errorf("java archive size mismatch: got %d, want %d", actualSize, asset.Size)
	}
	if asset.Checksum != "" && !strings.EqualFold(actualHash, asset.Checksum) {
		return fmt.Errorf("java archive sha256 mismatch: got %s, want %s", actualHash, asset.Checksum)
	}
	return nil
}

func extractTarGz(ctx context.Context, archivePath string, targetDir string, stripComponents int) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		relativePath, ok := stripPathComponents(header.Name, stripComponents)
		if !ok {
			continue
		}
		targetPath, err := safeJoin(targetDir, relativePath)
		if err != nil {
			return err
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, header.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			if err := writeTarFile(reader, targetPath, header.FileInfo().Mode()); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := writeTarSymlink(targetDir, targetPath, header.Linkname); err != nil {
				return err
			}
		case tar.TypeLink:
			if err := writeTarHardlink(targetDir, targetPath, header.Linkname, stripComponents); err != nil {
				return err
			}
		}
	}
}

func writeTarFile(reader io.Reader, targetPath string, mode os.FileMode) error {
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, reader); err != nil {
		return err
	}
	return os.Chmod(targetPath, mode)
}

func writeTarSymlink(root string, targetPath string, linkName string) error {
	if linkName == "" {
		return nil
	}
	linkTarget := filepath.Clean(filepath.Join(filepath.Dir(targetPath), linkName))
	if !pathWithin(root, linkTarget) {
		return fmt.Errorf("unsafe symlink target %q", linkName)
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(targetPath)
	return os.Symlink(linkName, targetPath)
}

func writeTarHardlink(root string, targetPath string, linkName string, stripComponents int) error {
	relativeLink, ok := stripPathComponents(linkName, stripComponents)
	if !ok {
		return nil
	}
	sourcePath, err := safeJoin(root, relativeLink)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(targetPath)
	return os.Link(sourcePath, targetPath)
}

func findJavaExecutable(root string) (string, error) {
	var found string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Base(path) == "java" && filepath.Base(filepath.Dir(path)) == "bin" {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("java executable was not found under %s", root)
	}
	return found, nil
}

func stripPathComponents(path string, count int) (string, bool) {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." || strings.HasPrefix(path, "../") || path == ".." || strings.HasPrefix(path, "/") {
		return "", false
	}
	parts := strings.Split(path, "/")
	if len(parts) <= count {
		return "", false
	}
	return filepath.FromSlash(strings.Join(parts[count:], "/")), true
}

func safeJoin(root string, relativePath string) (string, error) {
	cleanRelative := filepath.Clean(relativePath)
	if !filepath.IsLocal(cleanRelative) {
		return "", fmt.Errorf("unsafe archive path %q", relativePath)
	}
	target := filepath.Join(root, cleanRelative)
	if !pathWithin(root, target) {
		return "", fmt.Errorf("archive path escapes target directory: %q", relativePath)
	}
	return target, nil
}

func pathWithin(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func adoptiumOS(goos string) (string, error) {
	switch goos {
	case "darwin":
		return "mac", nil
	case "linux":
		return "linux", nil
	default:
		return "", fmt.Errorf("unsupported os %q for Java runtime install", goos)
	}
}

func adoptiumArch(goarch string) (string, error) {
	switch goarch {
	case "amd64":
		return "x64", nil
	case "arm64":
		return "aarch64", nil
	default:
		return "", fmt.Errorf("unsupported architecture %q for Java runtime install", goarch)
	}
}

func scaledPercent(current int64, total int64, min int, span int) int {
	if total <= 0 || current <= 0 {
		return min
	}
	percent := min + int(current*int64(span)/total)
	if percent > min+span {
		return min + span
	}
	return percent
}

func emitError(emit ProgressFunc, stage string, message string, err error) {
	emit(domain.JavaInstallProgress{
		Stage:   stage,
		Message: message,
		Done:    true,
		Error:   err.Error(),
	})
}

type progressReader struct {
	reader     io.Reader
	total      int64
	current    int64
	onProgress func(current int64, total int64)
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.current += int64(n)
		if r.onProgress != nil {
			r.onProgress(r.current, r.total)
		}
	}
	return n, err
}

type temurinAsset struct {
	Binary struct {
		Package temurinPackage `json:"package"`
	} `json:"binary"`
}

type temurinPackage struct {
	Checksum string `json:"checksum"`
	Link     string `json:"link"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
}
