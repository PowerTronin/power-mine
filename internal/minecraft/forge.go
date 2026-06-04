package minecraft

import (
	"archive/zip"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"power-mine/internal/domain"
)

const (
	forgeMavenBaseURL        = "https://maven.minecraftforge.net"
	forgeMavenMetadataURL    = forgeMavenBaseURL + "/net/minecraftforge/forge/maven-metadata.xml"
	neoForgeMavenBaseURL     = "https://maven.neoforged.net/releases"
	neoForgeMavenMetadataURL = neoForgeMavenBaseURL + "/net/neoforged/neoforge/maven-metadata.xml"
)

type forgeLikeInstallerProvider struct {
	Name                   string
	Stage                  string
	MavenBaseURL           string
	MavenMetadataURL       string
	Group                  string
	Artifact               string
	VersionID              func(string, string) string
	MinecraftVersion       func(string) string
	NormalizeLoaderVersion func(string, string) string
}

var forgeInstallerProvider = forgeLikeInstallerProvider{
	Name:             "Forge",
	Stage:            "forge",
	MavenBaseURL:     forgeMavenBaseURL,
	MavenMetadataURL: forgeMavenMetadataURL,
	Group:            "net.minecraftforge",
	Artifact:         "forge",
	VersionID:        ForgeVersionID,
	MinecraftVersion: forgeMinecraftVersion,
	NormalizeLoaderVersion: func(minecraftVersion string, loaderVersion string) string {
		if !strings.Contains(loaderVersion, "-") {
			return minecraftVersion + "-" + loaderVersion
		}
		return loaderVersion
	},
}

var neoForgeInstallerProvider = forgeLikeInstallerProvider{
	Name:                   "NeoForge",
	Stage:                  "neoforge",
	MavenBaseURL:           neoForgeMavenBaseURL,
	MavenMetadataURL:       neoForgeMavenMetadataURL,
	Group:                  "net.neoforged",
	Artifact:               "neoforge",
	VersionID:              NeoForgeVersionID,
	MinecraftVersion:       neoForgeMinecraftVersion,
	NormalizeLoaderVersion: func(_ string, loaderVersion string) string { return loaderVersion },
}

