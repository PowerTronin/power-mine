package minecraft

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"power-mine/internal/domain"
)

const (
	versionManifestURL    = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	resourcesURL          = "https://resources.download.minecraft.net"
	minecraftLibrariesURL = "https://libraries.minecraft.net"
	fabricMetaURL         = "https://meta.fabricmc.net/v2/versions/loader"
	quiltMetaURL          = "https://meta.quiltmc.org/v3/versions/loader"
	downloadTimeout       = 2 * time.Minute
	downloadAttempts      = 3
	downloadRetryDelay    = 750 * time.Millisecond
)

type ProgressFunc func(domain.InstallProgress)

type Service struct {
	dataDir string
	client  *http.Client
}

func NewService(dataDir string) *Service {
	return &Service{
		dataDir: dataDir,
		client:  minecraftHTTPClient(),
	}
}

func minecraftHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 30 * time.Second
	return &http.Client{
		Timeout:   downloadTimeout,
		Transport: transport,
	}
}

func (s *Service) InstallVanillaBase(ctx context.Context, profile domain.Profile, progress ProgressFunc) error {
	emit := func(event domain.InstallProgress) {
		event.ProfileID = profile.ID
		if progress != nil {
			progress(event)
		}
	}

	emit(domain.InstallProgress{
		Stage:   "metadata",
		Message: "Resolving Minecraft metadata",
	})

	version, err := s.fetchVersion(ctx, profile.MinecraftVersion)
	if err != nil {
		return err
	}

	assetIndex, assetIndexRaw, err := s.fetchAssetIndex(ctx, version.AssetIndex.URL)
	if err != nil {
		return err
	}

	plan := s.buildDownloadPlan(version, assetIndex)
	if version.AssetIndex.URL != "" {
		plan = append([]downloadItem{{
			URL:    version.AssetIndex.URL,
			Path:   filepath.Join(s.minecraftDir(), "assets", "indexes", version.AssetIndex.ID+".json"),
			SHA1:   version.AssetIndex.SHA1,
			Size:   version.AssetIndex.Size,
			Label:  "Asset index " + version.AssetIndex.ID,
			Raw:    assetIndexRaw,
			RawSet: true,
		}}, plan...)
	}

	total := len(plan)
	completed := 0
	emit(domain.InstallProgress{
		Stage:   "download",
		Message: fmt.Sprintf("Downloading %d required files", total),
		Total:   total,
	})

	for _, item := range plan {
		completed++
		percent := 0
		if total > 0 {
			percent = completed * 100 / total
		}
		emit(domain.InstallProgress{
			Stage:   "download",
			Message: item.Label,
			Current: completed,
			Total:   total,
			Percent: percent,
		})
		if err := s.ensureDownload(ctx, item); err != nil {
			return fmt.Errorf("%s: %w", item.Label, err)
		}
	}

	if err := s.prepareAssetViews(version.AssetIndex.ID, assetIndex); err != nil {
		return err
	}

	emit(domain.InstallProgress{
		Stage:   "complete",
		Message: "Vanilla base files installed",
		Current: total,
		Total:   total,
		Percent: 100,
		Done:    true,
	})
	return nil
}

