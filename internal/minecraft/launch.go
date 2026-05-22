package minecraft

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"power-mine/internal/account"
	"power-mine/internal/domain"
)

type LaunchOptions struct {
	JavaPath string
	Memory   domain.MemorySettings
	Account  domain.AccountConfig
}

type LaunchCommand struct {
	JavaPath string
	Args     []string
	WorkDir  string
}

func (s *Service) BuildVanillaLaunchCommand(ctx context.Context, profile domain.Profile, options LaunchOptions) (LaunchCommand, error) {
	return s.BuildLaunchCommand(ctx, profile, options)
}

func (s *Service) BuildLaunchCommand(ctx context.Context, profile domain.Profile, options LaunchOptions) (LaunchCommand, error) {
	if profile.Loader.Type != domain.LoaderVanilla && profile.Loader.Type != domain.LoaderFabric && profile.Loader.Type != domain.LoaderQuilt && profile.Loader.Type != domain.LoaderForge && profile.Loader.Type != domain.LoaderNeoForge {
		return LaunchCommand{}, fmt.Errorf("loader %q launch is not implemented yet", profile.Loader.Type)
	}
	options.JavaPath = strings.TrimSpace(options.JavaPath)
	if options.JavaPath == "" {
		options.JavaPath = "java"
	}

	versionID := s.launchVersionID(profile)
	version, err := s.loadInstalledVersion(versionID)
	if err != nil {
		return LaunchCommand{}, err
	}
	if version.MainClass == "" {
		return LaunchCommand{}, errors.New("version metadata is missing mainClass")
	}
	if err := s.prepareInstalledAssetViews(version); err != nil {
		return LaunchCommand{}, err
	}

	if err := os.MkdirAll(profile.GameDir, 0o755); err != nil {
		return LaunchCommand{}, err
	}

	nativesDir, err := s.prepareNatives(ctx, version)
	if err != nil {
		return LaunchCommand{}, err
	}

	classpath, err := s.classpath(version, profile.Loader.Type)
	if err != nil {
		return LaunchCommand{}, err
	}

	vars := s.launchVariables(profile, version, nativesDir, classpath, options.Account)
	jvmArgs := resolveArgumentList(version.Arguments.JVM, vars)
	if len(jvmArgs) == 0 {
		jvmArgs = legacyJVMArgs(nativesDir, classpath)
	}
	if loggingArg := s.loggingArgument(version); loggingArg != "" {
		jvmArgs = append([]string{loggingArg}, jvmArgs...)
	}

	memory := options.Memory
	if memory.MinMB <= 0 {
		memory.MinMB = profile.Memory.MinMB
	}
	if memory.MaxMB <= 0 {
		memory.MaxMB = profile.Memory.MaxMB
	}
	if memory.MaxMB > 0 {
		jvmArgs = append([]string{fmt.Sprintf("-Xmx%dM", memory.MaxMB)}, jvmArgs...)
	}
	if memory.MinMB > 0 {
		jvmArgs = append([]string{fmt.Sprintf("-Xms%dM", memory.MinMB)}, jvmArgs...)
	}

	gameArgs := resolveArgumentList(version.Arguments.Game, vars)
	if len(gameArgs) == 0 && version.MinecraftArguments != "" {
		gameArgs = resolvePlaceholders(strings.Fields(version.MinecraftArguments), vars)
	}

	args := make([]string, 0, len(jvmArgs)+1+len(gameArgs))
	args = append(args, jvmArgs...)
	args = append(args, version.MainClass)
	args = append(args, gameArgs...)

	return LaunchCommand{
		JavaPath: options.JavaPath,
		Args:     args,
		WorkDir:  profile.GameDir,
	}, nil
}

func (s *Service) loadInstalledVersion(versionID string) (versionMetadata, error) {
	path := filepath.Join(s.minecraftDir(), "versions", versionID, versionID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return versionMetadata{}, fmt.Errorf("version %q is not installed; install the profile first", versionID)
		}
		return versionMetadata{}, err
	}

	var version versionMetadata
	if err := json.Unmarshal(raw, &version); err != nil {
		return versionMetadata{}, err
	}
	return version, nil
}

func (s *Service) prepareNatives(ctx context.Context, version versionMetadata) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	nativesDir := filepath.Join(s.minecraftDir(), "natives", version.ID+"-"+runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.RemoveAll(nativesDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(nativesDir, 0o755); err != nil {
		return "", err
	}

	for _, library := range version.Libraries {
		if !libraryAllowed(library.Rules) {
			continue
		}
		classifier, ok := nativeClassifier(library)
		if !ok {
			continue
		}
		artifact, ok := library.Downloads.Classifiers[classifier]
		if !ok || artifact.Path == "" {
			continue
		}
		if err := extractNativeJar(filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(artifact.Path)), nativesDir, library.Extract.Exclude); err != nil {
			return "", fmt.Errorf("extract native %s: %w", library.Name, err)
		}
	}

	return nativesDir, nil
}