type forgeMetadata struct {
	Versioning struct {
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

type forgeInstallProfile struct {
	Version     string                    `json:"version"`
	Minecraft   string                    `json:"minecraft"`
	Install     forgeInstallerInstall     `json:"install"`
	VersionInfo versionMetadata           `json:"versionInfo"`
	Data        map[string]forgeDataValue `json:"data"`
	Processors  []forgeProcessor          `json:"processors"`
	Libraries   []libraryMetadata         `json:"libraries"`
}

type forgeInstallerInstall struct {
	Path     string `json:"path"`
	FilePath string `json:"filePath"`
}

type forgeDataValue struct {
	Client string `json:"client"`
	Server string `json:"server"`
}

type forgeProcessor struct {
	Sides     []string          `json:"sides"`
	Jar       string            `json:"jar"`
	Classpath []string          `json:"classpath"`
	Args      []string          `json:"args"`
	Outputs   map[string]string `json:"outputs"`
}

type mavenCoordinate struct {
	Group      string
	Artifact   string
	Version    string
	Classifier string
	Extension  string
}

func (s *Service) InstallForgeLoader(ctx context.Context, profile domain.Profile, javaPath string, progress ProgressFunc) (string, error) {
	return s.installForgeLikeLoader(ctx, profile, javaPath, progress, forgeInstallerProvider)
}

func (s *Service) InstallNeoForgeLoader(ctx context.Context, profile domain.Profile, javaPath string, progress ProgressFunc) (string, error) {
	return s.installForgeLikeLoader(ctx, profile, javaPath, progress, neoForgeInstallerProvider)
}

func (s *Service) installForgeLikeLoader(ctx context.Context, profile domain.Profile, javaPath string, progress ProgressFunc, provider forgeLikeInstallerProvider) (string, error) {
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
		loaderVersion, err = s.latestForgeLikeLoaderVersion(ctx, profile.MinecraftVersion, provider)
		if err != nil {
			return "", err
		}
	} else {
		loaderVersion = provider.NormalizeLoaderVersion(profile.MinecraftVersion, loaderVersion)
	}

	emit(domain.InstallProgress{
		Stage:   provider.Stage + "-metadata",
		Message: "Resolving " + provider.Name + " installer metadata",
		Percent: 80,
	})

	installerPath, err := s.ensureForgeInstaller(ctx, loaderVersion, provider)
	if err != nil {
		return "", err
	}
	forge, installProfile, err := readForgeInstallerProfile(installerPath)
	if err != nil {
		return "", err
	}

	base, err := s.loadInstalledVersion(profile.MinecraftVersion)
	if err != nil {
		return "", err
	}

	normalizeLibraryDownloads(forge.Libraries)
	normalizeLibraryDownloads(installProfile.Libraries)
	if err := s.extractForgeInstallerData(installerPath, loaderVersion, installProfile, provider); err != nil {
		return "", err
	}
	if err := s.ensureForgeLibraries(ctx, append(append([]libraryMetadata{}, installProfile.Libraries...), forge.Libraries...), emit, provider); err != nil {
		return "", err
	}
	if err := s.ensureForgeProcessorArtifacts(ctx, installProfile, provider); err != nil {
		return "", err
	}
	if err := s.runForgeClientProcessors(ctx, javaPath, installerPath, loaderVersion, profile, installProfile, emit, provider); err != nil {
		return "", err
	}

	versionID := provider.VersionID(profile.MinecraftVersion, requestedLoaderVersion)
	composite := mergeFabricProfile(base, forge, versionID)
	normalizeLibraryDownloads(composite.Libraries)
	if err := s.writeVersionMetadata(versionID, composite); err != nil {
		return "", err
	}

	emit(domain.InstallProgress{
		Stage:   provider.Stage + "-complete",
		Message: provider.Name + " loader installed",
		Percent: 100,
		Done:    true,
	})
	return loaderVersion, nil
}

func (s *Service) latestForgeLikeLoaderVersion(ctx context.Context, minecraftVersion string, provider forgeLikeInstallerProvider) (string, error) {
	versions, err := s.fetchForgeLikeMavenVersions(ctx, provider)
	if err != nil {
		return "", err
	}
	for _, version := range versions {
		if provider.MinecraftVersion(version) == minecraftVersion {
			return version, nil
		}
	}
	return "", fmt.Errorf("no %s loader version found for Minecraft %s", provider.Name, minecraftVersion)
}

func (s *Service) fetchForgeLikeMavenVersions(ctx context.Context, provider forgeLikeInstallerProvider) ([]string, error) {
	raw, err := s.getRaw(ctx, provider.MavenMetadataURL)
	if err != nil {
		return nil, err
	}
	var metadata forgeMetadata
	if err := xml.Unmarshal(raw, &metadata); err != nil {
		return nil, err
	}
	versions := append([]string{}, metadata.Versioning.Versions...)
	sort.SliceStable(versions, func(left int, right int) bool {
		return compareLoaderVersions(versions[left], versions[right]) > 0
	})
	return versions, nil
}

func (s *Service) ensureForgeInstaller(ctx context.Context, loaderVersion string, provider forgeLikeInstallerProvider) (string, error) {
	path, ok := mavenArtifactPathWithExtension(mavenCoordinate{
		Group:      provider.Group,
		Artifact:   provider.Artifact,
		Version:    loaderVersion,
		Classifier: "installer",
		Extension:  "jar",
	})
	if !ok {
		return "", fmt.Errorf("invalid %s version %q", provider.Name, loaderVersion)
	}

	url := strings.TrimRight(provider.MavenBaseURL, "/") + "/" + path
	sha1 := ""
	if raw, err := s.fetchOptionalText(ctx, url+".sha1"); err == nil {
		if fields := strings.Fields(raw); len(fields) > 0 {
			sha1 = fields[0]
		}
	}
	target := filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(path))
	return target, s.ensureDownload(ctx, downloadItem{
		URL:   url,
		Path:  target,
		SHA1:  sha1,
		Label: provider.Name + " installer " + loaderVersion,
	})
}