func (s *Service) InstallFabricLoader(ctx context.Context, profile domain.Profile, progress ProgressFunc) (string, error) {
	emit := func(event domain.InstallProgress) {
		event.ProfileID = profile.ID
		if progress != nil {
			progress(event)
		}
	}

	requestedLoaderVersion := strings.TrimSpace(profile.Loader.Version)
	if requestedLoaderVersion == "" {
		requestedLoaderVersion = "latest"
	}
	loaderVersion := requestedLoaderVersion
	if loaderVersion == "latest" {
		var err error
		loaderVersion, err = s.latestFabricLoaderVersion(ctx, profile.MinecraftVersion)
		if err != nil {
			return "", err
		}
	}

	emit(domain.InstallProgress{
		Stage:   "fabric-metadata",
		Message: "Resolving Fabric loader metadata",
		Percent: 85,
	})

	fabric, err := s.fetchFabricProfile(ctx, profile.MinecraftVersion, loaderVersion)
	if err != nil {
		return "", err
	}

	base, err := s.loadInstalledVersion(profile.MinecraftVersion)
	if err != nil {
		return "", err
	}

	versionID := FabricVersionID(profile.MinecraftVersion, requestedLoaderVersion)
	normalizeLibraryDownloads(fabric.Libraries)
	composite := mergeFabricProfile(base, fabric, versionID)
	normalizeLibraryDownloads(composite.Libraries)

	plan := s.buildLibraryDownloadPlan(fabric.Libraries)
	total := len(plan)
	for index, item := range plan {
		current := index + 1
		percent := 85
		if total > 0 {
			percent = 85 + current*10/total
		}
		emit(domain.InstallProgress{
			Stage:   "fabric-download",
			Message: item.Label,
			Current: current,
			Total:   total,
			Percent: percent,
		})
		if err := s.ensureDownload(ctx, item); err != nil {
			return "", fmt.Errorf("%s: %w", item.Label, err)
		}
	}

	if err := s.writeVersionMetadata(versionID, composite); err != nil {
		return "", err
	}

	emit(domain.InstallProgress{
		Stage:   "fabric-complete",
		Message: "Fabric loader installed",
		Current: total,
		Total:   total,
		Percent: 100,
		Done:    true,
	})
	return loaderVersion, nil
}

func (s *Service) InstallQuiltLoader(ctx context.Context, profile domain.Profile, progress ProgressFunc) (string, error) {
	emit := func(event domain.InstallProgress) {
		event.ProfileID = profile.ID
		if progress != nil {
			progress(event)
		}
	}

	requestedLoaderVersion := strings.TrimSpace(profile.Loader.Version)
	if requestedLoaderVersion == "" {
		requestedLoaderVersion = "latest"
	}
	loaderVersion := requestedLoaderVersion
	if loaderVersion == "latest" {
		var err error
		loaderVersion, err = s.latestQuiltLoaderVersion(ctx, profile.MinecraftVersion)
		if err != nil {
			return "", err
		}
	}

	emit(domain.InstallProgress{
		Stage:   "quilt-metadata",
		Message: "Resolving Quilt loader metadata",
		Percent: 85,
	})

	quilt, err := s.fetchQuiltProfile(ctx, profile.MinecraftVersion, loaderVersion)
	if err != nil {
		return "", err
	}

	base, err := s.loadInstalledVersion(profile.MinecraftVersion)
	if err != nil {
		return "", err
	}

	versionID := QuiltVersionID(profile.MinecraftVersion, requestedLoaderVersion)
	normalizeLibraryDownloads(quilt.Libraries)
	composite := mergeFabricProfile(base, quilt, versionID)
	normalizeLibraryDownloads(composite.Libraries)

	plan := s.buildLibraryDownloadPlan(quilt.Libraries)
	total := len(plan)
	for index, item := range plan {
		current := index + 1
		percent := 85
		if total > 0 {
			percent = 85 + current*10/total
		}
		emit(domain.InstallProgress{
			Stage:   "quilt-download",
			Message: item.Label,
			Current: current,
			Total:   total,
			Percent: percent,
		})
		if err := s.ensureDownload(ctx, item); err != nil {
			return "", fmt.Errorf("%s: %w", item.Label, err)
		}
	}

	if err := s.writeVersionMetadata(versionID, composite); err != nil {
		return "", err
	}

	emit(domain.InstallProgress{
		Stage:   "quilt-complete",
		Message: "Quilt loader installed",
		Current: total,
		Total:   total,
		Percent: 100,
		Done:    true,
	})
	return loaderVersion, nil
}

func (s *Service) RequiredJavaVersion(versionID string) (int, error) {
	version, err := s.loadInstalledVersion(versionID)
	if err != nil {
		return InferRequiredJavaVersion(versionID), nil
	}
	if version.JavaVersion.MajorVersion > 0 {
		return version.JavaVersion.MajorVersion, nil
	}
	return InferRequiredJavaVersion(versionID), nil
}