func (s *Service) classpath(version versionMetadata, loader domain.LoaderType) (string, error) {
	entries := make([]string, 0, len(version.Libraries)+1)
	seen := map[string]bool{}
	addEntry := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		entries = append(entries, path)
	}
	for _, library := range version.Libraries {
		if !libraryAllowed(library.Rules) {
			continue
		}
		if library.Downloads.Artifact.Path == "" {
			continue
		}
		path := filepath.Join(s.minecraftDir(), "libraries", filepath.FromSlash(library.Downloads.Artifact.Path))
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("library %s is missing: %w", library.Name, err)
		}
		addEntry(path)
	}

	if loader != domain.LoaderForge && loader != domain.LoaderNeoForge {
		clientVersionID := version.ID
		if version.InheritsFrom != "" {
			clientVersionID = version.InheritsFrom
		}
		clientJar := filepath.Join(s.minecraftDir(), "versions", clientVersionID, clientVersionID+".jar")
		if _, err := os.Stat(clientJar); err != nil {
			return "", fmt.Errorf("client jar is missing: %w", err)
		}
		addEntry(clientJar)
	}
	return strings.Join(entries, string(os.PathListSeparator)), nil
}

func (s *Service) launchVersionID(profile domain.Profile) string {
	if profile.Loader.Type == domain.LoaderFabric {
		return FabricVersionID(profile.MinecraftVersion, profile.Loader.Version)
	}
	if profile.Loader.Type == domain.LoaderQuilt {
		return QuiltVersionID(profile.MinecraftVersion, profile.Loader.Version)
	}
	if profile.Loader.Type == domain.LoaderForge {
		return ForgeVersionID(profile.MinecraftVersion, profile.Loader.Version)
	}
	if profile.Loader.Type == domain.LoaderNeoForge {
		return NeoForgeVersionID(profile.MinecraftVersion, profile.Loader.Version)
	}
	return profile.MinecraftVersion
}

func (s *Service) launchVariables(profile domain.Profile, version versionMetadata, nativesDir string, classpath string, accountConfig domain.AccountConfig) map[string]string {
	if accountConfig.Mode == "" {
		accountConfig = profile.Account
	}
	accountConfig = account.Normalize(accountConfig)
	if accountConfig.Mode != domain.AccountOffline {
		accountConfig = account.Default()
	}

	assetsIndex := version.AssetIndex.ID
	if assetsIndex == "" {
		assetsIndex = version.Assets
	}

	return map[string]string{
		"auth_player_name":    accountConfig.OfflineName,
		"auth_uuid":           strings.ReplaceAll(accountConfig.OfflineUUID, "-", ""),
		"auth_access_token":   "0",
		"auth_session":        "0",
		"user_type":           "legacy",
		"version_name":        version.ID,
		"version_type":        version.Type,
		"game_directory":      profile.GameDir,
		"assets_root":         filepath.Join(s.minecraftDir(), "assets"),
		"assets_index_name":   assetsIndex,
		"game_assets":         filepath.Join(s.minecraftDir(), "assets", "virtual", assetsIndex),
		"natives_directory":   nativesDir,
		"launcher_name":       "Power Mine",
		"launcher_version":    "0.1.0-dev",
		"classpath":           classpath,
		"classpath_separator": string(os.PathListSeparator),
		"library_directory":   filepath.Join(s.minecraftDir(), "libraries"),
		"clientid":            "",
		"auth_xuid":           "",
	}
}

func (s *Service) loggingArgument(version versionMetadata) string {
	if version.Logging.Client.Argument == "" || version.Logging.Client.File.ID == "" {
		return ""
	}
	path := filepath.Join(s.minecraftDir(), "assets", "log_configs", version.Logging.Client.File.ID)
	return strings.ReplaceAll(version.Logging.Client.Argument, "${path}", path)
}

func resolveArgumentList(raw []json.RawMessage, vars map[string]string) []string {
	var args []string
	for _, item := range raw {
		var text string
		if err := json.Unmarshal(item, &text); err == nil {
			args = append(args, replacePlaceholders(text, vars))
			continue
		}

		var object struct {
			Rules []rule          `json:"rules"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(item, &object); err != nil || !argumentRulesAllowed(object.Rules) {
			continue
		}

		var value string
		if err := json.Unmarshal(object.Value, &value); err == nil {
			args = append(args, replacePlaceholders(value, vars))
			continue
		}

		var values []string
		if err := json.Unmarshal(object.Value, &values); err == nil {
			args = append(args, resolvePlaceholders(values, vars)...)
		}
	}
	return args
}

func argumentRulesAllowed(rules []rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, rule := range rules {
		if len(rule.Features) > 0 {
			continue
		}
		if ruleMatches(rule) {
			allowed = rule.Action == "allow"
		}
	}
	return allowed
}

func resolvePlaceholders(values []string, vars map[string]string) []string {
	resolved := make([]string, 0, len(values))
	for _, value := range values {
		resolved = append(resolved, replacePlaceholders(value, vars))
	}
	return resolved
}

func replacePlaceholders(value string, vars map[string]string) string {
	for key, replacement := range vars {
		value = strings.ReplaceAll(value, "${"+key+"}", replacement)
	}
	return value
}

func legacyJVMArgs(nativesDir string, classpath string) []string {
	return []string{
		"-Djava.library.path=" + nativesDir,
		"-cp",
		classpath,
	}
}

func extractNativeJar(path string, targetDir string, excludes []string) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || nativeExcluded(file.Name, excludes) {
			continue
		}
		cleanName := filepath.Clean(file.Name)
		if !filepath.IsLocal(cleanName) {
			continue
		}
		targetPath := filepath.Join(targetDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(file, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(file *zip.File, targetPath string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	mode := file.FileInfo().Mode()
	if mode == 0 {
		mode = 0o644
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer target.Close()

	_, err = io.Copy(target, source)
	return err
}

func nativeExcluded(path string, excludes []string) bool {
	for _, exclude := range excludes {
		if strings.HasPrefix(path, exclude) {
			return true
		}
	}
	return false
}