func (s *Service) fetchOptionalText(ctx context.Context, url string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "Power-Mine-Launcher/0.1")
	response, err := s.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("metadata request failed: %s", response.Status)
	}
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func readForgeInstallerProfile(installerPath string) (versionMetadata, forgeInstallProfile, error) {
	reader, err := zip.OpenReader(installerPath)
	if err != nil {
		return versionMetadata{}, forgeInstallProfile{}, err
	}
	defer reader.Close()

	installRaw, err := readZipEntry(reader.File, "install_profile.json")
	if err != nil {
		return versionMetadata{}, forgeInstallProfile{}, err
	}

	var installProfile forgeInstallProfile
	if err := json.Unmarshal(installRaw, &installProfile); err != nil {
		return versionMetadata{}, forgeInstallProfile{}, err
	}

	var version versionMetadata
	versionRaw, versionErr := readZipEntry(reader.File, "version.json")
	if versionErr == nil {
		if err := json.Unmarshal(versionRaw, &version); err != nil {
			return versionMetadata{}, forgeInstallProfile{}, err
		}
	} else if installProfile.VersionInfo.ID != "" {
		version = installProfile.VersionInfo
	} else {
		return versionMetadata{}, forgeInstallProfile{}, versionErr
	}
	return version, installProfile, nil
}

func (s *Service) ensureForgeLibraries(ctx context.Context, libraries []libraryMetadata, emit func(domain.InstallProgress), provider forgeLikeInstallerProvider) error {
	plan := s.buildLibraryDownloadPlan(libraries)
	total := len(plan)
	for index, item := range plan {
		current := index + 1
		percent := 82
		if total > 0 {
			percent = 82 + current*6/total
		}
		emit(domain.InstallProgress{
			Stage:   provider.Stage + "-download",
			Message: item.Label,
			Current: current,
			Total:   total,
			Percent: percent,
		})
		if err := s.ensureDownload(ctx, item); err != nil {
			return fmt.Errorf("%s: %w", item.Label, err)
		}
	}
	return nil
}