func InferRequiredJavaVersion(versionID string) int {
	major, minor, patch, ok := parseReleaseVersion(versionID)
	if !ok || major != 1 {
		return 8
	}
	if minor > 20 || (minor == 20 && patch >= 5) {
		return 21
	}
	if minor >= 18 {
		return 17
	}
	if minor == 17 {
		return 16
	}
	return 8
}

func parseReleaseVersion(versionID string) (int, int, int, bool) {
	parts := strings.Split(versionID, ".")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	major, ok := parseNumericPart(parts[0])
	if !ok {
		return 0, 0, 0, false
	}
	minor, ok := parseNumericPart(parts[1])
	if !ok {
		return 0, 0, 0, false
	}
	patch := 0
	if len(parts) >= 3 {
		patch, _ = parseNumericPart(parts[2])
	}
	return major, minor, patch, true
}

func parseNumericPart(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	number := 0
	seenDigit := false
	for _, char := range value {
		if char < '0' || char > '9' {
			break
		}
		seenDigit = true
		number = number*10 + int(char-'0')
	}
	return number, seenDigit
}

func (s *Service) minecraftDir() string {
	return filepath.Join(s.dataDir, "minecraft")
}

func (s *Service) fetchVersion(ctx context.Context, versionID string) (versionMetadata, error) {
	manifest, err := s.fetchManifest(ctx)
	if err != nil {
		return versionMetadata{}, err
	}

	var versionURL string
	for _, version := range manifest.Versions {
		if version.ID == versionID {
			versionURL = version.URL
			break
		}
	}
	if versionURL == "" {
		return versionMetadata{}, fmt.Errorf("minecraft version %q was not found", versionID)
	}

	raw, err := s.getRaw(ctx, versionURL)
	if err != nil {
		return versionMetadata{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.minecraftDir(), "versions", versionID), 0o755); err != nil {
		return versionMetadata{}, err
	}
	if err := os.WriteFile(filepath.Join(s.minecraftDir(), "versions", versionID, versionID+".json"), raw, 0o644); err != nil {
		return versionMetadata{}, err
	}

	var metadata versionMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return versionMetadata{}, err
	}
	return metadata, nil
}

func (s *Service) writeVersionMetadata(versionID string, metadata versionMetadata) error {
	if err := os.MkdirAll(filepath.Join(s.minecraftDir(), "versions", versionID), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.minecraftDir(), "versions", versionID, versionID+".json"), raw, 0o644)
}

func (s *Service) fetchManifest(ctx context.Context) (versionManifest, error) {
	var manifest versionManifest
	raw, err := s.getRaw(ctx, versionManifestURL)
	if err != nil {
		return manifest, err
	}
	if err := os.MkdirAll(filepath.Join(s.minecraftDir(), "versions"), 0o755); err != nil {
		return manifest, err
	}
	if err := os.WriteFile(filepath.Join(s.minecraftDir(), "versions", "version_manifest_v2.json"), raw, 0o644); err != nil {
		return manifest, err
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return manifest, err
	}
	return manifest, nil
}

func (s *Service) fetchAssetIndex(ctx context.Context, url string) (assetIndex, []byte, error) {
	raw, err := s.getRaw(ctx, url)
	if err != nil {
		return assetIndex{}, nil, err
	}
	var index assetIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return assetIndex{}, nil, err
	}
	return index, raw, nil
}

func (s *Service) latestFabricLoaderVersion(ctx context.Context, minecraftVersion string) (string, error) {
	raw, err := s.getRaw(ctx, fabricMetaURL+"/"+minecraftVersion)
	if err != nil {
		return "", err
	}
	var versions []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return "", err
	}
	for _, version := range versions {
		if version.Loader.Version != "" {
			return version.Loader.Version, nil
		}
	}
	return "", fmt.Errorf("no Fabric loader version found for Minecraft %s", minecraftVersion)
}