func (s *Service) ensureForgeProcessorArtifacts(ctx context.Context, installProfile forgeInstallProfile, provider forgeLikeInstallerProvider) error {
	coordinates := map[string]bool{}
	for _, processor := range installProfile.Processors {
		if processor.Jar != "" {
			coordinates[processor.Jar] = true
		}
		for _, item := range processor.Classpath {
			coordinates[item] = true
		}
		for _, arg := range processor.Args {
			if strings.HasPrefix(arg, "[") && strings.HasSuffix(arg, "]") {
				coordinates[strings.Trim(arg, "[]")] = true
			}
		}
	}
	for raw := range coordinates {
		path, ok := mavenArtifactPathFromSpec(raw)
		if !ok {
			continue
		}
		target := filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(path))
		if _, err := os.Stat(target); err == nil {
			continue
		}
		if err := s.ensureDownload(ctx, downloadItem{
			URL:   strings.TrimRight(provider.MavenBaseURL, "/") + "/" + path,
			Path:  target,
			Label: provider.Name + " processor artifact " + raw,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) extractForgeInstallerData(installerPath string, loaderVersion string, installProfile forgeInstallProfile, provider forgeLikeInstallerProvider) error {
	reader, err := zip.OpenReader(installerPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	if installProfile.Install.Path != "" && installProfile.Install.FilePath != "" {
		targetPath, ok := mavenArtifactPathFromSpec(installProfile.Install.Path)
		if !ok {
			return fmt.Errorf("invalid Forge install artifact path %q", installProfile.Install.Path)
		}
		sourcePath := filepath.Clean(filepath.FromSlash(installProfile.Install.FilePath))
		if !filepath.IsLocal(sourcePath) {
			return fmt.Errorf("invalid Forge install file path %q", installProfile.Install.FilePath)
		}
		target := filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(targetPath))
		if valid, err := validFile(target, downloadItem{}); err != nil {
			return err
		} else if !valid {
			if err := extractZipEntry(reader.File, filepath.ToSlash(sourcePath), target); err != nil {
				return err
			}
		}
	}

	for _, data := range installProfile.Data {
		value := data.Client
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, "/") {
			continue
		}
		entryName := strings.TrimPrefix(value, "/")
		target, ok := s.forgeLikeInstallerDataPath(loaderVersion, value, provider)
		if !ok {
			continue
		}
		if err := extractZipEntry(reader.File, entryName, target); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) runForgeClientProcessors(
	ctx context.Context,
	javaPath string,
	installerPath string,
	loaderVersion string,
	profile domain.Profile,
	installProfile forgeInstallProfile,
	emit func(domain.InstallProgress),
	provider forgeLikeInstallerProvider,
) error {
	if strings.TrimSpace(javaPath) == "" {
		javaPath = "java"
	}

	variables := s.forgeProcessorVariables(installerPath, loaderVersion, profile, installProfile, provider)
	total := len(clientForgeProcessors(installProfile.Processors))
	current := 0
	for _, processor := range installProfile.Processors {
		if !forgeProcessorSideAllowed(processor, "client") {
			continue
		}
		current++
		if forgeProcessorOutputsValid(processor, variables) {
			continue
		}
		if err := prepareForgeProcessorOutputs(processor, variables); err != nil {
			return err
		}
		mainJar, classpath, err := s.forgeProcessorClasspath(processor)
		if err != nil {
			return err
		}
		mainClass, err := jarMainClass(mainJar)
		if err != nil {
			return err
		}
		args := make([]string, 0, len(processor.Args))
		for _, arg := range processor.Args {
			args = append(args, s.resolveForgeProcessorValue(arg, loaderVersion, variables))
		}
		if err := prepareForgeProcessorArgOutputs(processor.Args, args); err != nil {
			return err
		}

		percent := 88
		if total > 0 {
			percent = 88 + current*10/total
		}
		emit(domain.InstallProgress{
			Stage:   provider.Stage + "-process",
			Message: fmt.Sprintf("Running %s processor %d/%d", provider.Name, current, total),
			Current: current,
			Total:   total,
			Percent: percent,
		})

		commandArgs := append([]string{"-Xmx1G", "-cp", strings.Join(classpath, string(os.PathListSeparator)), mainClass}, args...)
		command := exec.CommandContext(ctx, javaPath, commandArgs...)
		command.Dir = s.minecraftDir()
		output, err := command.CombinedOutput()
		if err != nil {
			text := strings.TrimSpace(string(output))
			if text != "" {
				return fmt.Errorf("%s processor %s failed: %w: %s", provider.Name, processor.Jar, err, text)
			}
			return fmt.Errorf("%s processor %s failed: %w", provider.Name, processor.Jar, err)
		}
	}
	return nil
}

func clientForgeProcessors(processors []forgeProcessor) []forgeProcessor {
	var filtered []forgeProcessor
	for _, processor := range processors {
		if forgeProcessorSideAllowed(processor, "client") {
			filtered = append(filtered, processor)
		}
	}
	return filtered
}

func forgeProcessorSideAllowed(processor forgeProcessor, side string) bool {
	if len(processor.Sides) == 0 {
		return true
	}
	for _, item := range processor.Sides {
		if item == side {
			return true
		}
	}
	return false
}

func (s *Service) forgeProcessorClasspath(processor forgeProcessor) (string, []string, error) {
	mainJar, ok := s.forgeArtifactPath(processor.Jar)
	if !ok {
		return "", nil, fmt.Errorf("invalid Forge processor artifact %q", processor.Jar)
	}
	classpath := []string{mainJar}
	for _, item := range processor.Classpath {
		path, ok := s.forgeArtifactPath(item)
		if !ok {
			return "", nil, fmt.Errorf("invalid Forge processor classpath artifact %q", item)
		}
		classpath = append(classpath, path)
	}
	return mainJar, classpath, nil
}

func (s *Service) forgeArtifactPath(raw string) (string, bool) {
	path, ok := mavenArtifactPathFromSpec(raw)
	if !ok {
		return "", false
	}
	return filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(path)), true
}

func (s *Service) forgeProcessorVariables(installerPath string, loaderVersion string, profile domain.Profile, installProfile forgeInstallProfile, provider forgeLikeInstallerProvider) map[string]string {
	libraryDir := filepath.Join(s.minecraftDir(), "libraries")
	variables := map[string]string{
		"SIDE":              "client",
		"ROOT":              s.minecraftDir(),
		"INSTALLER":         installerPath,
		"LIBRARY_DIR":       libraryDir,
		"MINECRAFT_VERSION": profile.MinecraftVersion,
		"MINECRAFT_JAR":     filepath.Join(s.minecraftDir(), "versions", profile.MinecraftVersion, profile.MinecraftVersion+".jar"),
	}
	for key, data := range installProfile.Data {
		value := data.Client
		if value == "" {
			value = data.Server
		}
		variables[key] = s.resolveForgeLikeDataValue(loaderVersion, value, provider)
	}
	return variables
}

func (s *Service) resolveForgeDataValue(forgeVersion string, value string) string {
	return s.resolveForgeLikeDataValue(forgeVersion, value, forgeInstallerProvider)
}

func (s *Service) resolveForgeLikeDataValue(loaderVersion string, value string, provider forgeLikeInstallerProvider) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") {
		return strings.Trim(value, "'")
	}
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		path, ok := mavenArtifactPathFromSpec(strings.Trim(value, "[]"))
		if ok {
			return filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(path))
		}
	}
	if strings.HasPrefix(value, "/") {
		path, ok := s.forgeLikeInstallerDataPath(loaderVersion, value, provider)
		if ok {
			return path
		}
	}
	return value
}

func (s *Service) resolveForgeProcessorValue(value string, forgeVersion string, variables map[string]string) string {
	resolved := value
	for key, replacement := range variables {
		resolved = strings.ReplaceAll(resolved, "{"+key+"}", replacement)
	}
	if strings.HasPrefix(resolved, "[") && strings.HasSuffix(resolved, "]") {
		path, ok := mavenArtifactPathFromSpec(strings.Trim(resolved, "[]"))
		if ok {
			return filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(path))
		}
	}
	return resolved
}

func (s *Service) forgeInstallerDataPath(forgeVersion string, value string) (string, bool) {
	return s.forgeLikeInstallerDataPath(forgeVersion, value, forgeInstallerProvider)
}

func (s *Service) forgeLikeInstallerDataPath(loaderVersion string, value string, provider forgeLikeInstallerProvider) (string, bool) {
	cleanPath := filepath.Clean(filepath.FromSlash(strings.TrimPrefix(value, "/")))
	if !filepath.IsLocal(cleanPath) {
		return "", false
	}
	groupPath := filepath.Join(strings.Split(provider.Group, ".")...)
	return filepath.Join(s.minecraftDir(), "libraries", groupPath, provider.Artifact, loaderVersion, "installer-data", cleanPath), true
}

func forgeProcessorOutputsValid(processor forgeProcessor, variables map[string]string) bool {
	if len(processor.Outputs) == 0 {
		return false
	}
	for output, expected := range processor.Outputs {
		path := resolveForgeOutputPath(output, variables)
		if path == "" {
			return false
		}
		sha1 := strings.Trim(resolveForgeOutputPath(expected, variables), "'")
		if valid, err := validFile(path, downloadItem{SHA1: sha1}); err != nil || !valid {
			return false
		}
	}
	return true
}