func (s *Service) latestQuiltLoaderVersion(ctx context.Context, minecraftVersion string) (string, error) {
	raw, err := s.getRaw(ctx, quiltMetaURL+"/"+minecraftVersion)
	if err != nil {
		return "", err
	}
	var versions []struct {
		Loader struct {
			Version string `json:"version"`
		} `json:"loader"`
	}
	if err := json.Unmarshal(raw, &versions); err != nil {
		return "", err
	}
	for _, version := range versions {
		if version.Loader.Version != "" {
			return version.Loader.Version, nil
		}
	}
	return "", fmt.Errorf("no Quilt loader version found for Minecraft %s", minecraftVersion)
}

func (s *Service) fetchFabricProfile(ctx context.Context, minecraftVersion string, loaderVersion string) (versionMetadata, error) {
	raw, err := s.getRaw(ctx, fabricMetaURL+"/"+minecraftVersion+"/"+loaderVersion+"/profile/json")
	if err != nil {
		return versionMetadata{}, err
	}
	var profile versionMetadata
	if err := json.Unmarshal(raw, &profile); err != nil {
		return versionMetadata{}, err
	}
	return profile, nil
}

func (s *Service) fetchQuiltProfile(ctx context.Context, minecraftVersion string, loaderVersion string) (versionMetadata, error) {
	raw, err := s.getRaw(ctx, quiltMetaURL+"/"+minecraftVersion+"/"+loaderVersion+"/profile/json")
	if err != nil {
		return versionMetadata{}, err
	}
	var profile versionMetadata
	if err := json.Unmarshal(raw, &profile); err != nil {
		return versionMetadata{}, err
	}
	return profile, nil
}

func (s *Service) buildDownloadPlan(version versionMetadata, index assetIndex) []downloadItem {
	var plan []downloadItem

	if version.Downloads.Client.URL != "" {
		plan = append(plan, downloadItem{
			URL:   version.Downloads.Client.URL,
			Path:  filepath.Join(s.minecraftDir(), "versions", version.ID, version.ID+".jar"),
			SHA1:  version.Downloads.Client.SHA1,
			Size:  version.Downloads.Client.Size,
			Label: "Client jar " + version.ID,
		})
	}

	if version.Logging.Client.File.URL != "" {
		plan = append(plan, downloadItem{
			URL:   version.Logging.Client.File.URL,
			Path:  filepath.Join(s.minecraftDir(), "assets", "log_configs", version.Logging.Client.File.ID),
			SHA1:  version.Logging.Client.File.SHA1,
			Size:  version.Logging.Client.File.Size,
			Label: "Log config " + version.Logging.Client.File.ID,
		})
	}

	for _, library := range version.Libraries {
		if !libraryAllowed(library.Rules) {
			continue
		}
		if library.Downloads.Artifact.URL != "" {
			plan = append(plan, downloadItem{
				URL:   library.Downloads.Artifact.URL,
				Path:  filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(library.Downloads.Artifact.Path)),
				SHA1:  library.Downloads.Artifact.SHA1,
				Size:  library.Downloads.Artifact.Size,
				Label: "Library " + library.Name,
			})
		}
		if classifier, ok := nativeClassifier(library); ok {
			artifact, ok := library.Downloads.Classifiers[classifier]
			if ok && artifact.URL != "" {
				plan = append(plan, downloadItem{
					URL:   artifact.URL,
					Path:  filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(artifact.Path)),
					SHA1:  artifact.SHA1,
					Size:  artifact.Size,
					Label: "Native library " + library.Name,
				})
			}
		}
	}

	for logicalPath, object := range index.Objects {
		if len(object.Hash) < 2 {
			continue
		}
		prefix := object.Hash[:2]
		plan = append(plan, downloadItem{
			URL:   resourcesURL + "/" + prefix + "/" + object.Hash,
			Path:  filepath.Join(s.minecraftDir(), "assets", "objects", prefix, object.Hash),
			SHA1:  object.Hash,
			Size:  object.Size,
			Label: "Asset " + logicalPath,
		})
	}

	return plan
}

func (s *Service) buildLibraryDownloadPlan(libraries []libraryMetadata) []downloadItem {
	var plan []downloadItem
	for _, library := range libraries {
		if !libraryAllowed(library.Rules) {
			continue
		}
		if library.Downloads.Artifact.URL == "" || library.Downloads.Artifact.Path == "" {
			continue
		}
		plan = append(plan, downloadItem{
			URL:   library.Downloads.Artifact.URL,
			Path:  filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(library.Downloads.Artifact.Path)),
			SHA1:  library.Downloads.Artifact.SHA1,
			Size:  library.Downloads.Artifact.Size,
			Label: "Library " + library.Name,
		})
	}
	return plan
}

func (s *Service) prepareAssetViews(indexID string, index assetIndex) error {
	if indexID == "" {
		return nil
	}
	if index.Virtual {
		if err := s.materializeAssets(filepath.Join(s.minecraftDir(), "assets", "virtual", indexID), index); err != nil {
			return err
		}
	}
	if index.MapToResources {
		if err := s.materializeAssets(filepath.Join(s.minecraftDir(), "resources"), index); err != nil {
			return err
		}
		if err := s.materializeAssets(filepath.Join(s.minecraftDir(), "assets", "virtual", indexID), index); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) prepareInstalledAssetViews(version versionMetadata) error {
	indexID := version.AssetIndex.ID
	if indexID == "" {
		indexID = version.Assets
	}
	if indexID == "" {
		return nil
	}

	raw, err := os.ReadFile(filepath.Join(s.minecraftDir(), "assets", "indexes", indexID+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var index assetIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return err
	}
	return s.prepareAssetViews(indexID, index)
}

func (s *Service) materializeAssets(targetRoot string, index assetIndex) error {
	for logicalPath, object := range index.Objects {
		if len(object.Hash) < 2 {
			continue
		}
		cleanPath := filepath.Clean(filepath.FromSlash(logicalPath))
		if !filepath.IsLocal(cleanPath) {
			continue
		}

		sourcePath := filepath.Join(s.minecraftDir(), "assets", "objects", object.Hash[:2], object.Hash)
		targetPath := filepath.Join(targetRoot, cleanPath)
		if valid, err := validFile(targetPath, downloadItem{SHA1: object.Hash, Size: object.Size}); err != nil {
			return err
		} else if valid {
			continue
		}
		if err := copyFile(sourcePath, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureDownload(ctx context.Context, item downloadItem) error {
	return retryNetwork(ctx, func() error {
		if valid, err := validFile(item.Path, item); err != nil {
			return err
		} else if valid {
			return nil
		}

		if item.RawSet {
			return writeVerified(item.Path, item.Raw, item)
		}

		return s.downloadItem(ctx, item)
	})
}

func (s *Service) downloadItem(ctx context.Context, item downloadItem) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
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
		return httpStatusError{Prefix: "download request failed", Status: response.Status, StatusCode: response.StatusCode}
	}

	if err := os.MkdirAll(filepath.Dir(item.Path), 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(item.Path), filepath.Base(item.Path)+".*.part")
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

	hash := sha1.New()
	written, err := io.Copy(io.MultiWriter(tmp, hash), response.Body)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := validateHashAndSize(hex.EncodeToString(hash.Sum(nil)), written, item); err != nil {
		return err
	}
	if err := os.Rename(tmpName, item.Path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func (s *Service) getRaw(ctx context.Context, url string) ([]byte, error) {
	var raw []byte
	err := retryNetwork(ctx, func() error {
		var err error
		raw, err = s.getRawOnce(ctx, url)
		return err
	})
	return raw, err
}

func (s *Service) getRawOnce(ctx context.Context, url string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Power-Mine-Launcher/0.1")

	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, httpStatusError{Prefix: "metadata request failed", Status: response.Status, StatusCode: response.StatusCode}
	}
	return io.ReadAll(response.Body)
}

func retryNetwork(ctx context.Context, fn func() error) error {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if !retryableNetworkError(err) || attempt == downloadAttempts {
				return err
			}
			if waitErr := waitNetworkRetry(ctx, attempt); waitErr != nil {
				return waitErr
			}
			continue
		}
		return nil
	}
	return lastErr
}

func retryableNetworkError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var statusErr httpStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode >= http.StatusBadRequest && statusErr.StatusCode < http.StatusInternalServerError {
		return false
	}
	return true
}

func waitNetworkRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * downloadRetryDelay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type httpStatusError struct {
	Prefix     string
	Status     string
	StatusCode int
}

func (e httpStatusError) Error() string {
	return e.Prefix + ": " + e.Status
}

func writeVerified(path string, raw []byte, item downloadItem) error {
	hash := sha1.Sum(raw)
	if err := validateHashAndSize(hex.EncodeToString(hash[:]), int64(len(raw)), item); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.part")
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
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func copyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

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

func validFile(path string, item downloadItem) (bool, error) {
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
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, err
	}
	if err := validateHashAndSize(hex.EncodeToString(hash.Sum(nil)), info.Size(), item); err != nil {
		return false, nil
	}
	return true, nil
}

func validateHashAndSize(actualHash string, actualSize int64, item downloadItem) error {
	if item.Size > 0 && actualSize != item.Size {
		return fmt.Errorf("size mismatch: got %d, want %d", actualSize, item.Size)
	}
	if item.SHA1 != "" && !strings.EqualFold(actualHash, item.SHA1) {
		return fmt.Errorf("sha1 mismatch: got %s, want %s", actualHash, item.SHA1)
	}
	return nil
}

func FabricVersionID(minecraftVersion string, loaderVersion string) string {
	loaderVersion = strings.TrimSpace(loaderVersion)
	if loaderVersion == "" {
		loaderVersion = "latest"
	}
	return "fabric-loader-" + loaderVersion + "-" + strings.TrimSpace(minecraftVersion)
}

func QuiltVersionID(minecraftVersion string, loaderVersion string) string {
	loaderVersion = strings.TrimSpace(loaderVersion)
	if loaderVersion == "" {
		loaderVersion = "latest"
	}
	return "quilt-loader-" + loaderVersion + "-" + strings.TrimSpace(minecraftVersion)
}

func mergeFabricProfile(base versionMetadata, fabric versionMetadata, versionID string) versionMetadata {
	merged := base
	merged.ID = versionID
	merged.InheritsFrom = base.ID
	merged.Type = fabric.Type
	if merged.Type == "" {
		merged.Type = "release"
	}
	if fabric.MainClass != "" {
		merged.MainClass = fabric.MainClass
	}
	merged.Libraries = append(append([]libraryMetadata{}, base.Libraries...), fabric.Libraries...)
	merged.Arguments.JVM = append(append([]json.RawMessage{}, base.Arguments.JVM...), fabric.Arguments.JVM...)
	if len(fabric.Arguments.Game) > 0 {
		merged.Arguments.Game = append(append([]json.RawMessage{}, base.Arguments.Game...), fabric.Arguments.Game...)
	}
	if fabric.MinecraftArguments != "" {
		merged.MinecraftArguments = fabric.MinecraftArguments
	}
	return merged
}

func normalizeLibraryDownloads(libraries []libraryMetadata) {
	for index := range libraries {
		if libraries[index].Downloads.Artifact.Path != "" {
			continue
		}
		if _, ok := nativeClassifier(libraries[index]); ok && len(libraries[index].Downloads.Classifiers) > 0 {
			continue
		}
		path, ok := mavenArtifactPath(libraries[index].Name)
		if !ok {
			continue
		}
		baseURL := strings.TrimSpace(libraries[index].URL)
		if baseURL == "" {
			baseURL = defaultLibraryBaseURL(libraries[index].Name)
		}
		if baseURL == "" {
			continue
		}
		sha1 := libraries[index].SHA1
		if sha1 == "" && len(libraries[index].Checksums) > 0 {
			sha1 = libraries[index].Checksums[0]
		}
		libraries[index].Downloads.Artifact = artifact{
			Path: path,
			URL:  strings.TrimRight(baseURL, "/") + "/" + path,
			SHA1: sha1,
			Size: libraries[index].Size,
		}
	}
}

func defaultLibraryBaseURL(name string) string {
	group, _, ok := strings.Cut(strings.TrimSpace(name), ":")
	if !ok || group == "" {
		return ""
	}
	switch group {
	case "net.minecraftforge":
		return forgeMavenBaseURL
	case "net.neoforged":
		return neoForgeMavenBaseURL
	default:
		return minecraftLibrariesURL
	}
}

func mavenArtifactPath(name string) (string, bool) {
	parts := strings.Split(name, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return "", false
	}
	group, artifactID, version := parts[0], parts[1], parts[2]
	if group == "" || artifactID == "" || version == "" {
		return "", false
	}
	fileName := artifactID + "-" + version
	if len(parts) == 4 && parts[3] != "" {
		fileName += "-" + parts[3]
	}
	fileName += ".jar"
	pathParts := append(strings.Split(group, "."), artifactID, version, fileName)
	return strings.Join(pathParts, "/"), true
}

func nativeClassifier(library libraryMetadata) (string, bool) {
	classifier, ok := library.Natives[minecraftOSName()]
	if !ok || classifier == "" {
		return "", false
	}
	return strings.ReplaceAll(classifier, "${arch}", "64"), true
}

func libraryAllowed(rules []rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, rule := range rules {
		if ruleMatches(rule) {
			allowed = rule.Action == "allow"
		}
	}
	return allowed
}

func ruleMatches(rule rule) bool {
	if rule.OS == nil {
		return true
	}
	if rule.OS.Name != "" && rule.OS.Name != minecraftOSName() {
		return false
	}
	if rule.OS.Arch != "" {
		matches, err := regexp.MatchString(rule.OS.Arch, runtime.GOARCH)
		if err != nil || !matches {
			return false
		}
	}
	return true
}

func minecraftOSName() string {
	switch runtime.GOOS {
	case "darwin":
		return "osx"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}

type downloadItem struct {
	URL    string
	Path   string
	SHA1   string
	Size   int64
	Label  string
	Raw    []byte
	RawSet bool
}

type versionManifest struct {
	Versions []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"versions"`
}

type versionMetadata struct {
	ID           string `json:"id"`
	InheritsFrom string `json:"inheritsFrom,omitempty"`
	Type         string `json:"type"`
	MainClass    string `json:"mainClass"`
	Assets       string `json:"assets"`
	AssetIndex   struct {
		ID   string `json:"id"`
		URL  string `json:"url"`
		SHA1 string `json:"sha1"`
		Size int64  `json:"size"`
	} `json:"assetIndex"`
	Downloads struct {
		Client artifact `json:"client"`
	} `json:"downloads"`
	JavaVersion struct {
		Component    string `json:"component"`
		MajorVersion int    `json:"majorVersion"`
	} `json:"javaVersion"`
	Arguments          launchArguments   `json:"arguments"`
	MinecraftArguments string            `json:"minecraftArguments"`
	Libraries          []libraryMetadata `json:"libraries"`
	Logging            struct {
		Client struct {
			Argument string   `json:"argument"`
			File     artifact `json:"file"`
		} `json:"client"`
	} `json:"logging"`
}

type launchArguments struct {
	Game []json.RawMessage `json:"game"`
	JVM  []json.RawMessage `json:"jvm"`
}

type libraryMetadata struct {
	Name      string   `json:"name"`
	URL       string   `json:"url,omitempty"`
	SHA1      string   `json:"sha1,omitempty"`
	Checksums []string `json:"checksums,omitempty"`
	Size      int64    `json:"size,omitempty"`
	Downloads struct {
		Artifact    artifact            `json:"artifact"`
		Classifiers map[string]artifact `json:"classifiers"`
	} `json:"downloads"`
	Natives map[string]string `json:"natives"`
	Rules   []rule            `json:"rules"`
	Extract struct {
		Exclude []string `json:"exclude"`
	} `json:"extract"`
}

type artifact struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	ID   string `json:"id"`
}

type rule struct {
	Action   string          `json:"action"`
	OS       *ruleOS         `json:"os"`
	Features map[string]bool `json:"features"`
}

type ruleOS struct {
	Name string `json:"name"`
	Arch string `json:"arch"`
}

type assetIndex struct {
	MapToResources bool                   `json:"map_to_resources"`
	Virtual        bool                   `json:"virtual"`
	Objects        map[string]assetObject `json:"objects"`
}

type assetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