func prepareForgeProcessorOutputs(processor forgeProcessor, variables map[string]string) error {
	for output := range processor.Outputs {
		path := resolveForgeOutputPath(output, variables)
		if path == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func prepareForgeProcessorArgOutputs(rawArgs []string, resolvedArgs []string) error {
	outputFlags := map[string]bool{
		"--output": true,
		"--slim":   true,
		"--extra":  true,
	}
	for index := 1; index < len(resolvedArgs); index++ {
		if !outputFlags[rawArgs[index-1]] {
			continue
		}
		path := resolvedArgs[index]
		if path == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func resolveForgeOutputPath(value string, variables map[string]string) string {
	resolved := value
	for key, replacement := range variables {
		resolved = strings.ReplaceAll(resolved, "{"+key+"}", replacement)
	}
	return resolved
}

func jarMainClass(path string) (string, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	raw, err := readZipEntry(reader.File, "META-INF/MANIFEST.MF")
	if err != nil {
		return "", err
	}
	attributes := parseManifestAttributes(raw)
	if mainClass := attributes["Main-Class"]; mainClass != "" {
		return mainClass, nil
	}
	return "", fmt.Errorf("jar %s is missing Main-Class", path)
}

func parseManifestAttributes(raw []byte) map[string]string {
	attributes := map[string]string{}
	var currentKey string
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line == "" {
			currentKey = ""
			continue
		}
		if strings.HasPrefix(line, " ") && currentKey != "" {
			attributes[currentKey] += strings.TrimPrefix(line, " ")
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		currentKey = key
		attributes[key] = strings.TrimSpace(value)
	}
	return attributes
}

func readZipEntry(files []*zip.File, name string) ([]byte, error) {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return nil, fmt.Errorf("zip entry %s was not found", name)
}

func extractZipEntry(files []*zip.File, name string, targetPath string) error {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		defer reader.Close()
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
		if _, err := io.Copy(tmp, reader); err != nil {
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
	return fmt.Errorf("zip entry %s was not found", name)
}

func mavenArtifactPathFromSpec(raw string) (string, bool) {
	coordinate, ok := parseMavenCoordinate(raw)
	if !ok {
		return "", false
	}
	return mavenArtifactPathWithExtension(coordinate)
}

func parseMavenCoordinate(raw string) (mavenCoordinate, bool) {
	raw = strings.TrimSpace(strings.Trim(raw, "[]"))
	if raw == "" {
		return mavenCoordinate{}, false
	}
	extension := "jar"
	if before, after, ok := strings.Cut(raw, "@"); ok {
		raw = before
		extension = after
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return mavenCoordinate{}, false
	}
	coordinate := mavenCoordinate{
		Group:     parts[0],
		Artifact:  parts[1],
		Version:   parts[2],
		Extension: extension,
	}
	if len(parts) == 4 {
		coordinate.Classifier = parts[3]
	}
	if coordinate.Group == "" || coordinate.Artifact == "" || coordinate.Version == "" || coordinate.Extension == "" {
		return mavenCoordinate{}, false
	}
	return coordinate, true
}

func mavenArtifactPathWithExtension(coordinate mavenCoordinate) (string, bool) {
	if coordinate.Group == "" || coordinate.Artifact == "" || coordinate.Version == "" {
		return "", false
	}
	extension := coordinate.Extension
	if extension == "" {
		extension = "jar"
	}
	fileName := coordinate.Artifact + "-" + coordinate.Version
	if coordinate.Classifier != "" {
		fileName += "-" + coordinate.Classifier
	}
	fileName += "." + extension
	pathParts := append(strings.Split(coordinate.Group, "."), coordinate.Artifact, coordinate.Version, fileName)
	return strings.Join(pathParts, "/"), true
}

func ForgeVersionID(minecraftVersion string, loaderVersion string) string {
	minecraftVersion = strings.TrimSpace(minecraftVersion)
	loaderVersion = strings.TrimSpace(loaderVersion)
	if loaderVersion == "" {
		loaderVersion = "latest"
	}
	if strings.HasPrefix(loaderVersion, minecraftVersion+"-") {
		loaderVersion = strings.TrimPrefix(loaderVersion, minecraftVersion+"-")
	}
	return minecraftVersion + "-forge-" + loaderVersion
}

func NeoForgeVersionID(minecraftVersion string, loaderVersion string) string {
	minecraftVersion = strings.TrimSpace(minecraftVersion)
	loaderVersion = strings.TrimSpace(loaderVersion)
	if loaderVersion == "" {
		loaderVersion = "latest"
	}
	return minecraftVersion + "-neoforge-" + loaderVersion
}

func forgeMinecraftVersion(version string) string {
	minecraftVersion, _, ok := strings.Cut(strings.TrimSpace(version), "-")
	if !ok {
		return ""
	}
	return minecraftVersion
}

func neoForgeMinecraftVersion(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return ""
	}
	major := leadingDigits(parts[0])
	minor := leadingDigits(parts[1])
	if major == "" || minor == "" {
		return ""
	}
	return "1." + major + "." + minor
}

func leadingDigits(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char < '0' || char > '9' {
			break
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func compareLoaderVersions(left string, right string) int {
	leftParts := splitLoaderVersionParts(left)
	rightParts := splitLoaderVersionParts(right)
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for index := 0; index < max; index++ {
		leftPart := loaderVersionPart{}
		rightPart := loaderVersionPart{}
		if index < len(leftParts) {
			leftPart = leftParts[index]
		}
		if index < len(rightParts) {
			rightPart = rightParts[index]
		}
		if leftPart.Number != rightPart.Number {
			if leftPart.Number > rightPart.Number {
				return 1
			}
			return -1
		}
		if leftPart.Suffix != rightPart.Suffix {
			if leftPart.Suffix == "" {
				return 1
			}
			if rightPart.Suffix == "" {
				return -1
			}
			if leftPart.Suffix > rightPart.Suffix {
				return 1
			}
			return -1
		}
	}
	return 0
}

type loaderVersionPart struct {
	Number int
	Suffix string
}

func splitLoaderVersionParts(version string) []loaderVersionPart {
	fields := strings.FieldsFunc(version, func(char rune) bool {
		return char == '.' || char == '-'
	})
	parts := make([]loaderVersionPart, 0, len(fields))
	for _, field := range fields {
		part := loaderVersionPart{}
		for _, char := range field {
			if char < '0' || char > '9' {
				part.Suffix += string(char)
				continue
			}
			if part.Suffix == "" {
				part.Number = part.Number*10 + int(char-'0')
			} else {
				part.Suffix += string(char)
			}
		}
		parts = append(parts, part)
	}
	return parts
}
