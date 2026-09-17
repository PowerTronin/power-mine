package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"sync"
	"time"

	"power-mine/internal/account"
	"power-mine/internal/catalog"
	"power-mine/internal/domain"
	"power-mine/internal/javasvc"
	"power-mine/internal/minecraft"
	"power-mine/internal/modpacks"
	"power-mine/internal/mods"
	"power-mine/internal/platform"
	"power-mine/internal/profiles"
	"power-mine/internal/servers"
	"power-mine/internal/settings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx              context.Context
	mu               sync.RWMutex
	launchMu         sync.Mutex
	settingsService  *settings.Service
	profileService   *profiles.Service
	catalogService   *catalog.Service
	minecraftService *minecraft.Service
	javaService      *javasvc.Service
	modpackService   *modpacks.Service
	modsService      *mods.Service
	serverService    *servers.Service
	running          map[string]*exec.Cmd
	launchDone       map[string]chan struct{}
	serverInputs     map[string]io.WriteCloser
	stopping         map[string]bool
	externalMu       sync.RWMutex
	externalServer   *http.Server
	externalBaseURL  string
	externalToken    string
	logsSnapshot     string
	serverEvents     map[string][]domain.LocalServerEvent
	startupErr       error
	headless         bool
}

const (
	maxModrinthDependencyDepth      = 12
	localServerStartupFailureWindow = 15 * time.Second
	maxLocalServerTerminalEvents    = 1000
)

type modrinthInstallState struct {
	seenProjects         map[string]bool
	seenVersions         map[string]bool
	limitDependencies    bool
	selectedDependencies map[string]bool
	installedFiles       []domain.ModrinthInstalledFile
	skippedDependencies  []domain.ModrinthSkippedDependency
}

type modrinthInstallPlanState struct {
	seenProjects         map[string]bool
	seenVersions         map[string]bool
	requiredDependencies []domain.ModrinthRequiredDependency
	skippedDependencies  []domain.ModrinthSkippedDependency
}

func NewApp() *App {
	return &App{
		running:      make(map[string]*exec.Cmd),
		launchDone:   make(map[string]chan struct{}),
		serverInputs: make(map[string]io.WriteCloser),
		stopping:     make(map[string]bool),
		serverEvents: make(map[string][]domain.LocalServerEvent),
	}
}

func (a *App) startup(ctx context.Context) {
	dataDir, err := platform.AppDataDir()
	if err != nil {
		a.startupErr = err
		return
	}
	a.initServices(ctx, dataDir)
}

func (a *App) initServices(ctx context.Context, dataDir string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ctx = ctx
	a.settingsService = settings.NewService(dataDir)
	a.profileService = profiles.NewService(dataDir)
	a.catalogService = catalog.NewService(dataDir)
	a.minecraftService = minecraft.NewService(dataDir)
	a.javaService = javasvc.NewService(dataDir)
	a.modpackService = modpacks.NewService()
	a.modsService = mods.NewService()
	a.serverService = servers.NewService(dataDir)
	a.startupErr = nil
}

func (a *App) shutdown(ctx context.Context) {
	a.stopRunningLocalServers(15 * time.Second)
	a.shutdownExternalWindowServer(ctx)
}

func (a *App) AppInfo() domain.AppInfo {
	return domain.AppInfo{
		Name:    platform.AppName,
		Version: "0.2.0",
	}
}

func (a *App) GetSettings() (domain.Settings, error) {
	if err := a.ensureReady(); err != nil {
		return domain.Settings{}, err
	}
	return a.settingsService.Get()
}

func (a *App) SaveSettings(next domain.Settings) (domain.Settings, error) {
	if err := a.ensureReady(); err != nil {
		return domain.Settings{}, err
	}
	return a.settingsService.Save(next)
}

func (a *App) GetAccount() (domain.AccountConfig, error) {
	if err := a.ensureReady(); err != nil {
		return domain.AccountConfig{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.AccountConfig{}, err
	}
	return currentSettings.Account, nil
}

func (a *App) SaveAccount(next domain.AccountConfig) (domain.AccountConfig, error) {
	if err := a.ensureReady(); err != nil {
		return domain.AccountConfig{}, err
	}
	next = account.Normalize(next)
	if err := account.Validate(next); err != nil {
		return domain.AccountConfig{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.AccountConfig{}, err
	}
	currentSettings.Account = next
	saved, err := a.settingsService.Save(currentSettings)
	if err != nil {
		return domain.AccountConfig{}, err
	}
	return saved.Account, nil
}

func (a *App) ListProfiles() (domain.ProfileList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ProfileList{}, err
	}
	return a.profileService.List()
}

func (a *App) CreateProfile(input domain.ProfileInput) (domain.Profile, error) {
	if err := a.ensureReady(); err != nil {
		return domain.Profile{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.Profile{}, err
	}
	return a.profileService.Create(input, currentSettings.DefaultMemory)
}

func (a *App) UpdateProfile(id string, input domain.ProfileInput) (domain.Profile, error) {
	if err := a.ensureReady(); err != nil {
		return domain.Profile{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.Profile{}, err
	}
	return a.profileService.Update(id, input, currentSettings.DefaultMemory)
}

func (a *App) DeleteProfile(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	return a.profileService.Delete(id)
}

func (a *App) SelectProfile(id string) (domain.ProfileList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ProfileList{}, err
	}
	return a.profileService.Select(id)
}

func (a *App) ListLocalServers() (domain.LocalServerList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LocalServerList{}, err
	}
	return a.serverService.List()
}

func (a *App) CreateLocalServer(input domain.LocalServerInput) (domain.LocalServer, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LocalServer{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.LocalServer{}, err
	}
	return a.serverService.Create(input, currentSettings.DefaultMemory)
}

func (a *App) InstallLocalServer(id string) (domain.LocalServer, error) {
	return a.installLocalServer(id, false)
}

func (a *App) RepairLocalServer(id string) (domain.LocalServer, error) {
	return a.installLocalServer(id, true)
}

func (a *App) installLocalServer(id string, repair bool) (domain.LocalServer, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LocalServer{}, err
	}

	server, err := a.serverService.Get(id)
	if err != nil {
		return domain.LocalServer{}, err
	}

	status := "installing"
	startMessage := "Installing local server"
	progressMessage := "Starting local server install"
	failMessage := "Local server install failed"
	successMessage := "Local server ready"
	if repair {
		status = "repairing"
		startMessage = "Checking and repairing local server"
		progressMessage = "Starting local server repair"
		failMessage = "Local server repair failed"
		successMessage = "Local server repaired"
	}

	server, err = a.serverService.SetInstallState(id, domain.InstallState{
		Status:      status,
		Installed:   false,
		Message:     startMessage,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseVersion: server.MinecraftVersion,
	})
	if err != nil {
		return domain.LocalServer{}, err
	}

	a.emitLocalServerProgress(domain.LocalServerProgress{
		ServerID: id,
		Stage:    "start",
		Message:  progressMessage,
	})
	if err := a.minecraftService.InstallVanillaServer(a.ctx, server, a.emitLocalServerProgress); err != nil {
		failed, stateErr := a.serverService.SetInstallState(id, domain.InstallState{
			Status:      "failed",
			Installed:   false,
			Message:     failMessage,
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
			LastError:   err.Error(),
			BaseVersion: server.MinecraftVersion,
		})
		a.emitLocalServerProgress(domain.LocalServerProgress{
			ServerID: id,
			Stage:    "failed",
			Message:  failMessage,
			Done:     true,
			Error:    err.Error(),
		})
		if stateErr != nil {
			return domain.LocalServer{}, stateErr
		}
		return failed, err
	}

	return a.serverService.SetInstallState(id, domain.InstallState{
		Status:      "installed",
		Installed:   true,
		Message:     successMessage,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseVersion: server.MinecraftVersion,
	})
}

func (a *App) ListMinecraftVersions() ([]domain.VersionOption, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	return a.catalogService.MinecraftVersions(a.ctx)
}

func (a *App) ListFabricLoaderVersions() ([]domain.VersionOption, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	return a.catalogService.FabricLoaderVersions(a.ctx)
}

func (a *App) ListQuiltLoaderVersions() ([]domain.VersionOption, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	return a.catalogService.QuiltLoaderVersions(a.ctx)
}

func (a *App) ListForgeLoaderVersions() ([]domain.VersionOption, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	return a.catalogService.ForgeLoaderVersions(a.ctx)
}

func (a *App) ListNeoForgeLoaderVersions() ([]domain.VersionOption, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	return a.catalogService.NeoForgeLoaderVersions(a.ctx)
}

func (a *App) GetCachedVersionCatalog() (domain.VersionCatalog, error) {
	if err := a.ensureReady(); err != nil {
		return domain.VersionCatalog{}, err
	}
	return a.catalogService.CachedVersionCatalog()
}

func (a *App) RefreshVersionCatalog() (domain.VersionCatalog, error) {
	if err := a.ensureReady(); err != nil {
		return domain.VersionCatalog{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.VersionCatalog{}, err
	}
	return a.catalogService.VersionCatalog(
		a.ctx,
		currentSettings.Network.MetadataTTLHours,
		currentSettings.Network.RetryCount,
	)
}

func (a *App) ValidateJava() (domain.JavaStatus, error) {
	if err := a.ensureReady(); err != nil {
		return domain.JavaStatus{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.JavaStatus{}, err
	}
	return a.javaService.Validate(a.ctx, currentSettings.JavaPath), nil
}

func (a *App) InstallJava(version int) (domain.JavaStatus, error) {
	if err := a.ensureReady(); err != nil {
		return domain.JavaStatus{}, err
	}

	javaPath, err := a.javaService.InstallTemurin(a.ctx, version, a.emitJavaProgress)
	if err != nil {
		return domain.JavaStatus{}, err
	}

	status := a.javaService.Validate(a.ctx, javaPath)
	a.emitJavaProgress(domain.JavaInstallProgress{
		Stage:    "complete",
		Message:  status.Message,
		Percent:  100,
		Done:     true,
		JavaPath: javaPath,
		Version:  fmt.Sprintf("%d", version),
	})
	return status, nil
}

func (a *App) GetProfileJavaRuntime(id string) (domain.ProfileJavaRuntime, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ProfileJavaRuntime{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ProfileJavaRuntime{}, err
	}
	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.ProfileJavaRuntime{}, err
	}
	return a.profileJavaRuntime(profile, currentSettings.JavaPath)
}

func (a *App) ListProfileMods(id string) (domain.ModList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModList{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ModList{}, err
	}
	return a.modsService.List(profile)
}

func (a *App) ImportProfileMod(id string) (domain.ModList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModList{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ModList{}, err
	}
	modsDir, err := a.modsService.EnsureDir(profile)
	if err != nil {
		return domain.ModList{}, err
	}

	sourcePath, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		DefaultDirectory: modsDir,
		Title:            "Import Minecraft mod",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Minecraft mods (*.jar)", Pattern: "*.jar"},
		},
	})
	if err != nil {
		return domain.ModList{}, err
	}
	if sourcePath == "" {
		return a.modsService.List(profile)
	}
	if _, err := a.modsService.Import(profile, sourcePath); err != nil {
		return domain.ModList{}, err
	}
	return a.modsService.List(profile)
}

func (a *App) ImportModrinthModpack() (domain.ModpackImportResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModpackImportResult{}, err
	}

	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.ModpackImportResult{}, err
	}

	sourcePath, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Import Modrinth modpack",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Modrinth modpacks (*.mrpack)", Pattern: "*.mrpack"},
		},
	})
	if err != nil {
		return domain.ModpackImportResult{}, err
	}
	if sourcePath == "" {
		return domain.ModpackImportResult{}, nil
	}

	index, err := a.modpackService.ReadIndex(sourcePath)
	if err != nil {
		return domain.ModpackImportResult{}, err
	}
	input, err := modpacks.ProfileInput(index, currentSettings.DefaultMemory)
	if err != nil {
		return domain.ModpackImportResult{}, err
	}

	profile, err := a.profileService.Create(input, currentSettings.DefaultMemory)
	if err != nil {
		return domain.ModpackImportResult{}, err
	}
	if _, err := a.profileService.Select(profile.ID); err != nil {
		return domain.ModpackImportResult{}, err
	}

	a.emitInstallProgress(domain.InstallProgress{
		ProfileID: profile.ID,
		Stage:     "modpack-start",
		Message:   "Importing Modrinth modpack " + input.Name,
	})

	profile, err = a.installProfile(profile.ID, false)
	if err != nil {
		return domain.ModpackImportResult{Profile: profile, Name: index.Name, VersionID: index.VersionID}, err
	}

	profile, err = a.profileService.SetInstallState(profile.ID, domain.InstallState{
		Status:      "installing",
		Installed:   false,
		Message:     "Importing modpack files",
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseVersion: profile.MinecraftVersion,
	})
	if err != nil {
		return domain.ModpackImportResult{}, err
	}

	result, err := a.modpackService.InstallClient(a.ctx, sourcePath, profile, a.emitInstallProgress)
	if err != nil {
		failed, stateErr := a.profileService.SetInstallState(profile.ID, domain.InstallState{
			Status:      "failed",
			Installed:   false,
			Message:     "Modpack import failed",
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
			LastError:   err.Error(),
			BaseVersion: profile.MinecraftVersion,
		})
		a.emitInstallProgress(domain.InstallProgress{
			ProfileID: profile.ID,
			Stage:     "failed",
			Message:   "Modpack import failed",
			Done:      true,
			Error:     err.Error(),
		})
		if stateErr != nil {
			return domain.ModpackImportResult{}, stateErr
		}
		return domain.ModpackImportResult{Profile: failed, Name: index.Name, VersionID: index.VersionID}, err
	}

	message := fmt.Sprintf("Imported modpack %s", index.Name)
	if strings.TrimSpace(index.VersionID) != "" {
		message += " " + index.VersionID
	}
	profile, err = a.profileService.SetInstallState(profile.ID, domain.InstallState{
		Status:      "installed",
		Installed:   true,
		Message:     message,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseVersion: profile.MinecraftVersion,
	})
	if err != nil {
		return domain.ModpackImportResult{}, err
	}

	return domain.ModpackImportResult{
		Profile:            profile,
		Name:               index.Name,
		VersionID:          index.VersionID,
		FilesInstalled:     result.FilesInstalled,
		FilesSkipped:       result.FilesSkipped,
		OverridesInstalled: result.OverridesInstalled,
	}, nil
}

func (a *App) ExportModrinthModpack(id string) (domain.ModpackExportResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModpackExportResult{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ModpackExportResult{}, err
	}

	options := wailsruntime.SaveDialogOptions{
		Title:           "Export Modrinth modpack",
		DefaultFilename: modpackDefaultFilename(profile.Name),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Modrinth modpacks (*.mrpack)", Pattern: "*.mrpack"},
		},
	}
	if info, err := os.Stat(profile.GameDir); err == nil && info.IsDir() {
		options.DefaultDirectory = profile.GameDir
	}

	targetPath, err := wailsruntime.SaveFileDialog(a.ctx, options)
	if err != nil {
		return domain.ModpackExportResult{}, err
	}
	if targetPath == "" {
		return domain.ModpackExportResult{ProfileID: profile.ID, Name: profile.Name}, nil
	}
	if !strings.EqualFold(filepath.Ext(targetPath), ".mrpack") {
		targetPath += ".mrpack"
	}

	exportFiles := a.modrinthExportFiles(profile)
	result, err := a.modpackService.Export(profile, targetPath, modpacks.ExportOptions{Files: exportFiles})
	if err != nil {
		return domain.ModpackExportResult{}, err
	}
	return domain.ModpackExportResult{
		ProfileID:         profile.ID,
		Name:              profile.Name,
		VersionID:         result.VersionID,
		Path:              targetPath,
		FilesExported:     result.FilesExported,
		OverridesExported: result.OverridesExported,
	}, nil
}

func (a *App) ExportProfileLogs(id string, launcherEvents string) (domain.LogExportResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LogExportResult{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.LogExportResult{}, err
	}

	options := wailsruntime.SaveDialogOptions{
		Title:           "Export profile logs",
		DefaultFilename: logsDefaultFilename(profile.Name),
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "ZIP archives (*.zip)", Pattern: "*.zip"},
		},
	}
	if info, err := os.Stat(profile.GameDir); err == nil && info.IsDir() {
		options.DefaultDirectory = profile.GameDir
	}

	targetPath, err := wailsruntime.SaveFileDialog(a.ctx, options)
	if err != nil {
		return domain.LogExportResult{}, err
	}
	if targetPath == "" {
		return domain.LogExportResult{ProfileID: profile.ID, Name: profile.Name}, nil
	}
	if !strings.EqualFold(filepath.Ext(targetPath), ".zip") {
		targetPath += ".zip"
	}

	return a.minecraftService.ExportGameLogs(profile, targetPath, launcherEvents)
}

func (a *App) SetProfileModEnabled(id string, fileName string, enabled bool) (domain.ModList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModList{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ModList{}, err
	}
	if _, err := a.modsService.SetEnabled(profile, fileName, enabled); err != nil {
		return domain.ModList{}, err
	}
	return a.modsService.List(profile)
}

func (a *App) DeleteProfileMod(id string, fileName string) (domain.ModList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModList{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.ModList{}, err
	}
	if err := a.modsService.Delete(profile, fileName); err != nil {
		return domain.ModList{}, err
	}
	return a.modsService.List(profile)
}

func (a *App) OpenProfileModsFolder(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return err
	}
	modsDir, err := a.modsService.EnsureDir(profile)
	if err != nil {
		return err
	}

	switch stdruntime.GOOS {
	case "darwin":
		return exec.Command("open", modsDir).Start()
	case "linux":
		return exec.Command("xdg-open", modsDir).Start()
	default:
		wailsruntime.BrowserOpenURL(a.ctx, "file://"+modsDir)
		return nil
	}
}

func (a *App) ListProfileGameLogs(id string) (domain.GameLogList, error) {
	if err := a.ensureReady(); err != nil {
		return domain.GameLogList{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.GameLogList{}, err
	}
	return a.minecraftService.ListGameLogs(profile)
}

func (a *App) ReadProfileGameLog(id string, fileName string) (domain.GameLogContent, error) {
	if err := a.ensureReady(); err != nil {
		return domain.GameLogContent{}, err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.GameLogContent{}, err
	}
	return a.minecraftService.ReadGameLog(profile, fileName)
}

func (a *App) OpenProfileLogsFolder(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	profile, err := a.profileService.Get(id)
	if err != nil {
		return err
	}
	logsDir := filepath.Join(profile.GameDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return err
	}

	switch stdruntime.GOOS {
	case "darwin":
		return exec.Command("open", logsDir).Start()
	case "linux":
		return exec.Command("xdg-open", logsDir).Start()
	default:
		wailsruntime.BrowserOpenURL(a.ctx, "file://"+logsDir)
		return nil
	}
}

func (a *App) OpenLocalServerFolder(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	server, err := a.serverService.Get(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(server.ServerDir, 0o755); err != nil {
		return err
	}

	switch stdruntime.GOOS {
	case "darwin":
		return exec.Command("open", server.ServerDir).Start()
	case "linux":
		return exec.Command("xdg-open", server.ServerDir).Start()
	default:
		wailsruntime.BrowserOpenURL(a.ctx, "file://"+server.ServerDir)
		return nil
	}
}

func (a *App) OpenLocalServerSettings(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	server, err := a.serverService.Get(id)
	if err != nil {
		return err
	}
	propertiesPath := filepath.Join(server.ServerDir, "server.properties")
	if _, err := os.Stat(propertiesPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("server.properties not found; install or repair the local server first")
		}
		return fmt.Errorf("open local server settings: %w", err)
	}

	switch stdruntime.GOOS {
	case "darwin":
		return exec.Command("open", propertiesPath).Start()
	case "linux":
		return exec.Command("xdg-open", propertiesPath).Start()
	default:
		wailsruntime.BrowserOpenURL(a.ctx, "file://"+propertiesPath)
		return nil
	}
}

func (a *App) OpenDetachedLogsWindow(snapshot string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	if err := a.setLogsSnapshot(snapshot); err != nil {
		return err
	}
	openURL, err := a.externalWindowURL("/logs")
	if err != nil {
		return err
	}
	wailsruntime.BrowserOpenURL(a.ctx, openURL)
	return nil
}

func (a *App) SyncDetachedLogsWindow(snapshot string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	return a.setLogsSnapshot(snapshot)
}

func (a *App) OpenLocalServerTerminal(id string) error {
	if err := a.ensureReady(); err != nil {
		return err
	}
	if _, err := a.serverService.Get(id); err != nil {
		return err
	}
	openURL, err := a.externalWindowURL("/server-terminal/" + url.PathEscape(id))
	if err != nil {
		return err
	}
	wailsruntime.BrowserOpenURL(a.ctx, openURL)
	return nil
}

func (a *App) SearchModrinthMods(profileID string, query string) (domain.ModrinthSearchResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthSearchResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthSearchResult{}, err
	}
	return a.catalogService.SearchModrinthMods(a.ctx, profile, query, 20)
}

func (a *App) GetModrinthProject(projectID string) (domain.ModrinthProject, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthProject{}, err
	}
	return a.catalogService.ModrinthProject(a.ctx, projectID)
}

func (a *App) ListInstalledModrinthProjects(profileID string) ([]string, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return nil, err
	}
	return a.modsService.ModrinthInstalledProjectIDs(profile)
}

func (a *App) ListModrinthUpdates(profileID string) ([]domain.ModrinthUpdatePlan, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return nil, err
	}
	installedProjects, err := a.modsService.ModrinthInstalledProjects(profile)
	if err != nil {
		return nil, err
	}
	plans := make([]domain.ModrinthUpdatePlan, 0, len(installedProjects))
	trackedFileKeys := map[string]bool{}
	for _, installed := range installedProjects {
		trackedFileKeys[modrinthFileNameKey(installed.FileName)] = true
		for _, file := range installed.Files {
			trackedFileKeys[modrinthFileNameKey(file.FileName)] = true
		}
		plan, err := a.modrinthUpdatePlan(profile, installed)
		if err != nil {
			plans = append(plans, domain.ModrinthUpdatePlan{
				ProfileID:            profile.ID,
				ProjectID:            installed.ProjectID,
				ProjectTitle:         installed.ProjectTitle,
				Tracked:              true,
				CurrentVersionID:     installed.VersionID,
				CurrentVersionName:   installed.VersionName,
				CurrentVersionNumber: installed.VersionNumber,
				CurrentFileName:      installed.FileName,
				CheckError:           err.Error(),
			})
			continue
		}
		plans = append(plans, plan)
	}

	modList, err := a.modsService.List(profile)
	if err != nil {
		return nil, err
	}
	for _, modFile := range modList.Mods {
		if trackedFileKeys[modrinthFileNameKey(modFile.FileName)] || strings.TrimSpace(modFile.SHA1) == "" {
			continue
		}
		plan, err := a.modrinthUpdatePlanForFile(profile, modFile)
		if err != nil {
			if modrinthMetadataNotFound(err) {
				continue
			}
			plans = append(plans, domain.ModrinthUpdatePlan{
				ProfileID:       profile.ID,
				ProjectTitle:    modFile.DisplayName,
				CurrentFileName: modFile.FileName,
				CheckError:      err.Error(),
			})
			continue
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func (a *App) ListModrinthProjectVersions(profileID string, projectID string) ([]domain.ModrinthVersion, error) {
	if err := a.ensureReady(); err != nil {
		return nil, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return nil, err
	}
	return a.catalogService.ModrinthProjectVersions(a.ctx, profile, projectID)
}

func (a *App) PlanModrinthInstall(profileID string, projectID string) (domain.ModrinthInstallPlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthInstallPlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthInstallPlan{}, err
	}

	version, err := a.catalogService.LatestModrinthVersion(a.ctx, profile, projectID)
	if err != nil {
		return domain.ModrinthInstallPlan{}, err
	}

	return a.modrinthInstallPlanForVersion(profile, version)
}

func (a *App) PlanModrinthInstallVersion(profileID string, projectID string, versionID string) (domain.ModrinthInstallPlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthInstallPlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthInstallPlan{}, err
	}
	version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, projectID, versionID)
	if err != nil {
		return domain.ModrinthInstallPlan{}, err
	}
	return a.modrinthInstallPlanForVersion(profile, version)
}

func (a *App) PlanModrinthUpdate(profileID string, projectID string) (domain.ModrinthUpdatePlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	installed, found, err := a.modsService.ModrinthInstalledProject(profile, projectID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	if !found {
		return domain.ModrinthUpdatePlan{}, fmt.Errorf("Modrinth project %s is not installed in this profile", projectID)
	}
	return a.modrinthUpdatePlan(profile, installed)
}

func (a *App) PlanModrinthUpdateVersion(profileID string, projectID string, versionID string) (domain.ModrinthUpdatePlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	installed, found, err := a.modsService.ModrinthInstalledProject(profile, projectID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	if !found {
		return domain.ModrinthUpdatePlan{}, fmt.Errorf("Modrinth project %s is not installed in this profile", projectID)
	}
	version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, projectID, versionID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	return a.modrinthUpdatePlanForVersion(profile, installed, version)
}

func (a *App) PlanModrinthUpdateFile(profileID string, fileName string) (domain.ModrinthUpdatePlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	modFile, found, err := a.modsService.Existing(profile, fileName)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	if !found {
		return domain.ModrinthUpdatePlan{}, fmt.Errorf("mod file %s is not installed in this profile", fileName)
	}
	return a.modrinthUpdatePlanForFile(profile, modFile)
}

func (a *App) PlanModrinthDelete(profileID string, projectID string) (domain.ModrinthDeletePlan, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthDeletePlan{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthDeletePlan{}, err
	}
	if plan, found, err := a.modsService.PlanModrinthDelete(profile, projectID); err != nil || found {
		return plan, err
	}

	installPlan, err := a.PlanModrinthInstall(profileID, projectID)
	if err != nil {
		return domain.ModrinthDeletePlan{}, err
	}
	return a.untrackedModrinthDeletePlan(profile, installPlan)
}

func (a *App) DeleteModrinthMod(profileID string, projectID string) (domain.ModrinthDeleteResult, error) {
	return a.DeleteModrinthModFiles(profileID, projectID, nil)
}

func (a *App) DeleteModrinthModFiles(profileID string, projectID string, fileNames []string) (domain.ModrinthDeleteResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthDeleteResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthDeleteResult{}, err
	}
	if result, found, err := a.modsService.DeleteModrinthInstallFiles(profile, projectID, fileNames); err != nil || found {
		return result, err
	}

	plan, err := a.PlanModrinthDelete(profileID, projectID)
	if err != nil {
		return domain.ModrinthDeleteResult{}, err
	}
	return a.modsService.DeleteSelectedModrinthFiles(profile, plan, fileNames)
}

func (a *App) InstallModrinthMod(profileID string, projectID string) (domain.ModrinthInstallResult, error) {
	return a.InstallModrinthModFiles(profileID, projectID, nil)
}

func (a *App) UpdateModrinthModFiles(profileID string, projectID string, selectedDependencyIDs []string) (domain.ModrinthUpdateResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	installed, found, err := a.modsService.ModrinthInstalledProject(profile, projectID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	if !found {
		return domain.ModrinthUpdateResult{}, fmt.Errorf("Modrinth project %s is not installed in this profile", projectID)
	}

	version, err := a.catalogService.LatestModrinthVersion(a.ctx, profile, projectID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	return a.updateModrinthToVersion(profile, installed, version, selectedDependencyIDs)
}

func (a *App) UpdateModrinthModVersionFiles(profileID string, projectID string, versionID string, selectedDependencyIDs []string) (domain.ModrinthUpdateResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	installed, found, err := a.modsService.ModrinthInstalledProject(profile, projectID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	if !found {
		return domain.ModrinthUpdateResult{}, fmt.Errorf("Modrinth project %s is not installed in this profile", projectID)
	}
	version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, projectID, versionID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	return a.updateModrinthToVersion(profile, installed, version, selectedDependencyIDs)
}

func (a *App) updateModrinthToVersion(profile domain.Profile, installed domain.ModrinthInstalledProject, version domain.ModrinthVersion, selectedDependencyIDs []string) (domain.ModrinthUpdateResult, error) {
	plan, err := a.modrinthUpdatePlanForVersion(profile, installed, version)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	if !plan.UpdateAvailable {
		modList, err := a.modsService.List(profile)
		if err != nil {
			return domain.ModrinthUpdateResult{}, err
		}
		return domain.ModrinthUpdateResult{
			ProfileID:    profile.ID,
			ProjectID:    installed.ProjectID,
			ProjectTitle: installed.ProjectTitle,
			Updated:      false,
			OldFileName:  installed.FileName,
			NewFileName:  installed.FileName,
			ModList:      modList,
		}, nil
	}

	state := &modrinthInstallState{
		seenProjects:         make(map[string]bool),
		seenVersions:         make(map[string]bool),
		limitDependencies:    selectedDependencyIDs != nil,
		selectedDependencies: selectedModrinthDependencyMap(selectedDependencyIDs),
	}
	if err := a.installModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}

	deleted, skipped, err := a.modsService.PruneReplacedModrinthFiles(profile, installed.ProjectID, state.installedFiles)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	modList, err := a.modsService.List(profile)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}

	title := a.modrinthProjectTitle(version)
	fileName := version.File.FileName
	for _, installedFile := range state.installedFiles {
		if installedFile.VersionID == version.ID {
			if installedFile.DisplayName != "" {
				title = installedFile.DisplayName
			}
			fileName = installedFile.FileName
			break
		}
	}
	installResult := domain.ModrinthInstallResult{
		ProfileID:           profile.ID,
		ProjectID:           version.ProjectID,
		ProjectTitle:        title,
		VersionID:           version.ID,
		VersionName:         version.Name,
		VersionNumber:       version.VersionNumber,
		FileName:            fileName,
		ModList:             modList,
		Dependencies:        version.Dependencies,
		InstalledFiles:      state.installedFiles,
		SkippedDependencies: state.skippedDependencies,
	}
	if err := a.modsService.RecordModrinthInstall(profile, installResult); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	return domain.ModrinthUpdateResult{
		ProfileID:           profile.ID,
		ProjectID:           version.ProjectID,
		ProjectTitle:        title,
		Updated:             true,
		OldFileName:         installed.FileName,
		NewFileName:         fileName,
		ModList:             modList,
		InstalledFiles:      state.installedFiles,
		DeletedFiles:        deleted,
		SkippedFiles:        skipped,
		SkippedDependencies: state.skippedDependencies,
	}, nil
}

func (a *App) UpdateModrinthModFile(profileID string, fileName string, selectedDependencyIDs []string) (domain.ModrinthUpdateResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	modFile, found, err := a.modsService.Existing(profile, fileName)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	if !found {
		return domain.ModrinthUpdateResult{}, fmt.Errorf("mod file %s is not installed in this profile", fileName)
	}
	plan, err := a.modrinthUpdatePlanForFile(profile, modFile)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	if !plan.UpdateAvailable {
		modList, err := a.modsService.List(profile)
		if err != nil {
			return domain.ModrinthUpdateResult{}, err
		}
		return domain.ModrinthUpdateResult{
			ProfileID:    profile.ID,
			ProjectID:    plan.ProjectID,
			ProjectTitle: plan.ProjectTitle,
			Updated:      false,
			OldFileName:  modFile.FileName,
			NewFileName:  modFile.FileName,
			ModList:      modList,
		}, nil
	}

	version, err := a.catalogService.LatestModrinthVersionFromHash(a.ctx, profile, modFile.SHA1)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	state := &modrinthInstallState{
		seenProjects:         make(map[string]bool),
		seenVersions:         make(map[string]bool),
		limitDependencies:    selectedDependencyIDs != nil,
		selectedDependencies: selectedModrinthDependencyMap(selectedDependencyIDs),
	}
	if err := a.installModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}

	deleted := make([]domain.ModrinthDeleteFile, 0, 1)
	skipped := make([]domain.ModrinthDeleteFile, 0)
	if modrinthFileNameKey(modFile.FileName) != modrinthFileNameKey(version.File.FileName) {
		deleteFile := domain.ModrinthDeleteFile{
			ProjectID:      version.ProjectID,
			ProjectTitle:   plan.ProjectTitle,
			VersionID:      plan.CurrentVersionID,
			VersionName:    plan.CurrentVersionName,
			VersionNumber:  plan.CurrentVersionNumber,
			FileName:       modFile.FileName,
			DisplayName:    modFile.DisplayName,
			DependencyType: "",
		}
		if existing, ok, err := a.modsService.Existing(profile, modFile.FileName); err != nil {
			return domain.ModrinthUpdateResult{}, err
		} else if !ok {
			deleteFile.Reason = "file is already missing"
			skipped = append(skipped, deleteFile)
		} else if err := a.modsService.Delete(profile, existing.FileName); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				deleteFile.Reason = "file is already missing"
				skipped = append(skipped, deleteFile)
			} else {
				return domain.ModrinthUpdateResult{}, err
			}
		} else {
			deleteFile.FileName = existing.FileName
			if existing.DisplayName != "" {
				deleteFile.DisplayName = existing.DisplayName
			}
			deleted = append(deleted, deleteFile)
		}
	}

	modList, err := a.modsService.List(profile)
	if err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	title := a.modrinthProjectTitle(version)
	newFileName := version.File.FileName
	for _, installedFile := range state.installedFiles {
		if installedFile.VersionID == version.ID {
			if installedFile.DisplayName != "" {
				title = installedFile.DisplayName
			}
			newFileName = installedFile.FileName
			break
		}
	}
	installResult := domain.ModrinthInstallResult{
		ProfileID:           profile.ID,
		ProjectID:           version.ProjectID,
		ProjectTitle:        title,
		VersionID:           version.ID,
		VersionName:         version.Name,
		VersionNumber:       version.VersionNumber,
		FileName:            newFileName,
		ModList:             modList,
		Dependencies:        version.Dependencies,
		InstalledFiles:      state.installedFiles,
		SkippedDependencies: state.skippedDependencies,
	}
	if err := a.modsService.RecordModrinthInstall(profile, installResult); err != nil {
		return domain.ModrinthUpdateResult{}, err
	}
	return domain.ModrinthUpdateResult{
		ProfileID:           profile.ID,
		ProjectID:           version.ProjectID,
		ProjectTitle:        title,
		Updated:             true,
		OldFileName:         modFile.FileName,
		NewFileName:         newFileName,
		ModList:             modList,
		InstalledFiles:      state.installedFiles,
		DeletedFiles:        deleted,
		SkippedFiles:        skipped,
		SkippedDependencies: state.skippedDependencies,
	}, nil
}

func (a *App) InstallModrinthModFiles(profileID string, projectID string, selectedDependencyIDs []string) (domain.ModrinthInstallResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthInstallResult{}, err
	}

	version, err := a.catalogService.LatestModrinthVersion(a.ctx, profile, projectID)
	if err != nil {
		return domain.ModrinthInstallResult{}, err
	}

	return a.installModrinthVersion(profile, version, selectedDependencyIDs)
}

func (a *App) InstallModrinthModVersionFiles(profileID string, projectID string, versionID string, selectedDependencyIDs []string) (domain.ModrinthInstallResult, error) {
	if err := a.ensureReady(); err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	profile, err := a.profileService.Get(profileID)
	if err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, projectID, versionID)
	if err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	return a.installModrinthVersion(profile, version, selectedDependencyIDs)
}

func (a *App) installModrinthVersion(profile domain.Profile, version domain.ModrinthVersion, selectedDependencyIDs []string) (domain.ModrinthInstallResult, error) {
	state := &modrinthInstallState{
		seenProjects:         make(map[string]bool),
		seenVersions:         make(map[string]bool),
		limitDependencies:    selectedDependencyIDs != nil,
		selectedDependencies: selectedModrinthDependencyMap(selectedDependencyIDs),
	}
	if err := a.installModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	modList, err := a.modsService.List(profile)
	if err != nil {
		return domain.ModrinthInstallResult{}, err
	}

	title := version.ProjectID
	fileName := version.File.FileName
	for _, installedFile := range state.installedFiles {
		if installedFile.VersionID == version.ID {
			title = installedFile.DisplayName
			fileName = installedFile.FileName
			break
		}
	}
	if title == "" {
		title = version.ProjectID
	}
	result := domain.ModrinthInstallResult{
		ProfileID:           profile.ID,
		ProjectID:           version.ProjectID,
		ProjectTitle:        title,
		VersionID:           version.ID,
		VersionName:         version.Name,
		VersionNumber:       version.VersionNumber,
		FileName:            fileName,
		ModList:             modList,
		Dependencies:        version.Dependencies,
		InstalledFiles:      state.installedFiles,
		SkippedDependencies: state.skippedDependencies,
	}
	if err := a.modsService.RecordModrinthInstall(profile, result); err != nil {
		return domain.ModrinthInstallResult{}, err
	}
	return result, nil
}

func (a *App) modrinthInstallPlanForVersion(profile domain.Profile, version domain.ModrinthVersion) (domain.ModrinthInstallPlan, error) {
	state := &modrinthInstallPlanState{
		seenProjects: make(map[string]bool),
		seenVersions: make(map[string]bool),
	}
	if err := a.planModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthInstallPlan{}, err
	}

	return domain.ModrinthInstallPlan{
		ProfileID:            profile.ID,
		ProjectID:            version.ProjectID,
		ProjectTitle:         a.modrinthProjectTitle(version),
		VersionID:            version.ID,
		VersionName:          version.Name,
		VersionNumber:        version.VersionNumber,
		FileName:             version.File.FileName,
		RequiredDependencies: state.requiredDependencies,
		SkippedDependencies:  state.skippedDependencies,
	}, nil
}

func (a *App) modrinthUpdatePlan(profile domain.Profile, installed domain.ModrinthInstalledProject) (domain.ModrinthUpdatePlan, error) {
	version, err := a.catalogService.LatestModrinthVersion(a.ctx, profile, installed.ProjectID)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	return a.modrinthUpdatePlanForVersion(profile, installed, version)
}

func (a *App) modrinthUpdatePlanForVersion(profile domain.Profile, installed domain.ModrinthInstalledProject, version domain.ModrinthVersion) (domain.ModrinthUpdatePlan, error) {
	state := &modrinthInstallPlanState{
		seenProjects: make(map[string]bool),
		seenVersions: make(map[string]bool),
	}
	if err := a.planModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	title := installed.ProjectTitle
	if strings.TrimSpace(title) == "" {
		title = a.modrinthProjectTitle(version)
	}
	return domain.ModrinthUpdatePlan{
		ProfileID:            profile.ID,
		ProjectID:            installed.ProjectID,
		ProjectTitle:         title,
		Tracked:              true,
		CurrentVersionID:     installed.VersionID,
		CurrentVersionName:   installed.VersionName,
		CurrentVersionNumber: installed.VersionNumber,
		CurrentFileName:      installed.FileName,
		LatestVersionID:      version.ID,
		LatestVersionName:    version.Name,
		LatestVersionNumber:  version.VersionNumber,
		LatestFileName:       version.File.FileName,
		UpdateAvailable:      modrinthUpdateAvailable(installed, version),
		RequiredDependencies: state.requiredDependencies,
		SkippedDependencies:  state.skippedDependencies,
	}, nil
}

func (a *App) modrinthUpdatePlanForFile(profile domain.Profile, modFile domain.ModFile) (domain.ModrinthUpdatePlan, error) {
	version, err := a.catalogService.LatestModrinthVersionFromHash(a.ctx, profile, modFile.SHA1)
	if err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}
	state := &modrinthInstallPlanState{
		seenProjects: make(map[string]bool),
		seenVersions: make(map[string]bool),
	}
	if err := a.planModrinthVersionTree(profile, version, "", state, 0); err != nil {
		return domain.ModrinthUpdatePlan{}, err
	}

	currentVersionID := ""
	currentVersionName := modFile.DisplayName
	currentVersionNumber := ""
	updateAvailable := modrinthHashUpdateAvailable(modFile, version)
	currentVersion, err := a.catalogService.ModrinthVersionFromHash(a.ctx, modFile.SHA1)
	if err == nil {
		currentVersionID = currentVersion.ID
		currentVersionName = currentVersion.Name
		currentVersionNumber = currentVersion.VersionNumber
		if currentVersion.ID != "" && version.ID != "" {
			updateAvailable = currentVersion.ID != version.ID
		}
	} else if !updateAvailable {
		currentVersionID = version.ID
		currentVersionName = version.Name
		currentVersionNumber = version.VersionNumber
	}

	return domain.ModrinthUpdatePlan{
		ProfileID:            profile.ID,
		ProjectID:            version.ProjectID,
		ProjectTitle:         a.modrinthProjectTitle(version),
		Tracked:              false,
		CurrentVersionID:     currentVersionID,
		CurrentVersionName:   currentVersionName,
		CurrentVersionNumber: currentVersionNumber,
		CurrentFileName:      modFile.FileName,
		LatestVersionID:      version.ID,
		LatestVersionName:    version.Name,
		LatestVersionNumber:  version.VersionNumber,
		LatestFileName:       version.File.FileName,
		UpdateAvailable:      updateAvailable,
		RequiredDependencies: state.requiredDependencies,
		SkippedDependencies:  state.skippedDependencies,
	}, nil
}

func (a *App) planModrinthVersionTree(profile domain.Profile, version domain.ModrinthVersion, dependencyType string, state *modrinthInstallPlanState, depth int) error {
	if depth > maxModrinthDependencyDepth {
		return fmt.Errorf("Modrinth dependency tree is too deep")
	}
	if version.ID == "" {
		return fmt.Errorf("Modrinth version id is empty")
	}
	if state.seenVersions[version.ID] {
		return nil
	}
	if version.ProjectID != "" && state.seenProjects[version.ProjectID] {
		return nil
	}

	state.seenVersions[version.ID] = true
	if version.ProjectID != "" {
		state.seenProjects[version.ProjectID] = true
	}
	if dependencyType == "required" {
		state.requiredDependencies = append(state.requiredDependencies, a.modrinthRequiredDependency(profile, version))
	}

	for _, dependency := range version.Dependencies {
		kind := strings.ToLower(strings.TrimSpace(dependency.DependencyType))
		if kind != "required" {
			state.skippedDependencies = append(state.skippedDependencies, domain.ModrinthSkippedDependency{
				Dependency: dependency,
				Reason:     modrinthSkippedDependencyReason(kind),
			})
			continue
		}

		dependencyVersion, err := a.resolveRequiredModrinthDependency(profile, dependency)
		if err != nil {
			return err
		}
		if err := a.planModrinthVersionTree(profile, dependencyVersion, kind, state, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) installModrinthVersionTree(profile domain.Profile, version domain.ModrinthVersion, dependencyType string, state *modrinthInstallState, depth int) error {
	if depth > maxModrinthDependencyDepth {
		return fmt.Errorf("Modrinth dependency tree is too deep")
	}
	if version.ID == "" {
		return fmt.Errorf("Modrinth version id is empty")
	}
	if state.seenVersions[version.ID] {
		return nil
	}
	if version.ProjectID != "" && state.seenProjects[version.ProjectID] {
		return nil
	}

	state.seenVersions[version.ID] = true
	if version.ProjectID != "" {
		state.seenProjects[version.ProjectID] = true
	}

	for _, dependency := range version.Dependencies {
		kind := strings.ToLower(strings.TrimSpace(dependency.DependencyType))
		if kind != "required" {
			state.skippedDependencies = append(state.skippedDependencies, domain.ModrinthSkippedDependency{
				Dependency: dependency,
				Reason:     modrinthSkippedDependencyReason(kind),
			})
			continue
		}

		dependencyVersion, err := a.resolveRequiredModrinthDependency(profile, dependency)
		if err != nil {
			return err
		}
		if state.limitDependencies && !modrinthDependencySelected(state.selectedDependencies, dependencyVersion) {
			state.skippedDependencies = append(state.skippedDependencies, domain.ModrinthSkippedDependency{
				Dependency: domain.ModrinthDependency{
					VersionID:      dependencyVersion.ID,
					ProjectID:      dependencyVersion.ProjectID,
					FileName:       dependencyVersion.File.FileName,
					DependencyType: kind,
				},
				Reason: "not selected",
			})
			continue
		}
		if err := a.installModrinthVersionTree(profile, dependencyVersion, kind, state, depth+1); err != nil {
			return err
		}
	}

	installedFile, err := a.installModrinthJar(profile, version, dependencyType)
	if err != nil {
		return err
	}
	state.installedFiles = append(state.installedFiles, installedFile)
	return nil
}

func (a *App) resolveRequiredModrinthDependency(profile domain.Profile, dependency domain.ModrinthDependency) (domain.ModrinthVersion, error) {
	if strings.TrimSpace(dependency.VersionID) != "" && strings.TrimSpace(dependency.ProjectID) != "" {
		version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, dependency.ProjectID, dependency.VersionID)
		if err == nil {
			return version, nil
		}
	}
	if strings.TrimSpace(dependency.ProjectID) == "" {
		if strings.TrimSpace(dependency.VersionID) != "" {
			return domain.ModrinthVersion{}, fmt.Errorf("required dependency %s does not include a Modrinth project id for version lookup", modrinthDependencyLabel(dependency))
		}
		return domain.ModrinthVersion{}, fmt.Errorf("required dependency %s does not include a Modrinth project or version id", modrinthDependencyLabel(dependency))
	}

	version, err := a.catalogService.LatestModrinthVersion(a.ctx, profile, dependency.ProjectID)
	if err != nil {
		return domain.ModrinthVersion{}, fmt.Errorf("required dependency %s could not be resolved: %w", modrinthDependencyLabel(dependency), err)
	}
	return version, nil
}

func (a *App) installModrinthJar(profile domain.Profile, version domain.ModrinthVersion, dependencyType string) (domain.ModrinthInstalledFile, error) {
	if strings.TrimSpace(version.File.FileName) == "" || strings.TrimSpace(version.File.URL) == "" {
		return domain.ModrinthInstalledFile{}, fmt.Errorf("Modrinth version %s does not include a downloadable jar", version.ID)
	}

	if existing, ok, err := a.modsService.Existing(profile, version.File.FileName); err != nil {
		return domain.ModrinthInstalledFile{}, err
	} else if ok {
		if version.File.SHA1 == "" || strings.EqualFold(existing.SHA1, version.File.SHA1) {
			return modrinthInstalledFile(version, existing, dependencyType, true), nil
		}

		body, err := a.catalogService.OpenDownload(a.ctx, version.File.URL)
		if err != nil {
			return domain.ModrinthInstalledFile{}, err
		}
		defer body.Close()

		modFile, err := a.modsService.Replace(profile, existing.FileName, body)
		if err != nil {
			return domain.ModrinthInstalledFile{}, err
		}
		return modrinthInstalledFile(version, modFile, dependencyType, false), nil
	}

	body, err := a.catalogService.OpenDownload(a.ctx, version.File.URL)
	if err != nil {
		return domain.ModrinthInstalledFile{}, err
	}
	defer body.Close()

	modFile, err := a.modsService.Install(profile, version.File.FileName, body)
	if err != nil {
		return domain.ModrinthInstalledFile{}, err
	}
	return modrinthInstalledFile(version, modFile, dependencyType, false), nil
}

func (a *App) untrackedModrinthDeletePlan(profile domain.Profile, installPlan domain.ModrinthInstallPlan) (domain.ModrinthDeletePlan, error) {
	files := make([]domain.ModrinthDeleteFile, 0, 1+len(installPlan.RequiredDependencies))
	skipped := make([]domain.ModrinthDeleteFile, 0)

	mainFile, ok, err := a.modsService.Existing(profile, installPlan.FileName)
	if err != nil {
		return domain.ModrinthDeletePlan{}, err
	}
	mainDelete := domain.ModrinthDeleteFile{
		ProjectID:     installPlan.ProjectID,
		ProjectTitle:  installPlan.ProjectTitle,
		VersionID:     installPlan.VersionID,
		VersionName:   installPlan.VersionName,
		VersionNumber: installPlan.VersionNumber,
		FileName:      installPlan.FileName,
		DisplayName:   installPlan.ProjectTitle,
	}
	if ok {
		mainDelete.FileName = mainFile.FileName
		if mainFile.DisplayName != "" {
			mainDelete.DisplayName = mainFile.DisplayName
		}
		files = append(files, mainDelete)
	} else {
		mainDelete.Reason = "file is already missing"
		skipped = append(skipped, mainDelete)
	}

	for _, dependency := range installPlan.RequiredDependencies {
		deleteFile := domain.ModrinthDeleteFile{
			ProjectID:      dependency.ProjectID,
			ProjectTitle:   dependency.ProjectTitle,
			VersionID:      dependency.VersionID,
			VersionName:    dependency.VersionName,
			VersionNumber:  dependency.VersionNumber,
			FileName:       dependency.FileName,
			DisplayName:    dependency.DisplayName,
			DependencyType: "required",
		}
		existing, ok, err := a.modsService.Existing(profile, dependency.FileName)
		if err != nil {
			return domain.ModrinthDeletePlan{}, err
		}
		if !ok {
			deleteFile.Reason = "file is already missing"
			skipped = append(skipped, deleteFile)
			continue
		}
		deleteFile.FileName = existing.FileName
		if existing.DisplayName != "" {
			deleteFile.DisplayName = existing.DisplayName
		}
		files = append(files, deleteFile)
	}

	return domain.ModrinthDeletePlan{
		ProfileID:    profile.ID,
		ProjectID:    installPlan.ProjectID,
		ProjectTitle: installPlan.ProjectTitle,
		Files:        files,
		SkippedFiles: skipped,
		Tracked:      false,
	}, nil
}

func (a *App) modrinthRequiredDependency(profile domain.Profile, version domain.ModrinthVersion) domain.ModrinthRequiredDependency {
	title := a.modrinthProjectTitle(version)
	displayName := title
	alreadyPresent := false
	if existing, ok, err := a.modsService.Existing(profile, version.File.FileName); err == nil && ok {
		alreadyPresent = true
		if existing.DisplayName != "" {
			displayName = existing.DisplayName
		}
	}
	if displayName == "" {
		displayName = version.File.FileName
	}

	return domain.ModrinthRequiredDependency{
		ProjectID:      version.ProjectID,
		ProjectTitle:   title,
		VersionID:      version.ID,
		VersionName:    version.Name,
		VersionNumber:  version.VersionNumber,
		FileName:       version.File.FileName,
		DisplayName:    displayName,
		AlreadyPresent: alreadyPresent,
	}
}

func (a *App) modrinthProjectTitle(version domain.ModrinthVersion) string {
	title := strings.TrimSpace(version.ProjectID)
	if project, err := a.catalogService.ModrinthProject(a.ctx, version.ProjectID); err == nil && strings.TrimSpace(project.Title) != "" {
		title = strings.TrimSpace(project.Title)
	}
	if title == "" {
		title = strings.TrimSpace(version.File.FileName)
	}
	if title == "" {
		title = strings.TrimSpace(version.Name)
	}
	return title
}

func modrinthInstalledFile(version domain.ModrinthVersion, modFile domain.ModFile, dependencyType string, alreadyPresent bool) domain.ModrinthInstalledFile {
	return domain.ModrinthInstalledFile{
		ProjectID:      version.ProjectID,
		VersionID:      version.ID,
		VersionName:    version.Name,
		VersionNumber:  version.VersionNumber,
		FileName:       modFile.FileName,
		DisplayName:    modFile.DisplayName,
		DependencyType: dependencyType,
		AlreadyPresent: alreadyPresent,
	}
}

func modrinthUpdateAvailable(installed domain.ModrinthInstalledProject, version domain.ModrinthVersion) bool {
	if strings.TrimSpace(installed.VersionID) != "" && strings.TrimSpace(version.ID) != "" {
		return installed.VersionID != version.ID
	}
	return modrinthFileNameKey(installed.FileName) != modrinthFileNameKey(version.File.FileName)
}

func modrinthHashUpdateAvailable(modFile domain.ModFile, version domain.ModrinthVersion) bool {
	if strings.TrimSpace(modFile.SHA1) != "" && strings.TrimSpace(version.File.SHA1) != "" {
		return !strings.EqualFold(modFile.SHA1, version.File.SHA1)
	}
	return modrinthFileNameKey(modFile.FileName) != modrinthFileNameKey(version.File.FileName)
}

func modrinthMetadataNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "404")
}

func modrinthFileNameKey(fileName string) string {
	key := strings.ToLower(strings.TrimSpace(fileName))
	return strings.TrimSuffix(key, ".disabled")
}

func selectedModrinthDependencyMap(values []string) map[string]bool {
	selected := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			selected[value] = true
		}
	}
	return selected
}

func modrinthDependencySelected(selected map[string]bool, version domain.ModrinthVersion) bool {
	return selected[version.ID] || selected[version.ProjectID] || selected[version.File.FileName]
}

func modrinthDependencyLabel(dependency domain.ModrinthDependency) string {
	if dependency.ProjectID != "" {
		return dependency.ProjectID
	}
	if dependency.VersionID != "" {
		return dependency.VersionID
	}
	if dependency.FileName != "" {
		return dependency.FileName
	}
	return "unknown"
}

func modrinthSkippedDependencyReason(dependencyType string) string {
	switch dependencyType {
	case "optional":
		return "optional dependency"
	case "incompatible":
		return "incompatible dependency marker"
	case "embedded":
		return "embedded in the downloaded file"
	case "":
		return "dependency type is missing"
	default:
		return "unsupported dependency type"
	}
}

func (a *App) InstallProfile(id string) (domain.Profile, error) {
	return a.installProfile(id, false)
}

func (a *App) RepairProfile(id string) (domain.Profile, error) {
	return a.installProfile(id, true)
}

func (a *App) installProfile(id string, repair bool) (domain.Profile, error) {
	if err := a.ensureReady(); err != nil {
		return domain.Profile{}, err
	}

	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.Profile{}, err
	}

	status := "installing"
	startMessage := "Installing vanilla base files"
	progressMessage := "Starting install"
	failMessage := "Install failed"
	successMessage := "Installed"
	if repair {
		status = "repairing"
		startMessage = "Checking and repairing base files"
		progressMessage = "Starting repair"
		failMessage = "Repair failed"
		successMessage = "Repaired"
	}

	now := time.Now().UTC().Format(time.RFC3339)
	profile, err = a.profileService.SetInstallState(id, domain.InstallState{
		Status:      status,
		Installed:   false,
		Message:     startMessage,
		UpdatedAt:   now,
		BaseVersion: profile.MinecraftVersion,
	})
	if err != nil {
		return domain.Profile{}, err
	}

	a.emitInstallProgress(domain.InstallProgress{
		ProfileID: id,
		Stage:     "start",
		Message:   progressMessage,
	})

	err = a.minecraftService.InstallVanillaBase(a.ctx, profile, a.emitInstallProgress)
	if err != nil {
		failed, stateErr := a.profileService.SetInstallState(id, domain.InstallState{
			Status:      "failed",
			Installed:   false,
			Message:     failMessage,
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
			LastError:   err.Error(),
			BaseVersion: profile.MinecraftVersion,
		})
		a.emitInstallProgress(domain.InstallProgress{
			ProfileID: id,
			Stage:     "failed",
			Message:   failMessage,
			Done:      true,
			Error:     err.Error(),
		})
		if stateErr != nil {
			return domain.Profile{}, stateErr
		}
		return failed, err
	}

	finalStatus := "installed"
	installed := true
	message := successMessage
	switch profile.Loader.Type {
	case domain.LoaderFabric:
		loaderVersion, err := a.minecraftService.InstallFabricLoader(a.ctx, profile, a.emitInstallProgress)
		if err != nil {
			message := "Fabric install failed"
			if repair {
				message = "Fabric repair failed"
			}
			failed, stateErr := a.profileService.SetInstallState(id, domain.InstallState{
				Status:      "failed",
				Installed:   false,
				Message:     message,
				UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				LastError:   err.Error(),
				BaseVersion: profile.MinecraftVersion,
			})
			a.emitInstallProgress(domain.InstallProgress{
				ProfileID: id,
				Stage:     "failed",
				Message:   message,
				Done:      true,
				Error:     err.Error(),
			})
			if stateErr != nil {
				return domain.Profile{}, stateErr
			}
			return failed, err
		}
		if repair {
			message = "Repaired Fabric loader " + loaderVersion
		} else {
			message = "Installed Fabric loader " + loaderVersion
		}
	case domain.LoaderQuilt:
		loaderVersion, err := a.minecraftService.InstallQuiltLoader(a.ctx, profile, a.emitInstallProgress)
		if err != nil {
			message := "Quilt install failed"
			if repair {
				message = "Quilt repair failed"
			}
			failed, stateErr := a.profileService.SetInstallState(id, domain.InstallState{
				Status:      "failed",
				Installed:   false,
				Message:     message,
				UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				LastError:   err.Error(),
				BaseVersion: profile.MinecraftVersion,
			})
			a.emitInstallProgress(domain.InstallProgress{
				ProfileID: id,
				Stage:     "failed",
				Message:   message,
				Done:      true,
				Error:     err.Error(),
			})
			if stateErr != nil {
				return domain.Profile{}, stateErr
			}
			return failed, err
		}
		if repair {
			message = "Repaired Quilt loader " + loaderVersion
		} else {
			message = "Installed Quilt loader " + loaderVersion
		}
	case domain.LoaderForge, domain.LoaderNeoForge:
		loaderName := "Forge"
		installLoader := a.minecraftService.InstallForgeLoader
		if profile.Loader.Type == domain.LoaderNeoForge {
			loaderName = "NeoForge"
			installLoader = a.minecraftService.InstallNeoForgeLoader
		}
		currentSettings, settingsErr := a.settingsService.Get()
		if settingsErr != nil {
			return domain.Profile{}, settingsErr
		}
		javaPath, _, javaErr := a.ensureProfileJavaRuntime(profile, currentSettings.JavaPath)
		if javaErr != nil {
			message := loaderName + " install failed"
			if repair {
				message = loaderName + " repair failed"
			}
			failed, stateErr := a.profileService.SetInstallState(id, domain.InstallState{
				Status:      "failed",
				Installed:   false,
				Message:     message,
				UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				LastError:   javaErr.Error(),
				BaseVersion: profile.MinecraftVersion,
			})
			a.emitInstallProgress(domain.InstallProgress{
				ProfileID: id,
				Stage:     "failed",
				Message:   message,
				Done:      true,
				Error:     javaErr.Error(),
			})
			if stateErr != nil {
				return domain.Profile{}, stateErr
			}
			return failed, javaErr
		}
		loaderVersion, err := installLoader(a.ctx, profile, javaPath, a.emitInstallProgress)
		if err != nil {
			message := loaderName + " install failed"
			if repair {
				message = loaderName + " repair failed"
			}
			failed, stateErr := a.profileService.SetInstallState(id, domain.InstallState{
				Status:      "failed",
				Installed:   false,
				Message:     message,
				UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
				LastError:   err.Error(),
				BaseVersion: profile.MinecraftVersion,
			})
			a.emitInstallProgress(domain.InstallProgress{
				ProfileID: id,
				Stage:     "failed",
				Message:   message,
				Done:      true,
				Error:     err.Error(),
			})
			if stateErr != nil {
				return domain.Profile{}, stateErr
			}
			return failed, err
		}
		if repair {
			message = "Repaired " + loaderName + " loader " + loaderVersion
		} else {
			message = "Installed " + loaderName + " loader " + loaderVersion
		}
	}

	return a.profileService.SetInstallState(id, domain.InstallState{
		Status:      finalStatus,
		Installed:   installed,
		Message:     message,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		BaseVersion: profile.MinecraftVersion,
	})
}

func (a *App) LaunchProfile(id string) (domain.LaunchState, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LaunchState{}, err
	}

	if err := a.reserveLaunch(id); err != nil {
		return domain.LaunchState{}, err
	}
	reserved := true
	defer func() {
		if reserved {
			a.releaseLaunch(id)
		}
	}()

	prepareTime := time.Now().UTC().Format(time.RFC3339)
	a.emitLaunchEvent(domain.LaunchEvent{
		ProfileID: id,
		Status:    domain.LaunchStarting,
		Message:   "Preparing Minecraft launch",
		Time:      prepareTime,
	})

	profile, err := a.profileService.Get(id)
	if err != nil {
		return domain.LaunchState{}, err
	}
	if profile.Loader.Type != domain.LoaderVanilla && profile.Loader.Type != domain.LoaderFabric && profile.Loader.Type != domain.LoaderQuilt && profile.Loader.Type != domain.LoaderForge && profile.Loader.Type != domain.LoaderNeoForge {
		return domain.LaunchState{}, fmt.Errorf("loader %q launch is not implemented right now", profile.Loader.Type)
	}
	if profile.Install.Status != "installed" {
		return domain.LaunchState{}, fmt.Errorf("profile is not installed")
	}

	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.LaunchState{}, err
	}
	javaPath, requiredJava, err := a.javaPathForProfile(profile, currentSettings.JavaPath)
	if err != nil {
		return domain.LaunchState{}, err
	}
	javaStatus := a.javaService.Validate(a.ctx, javaPath)
	if !javaStatus.OK {
		return domain.LaunchState{}, errors.New(javaStatus.Message)
	}
	if requiredJava > 0 && !javasvc.CompatibleMajor(javasvc.MajorVersion(javaStatus.Version), requiredJava) {
		return domain.LaunchState{}, fmt.Errorf("minecraft %s requires Java %d; install Java %d from Library", profile.MinecraftVersion, requiredJava, requiredJava)
	}

	commandSpec, err := a.minecraftService.BuildLaunchCommand(a.ctx, profile, minecraft.LaunchOptions{
		JavaPath: javaPath,
		Memory:   profile.Memory,
		Account:  currentSettings.Account,
	})
	if err != nil {
		return domain.LaunchState{}, err
	}

	command := exec.CommandContext(a.ctx, commandSpec.JavaPath, commandSpec.Args...)
	command.Dir = commandSpec.WorkDir

	stdout, err := command.StdoutPipe()
	if err != nil {
		return domain.LaunchState{}, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return domain.LaunchState{}, err
	}

	startedAt := time.Now().UTC().Format(time.RFC3339)
	if err := command.Start(); err != nil {
		a.emitLaunchEvent(domain.LaunchEvent{
			ProfileID: id,
			Status:    domain.LaunchFailed,
			Message:   "Failed to start Java: " + err.Error(),
			Time:      time.Now().UTC().Format(time.RFC3339),
		})
		return domain.LaunchState{}, err
	}

	a.markLaunchRunning(id, command)
	reserved = false

	a.emitLaunchEvent(domain.LaunchEvent{
		ProfileID: id,
		Status:    domain.LaunchRunning,
		Message:   "Minecraft process started",
		Time:      startedAt,
	})

	go a.streamLaunchOutput(id, "stdout", stdout)
	go a.streamLaunchOutput(id, "stderr", stderr)
	go a.waitForLaunch(id, command)

	return domain.LaunchState{
		ProfileID: id,
		Status:    domain.LaunchRunning,
		Message:   "Minecraft process started",
		StartedAt: startedAt,
	}, nil
}

func (a *App) StartLocalServer(id string) (domain.LocalServerRunState, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LocalServerRunState{}, err
	}

	runningKey := localServerRunningKey(id)
	if err := a.reserveLaunch(runningKey); err != nil {
		return domain.LocalServerRunState{}, err
	}
	reserved := true
	defer func() {
		if reserved {
			a.releaseLaunch(runningKey)
		}
	}()

	a.emitLocalServerEvent(domain.LocalServerEvent{
		ServerID: id,
		Status:   domain.LaunchStarting,
		Message:  "Preparing local server",
		Time:     time.Now().UTC().Format(time.RFC3339),
	})

	server, err := a.serverService.Get(id)
	if err != nil {
		return domain.LocalServerRunState{}, err
	}
	if server.Install.Status != "installed" {
		return domain.LocalServerRunState{}, fmt.Errorf("local server is not installed")
	}
	if !server.EulaAccepted {
		return domain.LocalServerRunState{}, fmt.Errorf("minecraft EULA must be accepted before starting the server")
	}
	if err := ensureLocalServerPortAvailable(server); err != nil {
		a.emitLocalServerEvent(domain.LocalServerEvent{
			ServerID: id,
			Status:   domain.LaunchFailed,
			Message:  err.Error(),
			Time:     time.Now().UTC().Format(time.RFC3339),
		})
		return domain.LocalServerRunState{}, err
	}

	currentSettings, err := a.settingsService.Get()
	if err != nil {
		return domain.LocalServerRunState{}, err
	}
	javaPath, requiredJava, err := a.ensureMinecraftJavaRuntime(server.MinecraftVersion, currentSettings.JavaPath, func(required int) {
		a.emitLocalServerProgress(domain.LocalServerProgress{
			ServerID: id,
			Stage:    "java-runtime",
			Message:  fmt.Sprintf("Installing Java %d runtime", required),
			Percent:  10,
		})
	})
	if err != nil {
		return domain.LocalServerRunState{}, err
	}
	if requiredJava > 0 {
		a.emitLocalServerProgress(domain.LocalServerProgress{
			ServerID: id,
			Stage:    "java-runtime",
			Message:  fmt.Sprintf("Using Java %d runtime", requiredJava),
			Percent:  20,
		})
	}

	commandSpec, err := minecraft.BuildLocalServerLaunchCommand(server, minecraft.ServerLaunchOptions{
		JavaPath: javaPath,
		Memory:   server.Memory,
	})
	if err != nil {
		return domain.LocalServerRunState{}, err
	}

	command := exec.CommandContext(a.ctx, commandSpec.JavaPath, commandSpec.Args...)
	command.Dir = commandSpec.WorkDir
	stdin, err := command.StdinPipe()
	if err != nil {
		return domain.LocalServerRunState{}, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return domain.LocalServerRunState{}, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return domain.LocalServerRunState{}, err
	}

	startedAtTime := time.Now().UTC()
	startedAt := startedAtTime.Format(time.RFC3339)
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		a.emitLocalServerEvent(domain.LocalServerEvent{
			ServerID: id,
			Status:   domain.LaunchFailed,
			Message:  "Failed to start Java: " + err.Error(),
			Time:     time.Now().UTC().Format(time.RFC3339),
		})
		return domain.LocalServerRunState{}, err
	}

	done := make(chan struct{})
	a.markLocalServerRunning(runningKey, command, stdin, done)
	reserved = false

	a.emitLocalServerEvent(domain.LocalServerEvent{
		ServerID: id,
		Status:   domain.LaunchRunning,
		Message:  "Local server process started",
		Time:     startedAt,
	})

	go a.streamLocalServerOutput(id, "stdout", stdout)
	go a.streamLocalServerOutput(id, "stderr", stderr)
	go a.waitForLocalServer(id, runningKey, command, startedAtTime, done)

	return domain.LocalServerRunState{
		ServerID:  id,
		Status:    domain.LaunchRunning,
		Message:   "Local server process started",
		StartedAt: startedAt,
	}, nil
}

func (a *App) StopLocalServer(id string) (domain.LocalServerRunState, error) {
	if err := a.ensureReady(); err != nil {
		return domain.LocalServerRunState{}, err
	}
	runningKey := localServerRunningKey(id)
	command, stdin, err := a.runningLocalServer(runningKey)
	if err != nil {
		return domain.LocalServerRunState{}, err
	}
	if command.Process == nil {
		return domain.LocalServerRunState{}, fmt.Errorf("local server process is not running")
	}
	a.markLaunchStopping(runningKey)
	message := "Stop command sent"
	if stdin != nil {
		if _, err := io.WriteString(stdin, "stop\n"); err == nil {
			now := time.Now().UTC().Format(time.RFC3339)
			a.emitLocalServerEvent(domain.LocalServerEvent{
				ServerID: id,
				Status:   domain.LaunchRunning,
				Message:  message,
				Time:     now,
			})
			return domain.LocalServerRunState{
				ServerID: id,
				Status:   domain.LaunchRunning,
				Message:  message,
			}, nil
		}
	}
	message = "Stop signal sent"
	if err := command.Process.Signal(os.Interrupt); err != nil {
		if killErr := command.Process.Kill(); killErr != nil {
			return domain.LocalServerRunState{}, fmt.Errorf("stop local server: %w", err)
		}
		message = "Stop kill sent"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	a.emitLocalServerEvent(domain.LocalServerEvent{
		ServerID: id,
		Status:   domain.LaunchRunning,
		Message:  message,
		Time:     now,
	})
	return domain.LocalServerRunState{
		ServerID: id,
		Status:   domain.LaunchRunning,
		Message:  message,
	}, nil
}

func (a *App) javaPathForProfile(profile domain.Profile, fallbackPath string) (string, int, error) {
	runtime, err := a.profileJavaRuntime(profile, fallbackPath)
	if err != nil {
		return "", 0, err
	}
	if runtime.Installed {
		return runtime.JavaPath, runtime.RequiredMajor, nil
	}
	return "", runtime.RequiredMajor, fmt.Errorf("minecraft %s requires Java %d; install Java %d from Library", profile.MinecraftVersion, runtime.RequiredMajor, runtime.RequiredMajor)
}

func (a *App) ensureProfileJavaRuntime(profile domain.Profile, fallbackPath string) (string, int, error) {
	runtime, err := a.profileJavaRuntime(profile, fallbackPath)
	if err != nil {
		return "", 0, err
	}
	if runtime.Installed {
		return runtime.JavaPath, runtime.RequiredMajor, nil
	}
	if runtime.RequiredMajor <= 0 {
		return "", 0, fmt.Errorf("minecraft %s has no resolvable Java runtime requirement", profile.MinecraftVersion)
	}

	a.emitInstallProgress(domain.InstallProgress{
		ProfileID: profile.ID,
		Stage:     "java-runtime",
		Message:   fmt.Sprintf("Installing Java %d runtime", runtime.RequiredMajor),
		Percent:   80,
	})

	javaPath, err := a.javaService.InstallTemurin(a.ctx, runtime.RequiredMajor, a.emitJavaProgress)
	if err != nil {
		return "", runtime.RequiredMajor, err
	}

	status := a.javaService.Validate(a.ctx, javaPath)
	if !status.OK {
		return "", runtime.RequiredMajor, errors.New(status.Message)
	}
	if !javasvc.CompatibleMajor(javasvc.MajorVersion(status.Version), runtime.RequiredMajor) {
		return "", runtime.RequiredMajor, fmt.Errorf("minecraft %s requires Java %d; installed runtime is Java %s", profile.MinecraftVersion, runtime.RequiredMajor, status.Version)
	}

	a.emitInstallProgress(domain.InstallProgress{
		ProfileID: profile.ID,
		Stage:     "java-runtime",
		Message:   fmt.Sprintf("Java %d runtime installed", runtime.RequiredMajor),
		Percent:   81,
	})
	return javaPath, runtime.RequiredMajor, nil
}

func (a *App) ensureMinecraftJavaRuntime(minecraftVersion string, fallbackPath string, beforeInstall func(required int)) (string, int, error) {
	requiredJava, err := a.minecraftService.RequiredJavaVersion(minecraftVersion)
	if err != nil {
		return "", 0, err
	}
	if requiredJava <= 0 {
		return "", 0, fmt.Errorf("minecraft %s has no resolvable Java runtime requirement", minecraftVersion)
	}
	if javaPath, ok := a.javaService.InstalledTemurin(a.ctx, requiredJava); ok {
		return javaPath, requiredJava, nil
	}
	status := a.javaService.Validate(a.ctx, fallbackPath)
	if status.OK && javasvc.CompatibleMajor(javasvc.MajorVersion(status.Version), requiredJava) {
		return fallbackPath, requiredJava, nil
	}
	if beforeInstall != nil {
		beforeInstall(requiredJava)
	}
	javaPath, err := a.javaService.InstallTemurin(a.ctx, requiredJava, a.emitJavaProgress)
	if err != nil {
		return "", requiredJava, err
	}
	status = a.javaService.Validate(a.ctx, javaPath)
	if !status.OK {
		return "", requiredJava, errors.New(status.Message)
	}
	if !javasvc.CompatibleMajor(javasvc.MajorVersion(status.Version), requiredJava) {
		return "", requiredJava, fmt.Errorf("minecraft %s requires Java %d; installed runtime is Java %s", minecraftVersion, requiredJava, status.Version)
	}
	return javaPath, requiredJava, nil
}

func (a *App) profileJavaRuntime(profile domain.Profile, fallbackPath string) (domain.ProfileJavaRuntime, error) {
	requiredJava, err := a.minecraftService.RequiredJavaVersion(profile.MinecraftVersion)
	if err != nil {
		return domain.ProfileJavaRuntime{}, err
	}

	runtime := domain.ProfileJavaRuntime{
		ProfileID:     profile.ID,
		RequiredMajor: requiredJava,
		Message:       fmt.Sprintf("Requires Java %d", requiredJava),
	}

	if javaPath, ok := a.javaService.InstalledTemurin(a.ctx, requiredJava); ok {
		status := a.javaService.Validate(a.ctx, javaPath)
		runtime.Installed = status.OK
		runtime.JavaPath = javaPath
		runtime.Version = status.Version
		runtime.Message = fmt.Sprintf("Java %d runtime is installed", requiredJava)
		if !status.OK {
			runtime.Message = status.Message
		}
		return runtime, nil
	}

	status := a.javaService.Validate(a.ctx, fallbackPath)
	if status.OK && javasvc.CompatibleMajor(javasvc.MajorVersion(status.Version), requiredJava) {
		runtime.Installed = true
		runtime.JavaPath = fallbackPath
		runtime.Version = status.Version
		runtime.Message = fmt.Sprintf("Current Java path satisfies Java %d", requiredJava)
		return runtime, nil
	}

	runtime.Message = fmt.Sprintf("Install Java %d to launch Minecraft %s", requiredJava, profile.MinecraftVersion)
	return runtime, nil
}

func (a *App) reserveLaunch(profileID string) error {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	if _, ok := a.running[profileID]; ok {
		return fmt.Errorf("process is already running")
	}
	a.running[profileID] = nil
	return nil
}

func (a *App) markLaunchRunning(profileID string, command *exec.Cmd) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	a.running[profileID] = command
}

func (a *App) markLocalServerRunning(profileID string, command *exec.Cmd, stdin io.WriteCloser, done chan struct{}) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	a.running[profileID] = command
	a.launchDone[profileID] = done
	if a.serverInputs == nil {
		a.serverInputs = make(map[string]io.WriteCloser)
	}
	a.serverInputs[profileID] = stdin
}

func (a *App) runningCommand(profileID string) (*exec.Cmd, error) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	command, ok := a.running[profileID]
	if !ok || command == nil {
		return nil, fmt.Errorf("process is not running")
	}
	return command, nil
}

func (a *App) runningLocalServer(profileID string) (*exec.Cmd, io.WriteCloser, error) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	command, ok := a.running[profileID]
	if !ok || command == nil {
		return nil, nil, fmt.Errorf("process is not running")
	}
	return command, a.serverInputs[profileID], nil
}

func (a *App) markLaunchStopping(profileID string) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	if a.stopping == nil {
		a.stopping = make(map[string]bool)
	}
	a.stopping[profileID] = true
}

func (a *App) takeLaunchStopping(profileID string) bool {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	stopping := a.stopping[profileID]
	delete(a.stopping, profileID)
	return stopping
}

func (a *App) releaseLaunch(profileID string) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	if input := a.serverInputs[profileID]; input != nil {
		_ = input.Close()
	}
	delete(a.running, profileID)
	delete(a.launchDone, profileID)
	delete(a.serverInputs, profileID)
	delete(a.stopping, profileID)
}

func (a *App) emitInstallProgress(event domain.InstallProgress) {
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "install:progress", event)
}

func (a *App) emitJavaProgress(event domain.JavaInstallProgress) {
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "java:progress", event)
}

func (a *App) emitLocalServerProgress(event domain.LocalServerProgress) {
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "server:progress", event)
}

func (a *App) streamLaunchOutput(profileID string, stream string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		a.emitLaunchEvent(domain.LaunchEvent{
			ProfileID: profileID,
			Status:    domain.LaunchRunning,
			Stream:    stream,
			Message:   scanner.Text(),
			Time:      time.Now().UTC().Format(time.RFC3339),
		})
	}
	if err := scanner.Err(); err != nil {
		a.emitLaunchEvent(domain.LaunchEvent{
			ProfileID: profileID,
			Status:    domain.LaunchFailed,
			Stream:    stream,
			Message:   "Log stream failed: " + err.Error(),
			Time:      time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func (a *App) streamLocalServerOutput(serverID string, stream string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		a.emitLocalServerEvent(domain.LocalServerEvent{
			ServerID: serverID,
			Status:   domain.LaunchRunning,
			Stream:   stream,
			Message:  scanner.Text(),
			Time:     time.Now().UTC().Format(time.RFC3339),
		})
	}
	if err := scanner.Err(); err != nil {
		a.emitLocalServerEvent(domain.LocalServerEvent{
			ServerID: serverID,
			Status:   domain.LaunchFailed,
			Stream:   stream,
			Message:  "Log stream failed: " + err.Error(),
			Time:     time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func (a *App) waitForLaunch(profileID string, command *exec.Cmd) {
	err := command.Wait()
	exitCode := command.ProcessState.ExitCode()
	status := domain.LaunchStopped
	message := "Minecraft process stopped"
	if err != nil {
		status = domain.LaunchFailed
		message = "Minecraft process failed: " + err.Error()
	}

	a.releaseLaunch(profileID)

	a.emitLaunchEvent(domain.LaunchEvent{
		ProfileID: profileID,
		Status:    status,
		Message:   message,
		ExitCode:  exitCode,
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
}

func (a *App) waitForLocalServer(serverID string, runningKey string, command *exec.Cmd, startedAt time.Time, done chan struct{}) {
	defer close(done)
	err := command.Wait()
	exitCode := 0
	if command.ProcessState != nil {
		exitCode = command.ProcessState.ExitCode()
	}
	stopping := a.takeLaunchStopping(runningKey)
	status, message := localServerExitStatus(err, stopping, time.Since(startedAt))

	a.releaseLaunch(runningKey)

	a.emitLocalServerEvent(domain.LocalServerEvent{
		ServerID: serverID,
		Status:   status,
		Message:  message,
		ExitCode: exitCode,
		Time:     time.Now().UTC().Format(time.RFC3339),
	})
}

func localServerExitStatus(err error, stopping bool, ranFor time.Duration) (domain.LaunchStatus, string) {
	if stopping {
		return domain.LaunchStopped, "Local server process stopped"
	}
	if err != nil {
		return domain.LaunchFailed, "Local server process failed: " + err.Error()
	}
	if ranFor > 0 && ranFor < localServerStartupFailureWindow {
		return domain.LaunchFailed, "Local server exited during startup"
	}
	return domain.LaunchStopped, "Local server process stopped"
}

func (a *App) emitLaunchEvent(event domain.LaunchEvent) {
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "launch:event", event)
}

func (a *App) emitLocalServerEvent(event domain.LocalServerEvent) {
	a.recordLocalServerEvent(event)
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "server:event", event)
}

func (a *App) setLogsSnapshot(snapshot string) error {
	snapshot = strings.TrimSpace(snapshot)
	if snapshot == "" {
		snapshot = `{"logs":[],"profiles":[],"localServers":[]}`
	}
	if !json.Valid([]byte(snapshot)) {
		return fmt.Errorf("detached logs snapshot is not valid JSON")
	}
	a.externalMu.Lock()
	a.logsSnapshot = snapshot
	a.externalMu.Unlock()
	return nil
}

func (a *App) externalWindowURL(path string) (string, error) {
	if err := a.ensureExternalWindowServer(); err != nil {
		return "", err
	}
	a.externalMu.RLock()
	baseURL := a.externalBaseURL
	token := a.externalToken
	a.externalMu.RUnlock()
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return baseURL + path + separator + "token=" + url.QueryEscape(token), nil
}

func (a *App) ensureExternalWindowServer() error {
	a.externalMu.Lock()
	defer a.externalMu.Unlock()
	if a.externalServer != nil && a.externalBaseURL != "" {
		return nil
	}

	token, err := randomToken()
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start external window server: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/logs", a.handleDetachedLogsPage)
	mux.HandleFunc("/server-terminal/", a.handleServerTerminalPage)
	mux.HandleFunc("/api/logs", a.handleDetachedLogsAPI)
	mux.HandleFunc("/api/server/", a.handleServerTerminalAPI)

	server := &http.Server{Handler: mux}
	a.externalServer = server
	a.externalBaseURL = "http://" + listener.Addr().String()
	a.externalToken = token
	if a.logsSnapshot == "" {
		a.logsSnapshot = `{"logs":[],"profiles":[],"localServers":[]}`
	}

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "external window server stopped: %v\n", err)
		}
	}()
	return nil
}

func (a *App) shutdownExternalWindowServer(ctx context.Context) {
	a.externalMu.Lock()
	server := a.externalServer
	a.externalServer = nil
	a.externalBaseURL = ""
	a.externalToken = ""
	a.externalMu.Unlock()
	if server != nil {
		shutdownCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}
}

func (a *App) externalRequestAllowed(r *http.Request) bool {
	a.externalMu.RLock()
	expected := a.externalToken
	a.externalMu.RUnlock()
	return expected != "" && r.URL.Query().Get("token") == expected
}

func (a *App) handleDetachedLogsPage(w http.ResponseWriter, r *http.Request) {
	if !a.externalRequestAllowed(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	a.externalMu.RLock()
	token := a.externalToken
	a.externalMu.RUnlock()
	writeHTML(w, detachedLogsPageHTML(token))
}

func (a *App) handleDetachedLogsAPI(w http.ResponseWriter, r *http.Request) {
	if !a.externalRequestAllowed(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	a.externalMu.RLock()
	snapshot := a.logsSnapshot
	a.externalMu.RUnlock()
	if snapshot == "" {
		snapshot = `{"logs":[],"profiles":[],"localServers":[]}`
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.WriteString(w, snapshot)
}

func (a *App) handleServerTerminalPage(w http.ResponseWriter, r *http.Request) {
	if !a.externalRequestAllowed(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	serverID, ok := terminalServerIDFromPath(r.URL.Path, "/server-terminal/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	server, err := a.serverService.Get(serverID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	a.externalMu.RLock()
	token := a.externalToken
	a.externalMu.RUnlock()
	writeHTML(w, serverTerminalPageHTML(token, server))
}

func (a *App) handleServerTerminalAPI(w http.ResponseWriter, r *http.Request) {
	if !a.externalRequestAllowed(r) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	serverID, action, ok := terminalAPIPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch action {
	case "history":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"events": a.localServerEventHistory(serverID)})
	case "command":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Command string `json:"command"`
		}
		body := http.MaxBytesReader(w, r.Body, 16*1024)
		defer body.Close()
		if err := json.NewDecoder(body).Decode(&payload); err != nil {
			http.Error(w, "invalid command payload", http.StatusBadRequest)
			return
		}
		if err := a.sendLocalServerCommand(serverID, payload.Command); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "sent"})
	default:
		http.NotFound(w, r)
	}
}

func (a *App) recordLocalServerEvent(event domain.LocalServerEvent) {
	if event.ServerID == "" {
		return
	}
	a.externalMu.Lock()
	defer a.externalMu.Unlock()
	if a.serverEvents == nil {
		a.serverEvents = make(map[string][]domain.LocalServerEvent)
	}
	events := append(a.serverEvents[event.ServerID], event)
	if len(events) > maxLocalServerTerminalEvents {
		events = append([]domain.LocalServerEvent(nil), events[len(events)-maxLocalServerTerminalEvents:]...)
	}
	a.serverEvents[event.ServerID] = events
}

func (a *App) localServerEventHistory(serverID string) []domain.LocalServerEvent {
	a.externalMu.RLock()
	defer a.externalMu.RUnlock()
	events := a.serverEvents[serverID]
	return append([]domain.LocalServerEvent(nil), events...)
}

func (a *App) sendLocalServerCommand(serverID string, command string) error {
	command = strings.TrimSpace(strings.ReplaceAll(command, "\r", ""))
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("server command is empty")
	}
	runningKey := localServerRunningKey(serverID)
	_, stdin, err := a.runningLocalServer(runningKey)
	if err != nil {
		return fmt.Errorf("local server is not running")
	}
	if stdin == nil {
		return fmt.Errorf("local server console is not available")
	}
	stoppingCommand := strings.EqualFold(command, "stop")
	if stoppingCommand {
		a.markLaunchStopping(runningKey)
	}
	if _, err := io.WriteString(stdin, command+"\n"); err != nil {
		if stoppingCommand {
			_ = a.takeLaunchStopping(runningKey)
		}
		return fmt.Errorf("send local server command: %w", err)
	}
	a.emitLocalServerEvent(domain.LocalServerEvent{
		ServerID: serverID,
		Status:   domain.LaunchRunning,
		Stream:   "stdin",
		Message:  "> " + command,
		Time:     time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

type runningLocalServerProcess struct {
	runningKey string
	command    *exec.Cmd
	stdin      io.WriteCloser
	done       <-chan struct{}
}

func (a *App) runningLocalServerProcesses() []runningLocalServerProcess {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	if a.stopping == nil {
		a.stopping = make(map[string]bool)
	}
	processes := make([]runningLocalServerProcess, 0)
	for runningKey, command := range a.running {
		if !strings.HasPrefix(runningKey, "server:") || command == nil {
			continue
		}
		processes = append(processes, runningLocalServerProcess{
			runningKey: runningKey,
			command:    command,
			stdin:      a.serverInputs[runningKey],
			done:       a.launchDone[runningKey],
		})
		a.stopping[runningKey] = true
	}
	return processes
}

func (a *App) stopRunningLocalServers(timeout time.Duration) {
	processes := a.runningLocalServerProcesses()
	if len(processes) == 0 {
		return
	}
	for _, process := range processes {
		if process.stdin != nil {
			_, _ = io.WriteString(process.stdin, "stop\n")
			_ = process.stdin.Close()
			continue
		}
		if process.command.Process != nil {
			_ = process.command.Process.Signal(os.Interrupt)
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for i, process := range processes {
		if process.done == nil {
			continue
		}
		select {
		case <-process.done:
			continue
		case <-timer.C:
			for _, remaining := range processes[i:] {
				if remaining.command.Process != nil {
					_ = remaining.command.Process.Kill()
				}
			}
			return
		}
	}
}

func ensureLocalServerPortAvailable(server domain.LocalServer) error {
	if server.Port <= 0 {
		return nil
	}
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", server.Port))
	if err != nil {
		return fmt.Errorf("local server port %d is already in use; stop the other server or choose another port", server.Port)
	}
	return listener.Close()
}

func terminalServerIDFromPath(path string, prefix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rawID := strings.TrimPrefix(path, prefix)
	if rawID == "" || strings.Contains(rawID, "/") {
		return "", false
	}
	serverID, err := url.PathUnescape(rawID)
	if err != nil || serverID == "" {
		return "", false
	}
	return serverID, true
}

func terminalAPIPath(path string) (string, string, bool) {
	const prefix = "/api/server/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	serverID, err := url.PathUnescape(parts[0])
	if err != nil || serverID == "" {
		return "", "", false
	}
	return serverID, parts[1], true
}

func writeHTML(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, content)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("create external window token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func jsonString(value string) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(b)
}

func detachedLogsPageHTML(token string) string {
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Power Mine Logs</title>
<style>
:root{color-scheme:dark;--bg:#050505;--panel:#121212;--surface:#0d0d0d;--text:#fff;--muted:rgb(255 255 255 / 70%);--border:#fff;--ok:#4ade80;--error:#f87171;font-family:"SFMono-Regular","Cascadia Code","Liberation Mono",Menlo,Consolas,monospace}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text)}header{position:sticky;top:0;z-index:1;border-bottom:2px solid var(--border);background:var(--panel);padding:16px 18px;display:flex;align-items:flex-end;justify-content:space-between;gap:18px}h1,p{margin:0}.eyebrow{color:var(--muted);font-size:11px;font-weight:700;letter-spacing:.16em;text-transform:uppercase}h1{margin-top:6px;font-size:28px;letter-spacing:-.04em}.meta{color:var(--muted);font-size:12px;text-align:right}.feed{display:grid;gap:8px;padding:14px}.row{display:grid;grid-template-columns:86px 72px 150px 190px minmax(0,1fr);gap:10px;border:1px solid rgb(255 255 255 / 32%);background:var(--surface);padding:9px 10px}.row.success .level{color:var(--ok)}.row.error{border-color:var(--error);background:#1a0d0d}.row.error .level{color:var(--error)}.time,.source,.target{color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.level{font-weight:800;text-transform:uppercase}.message{min-width:0;white-space:pre-wrap;overflow-wrap:anywhere}.empty{border:2px dashed rgb(255 255 255 / 40%);padding:24px;color:var(--muted)}@media(max-width:820px){header{display:block}.meta{text-align:left;margin-top:10px}.row{grid-template-columns:1fr}.time,.source,.target{white-space:normal}}
</style>
</head>
<body>
<header><div><p class="eyebrow">Power Mine</p><h1>Detached logs</h1></div><p class="meta" id="meta">Loading...</p></header>
<main class="feed" id="feed"><p class="empty">Waiting for launcher logs.</p></main>
<script>
const token = ` + jsonString(token) + `;
const feed = document.getElementById('feed');
const meta = document.getElementById('meta');
function esc(value){return String(value ?? '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));}
function render(data){
  const logs = Array.isArray(data.logs) ? data.logs : [];
  meta.innerHTML = esc(logs.length) + ' events<br/>Updated ' + esc(new Date().toLocaleTimeString());
  if (logs.length === 0) { feed.innerHTML = '<p class="empty">No launcher events yet.</p>'; return; }
  feed.innerHTML = logs.slice(0, 1000).map(log => '<article class="row '+esc(log.level)+'"><span class="time">'+esc(log.time)+'</span><span class="level">'+esc(log.level)+'</span><span class="source" title="'+esc(log.source)+'">'+esc(log.source)+'</span><span class="target" title="'+esc(log.target)+'">'+esc(log.target)+'</span><span class="message">'+esc(log.message)+'</span></article>').join('');
}
async function refresh(){
  try {
    const response = await fetch('/api/logs?token=' + encodeURIComponent(token), {cache:'no-store'});
    if (!response.ok) throw new Error(await response.text());
    render(await response.json());
  } catch (error) {
    meta.textContent = 'Update failed';
    feed.innerHTML = '<p class="empty">' + esc(error.message || error) + '</p>';
  }
}
refresh();
setInterval(refresh, 1000);
</script>
</body>
</html>`
}

func serverTerminalPageHTML(token string, server domain.LocalServer) string {
	serverID := url.PathEscape(server.ID)
	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>Power Mine Terminal - ` + html.EscapeString(server.Name) + `</title>
<style>
:root{color-scheme:dark;--bg:#020202;--panel:#101010;--ink:#f8f8f8;--muted:rgb(248 248 248 / 66%);--line:rgb(248 248 248 / 24%);--green:#7cff9b;--red:#ff7777;--blue:#77d7ff;font-family:"SFMono-Regular","Cascadia Code","Liberation Mono",Menlo,Consolas,monospace}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:radial-gradient(circle at 20% 0%,#1d2b1f 0,#020202 34rem);color:var(--ink);display:grid;grid-template-rows:auto 1fr auto}header{border-bottom:1px solid var(--line);background:rgb(0 0 0 / 76%);padding:14px 16px;display:flex;justify-content:space-between;gap:16px;align-items:flex-end}h1,p{margin:0}.eyebrow{color:var(--green);font-size:11px;font-weight:800;letter-spacing:.16em;text-transform:uppercase}h1{margin-top:4px;font-size:23px}.meta{text-align:right;color:var(--muted);font-size:12px}.terminal{padding:14px;overflow:auto;white-space:pre-wrap;overflow-wrap:anywhere}.line{display:grid;grid-template-columns:82px 78px 72px minmax(0,1fr);gap:10px;padding:3px 0;border-bottom:1px solid rgb(255 255 255 / 4%)}.time,.stream,.status{color:var(--muted)}.stream.stdin{color:var(--blue)}.status.failed,.line.failed .msg{color:var(--red)}.status.running{color:var(--green)}.composer{border-top:1px solid var(--line);background:var(--panel);padding:12px 14px;display:flex;gap:10px}.prompt{color:var(--green);font-weight:800;padding-top:10px}input{flex:1;background:#050505;border:1px solid var(--line);color:var(--ink);font:inherit;padding:10px 12px}button{background:var(--green);border:0;color:#041007;font:inherit;font-weight:900;padding:10px 14px;cursor:pointer}.hint{color:var(--muted);font-size:12px;margin-top:4px}@media(max-width:760px){header{display:block}.meta{text-align:left;margin-top:8px}.line{grid-template-columns:1fr}.composer{align-items:stretch}.prompt{display:none}}
</style>
</head>
<body>
<header><div><p class="eyebrow">Power Mine server terminal</p><h1>` + html.EscapeString(server.Name) + `</h1><p class="hint">Type Minecraft server commands without a slash, for example: say Hello or stop.</p></div><p class="meta">` + html.EscapeString(server.MinecraftVersion) + `<br/>Port ` + fmt.Sprintf("%d", server.Port) + `</p></header>
<main class="terminal" id="terminal"><p class="hint">Loading terminal history...</p></main>
<form class="composer" id="composer"><span class="prompt">&gt;</span><input id="command" autocomplete="off" spellcheck="false" placeholder="server command"/><button type="submit">Send</button></form>
<script>
const token = ` + jsonString(token) + `;
const serverID = ` + jsonString(serverID) + `;
const terminal = document.getElementById('terminal');
const form = document.getElementById('composer');
const commandInput = document.getElementById('command');
let lastRendered = '';
function esc(value){return String(value ?? '').replace(/[&<>"']/g, char => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[char]));}
function lineKey(event){return [event.time,event.stream,event.status,event.message,event.exitCode].join('|');}
function render(events){
  const key = events.map(lineKey).join('\n');
  if (key === lastRendered) return;
  const stick = terminal.scrollTop + terminal.clientHeight >= terminal.scrollHeight - 24;
  lastRendered = key;
  terminal.innerHTML = events.length ? events.map(event => '<div class="line '+esc(event.status)+'"><span class="time">'+esc((event.time || '').slice(11,19))+'</span><span class="status '+esc(event.status)+'">'+esc(event.status || '')+'</span><span class="stream '+esc(event.stream)+'">'+esc(event.stream || 'event')+'</span><span class="msg">'+esc(event.message || '')+(event.exitCode !== undefined ? ' (exit '+esc(event.exitCode)+')' : '')+'</span></div>').join('') : '<p class="hint">No server output yet. Start the server from Power Mine, then keep this window open.</p>';
  if (stick) terminal.scrollTop = terminal.scrollHeight;
}
async function refresh(){
  try {
    const response = await fetch('/api/server/' + serverID + '/history?token=' + encodeURIComponent(token), {cache:'no-store'});
    if (!response.ok) throw new Error(await response.text());
    const data = await response.json();
    render(Array.isArray(data.events) ? data.events : []);
  } catch (error) {
    terminal.innerHTML = '<p class="hint">' + esc(error.message || error) + '</p>';
  }
}
form.addEventListener('submit', async event => {
  event.preventDefault();
  const command = commandInput.value.trim();
  if (!command) return;
  commandInput.value = '';
  try {
    const response = await fetch('/api/server/' + serverID + '/command?token=' + encodeURIComponent(token), {method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({command})});
    if (!response.ok) throw new Error(await response.text());
    refresh();
  } catch (error) {
    alert(error.message || error);
  }
});
refresh();
setInterval(refresh, 750);
commandInput.focus();
</script>
</body>
</html>`
}

func localServerRunningKey(id string) string {
	return "server:" + id
}

func modpackDefaultFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "modpack.mrpack"
	}

	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		case strings.ContainsRune(`/\:*?"<>|`, r) || r < 32:
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		default:
			builder.WriteRune(r)
			lastDash = false
		}
	}

	base := strings.Trim(builder.String(), "-")
	if base == "" {
		base = "modpack"
	}
	return base + ".mrpack"
}

func logsDefaultFilename(name string) string {
	base := strings.TrimSuffix(modpackDefaultFilename(name), ".mrpack")
	if base == "" {
		base = "profile"
	}
	return base + "-logs.zip"
}

func (a *App) modrinthExportFiles(profile domain.Profile) []modpacks.ExportFile {
	projects, err := a.modsService.ModrinthInstalledProjects(profile)
	if err != nil {
		return nil
	}

	result := make([]modpacks.ExportFile, 0)
	seen := map[string]bool{}
	for _, project := range projects {
		for _, installed := range project.Files {
			if strings.TrimSpace(installed.ProjectID) == "" || strings.TrimSpace(installed.VersionID) == "" || strings.TrimSpace(installed.FileName) == "" {
				continue
			}
			existing, ok, err := a.modsService.Existing(profile, installed.FileName)
			if err != nil || !ok || !existing.Enabled {
				continue
			}
			path := filepath.ToSlash(filepath.Join("mods", existing.FileName))
			key := strings.ToLower(path)
			if seen[key] {
				continue
			}

			version, err := a.catalogService.ModrinthProjectVersion(a.ctx, profile, installed.ProjectID, installed.VersionID)
			if err != nil {
				continue
			}
			if !modrinthExportFileMatches(existing, version.File) {
				continue
			}
			result = append(result, modpacks.ExportFile{
				Path:      path,
				Downloads: []string{version.File.URL},
				SHA1:      version.File.SHA1,
				FileSize:  version.File.Size,
			})
			seen[key] = true
		}
	}
	return result
}

func modrinthExportFileMatches(modFile domain.ModFile, versionFile domain.ModrinthVersionFile) bool {
	if strings.TrimSpace(versionFile.URL) == "" || strings.TrimSpace(versionFile.SHA1) == "" {
		return false
	}
	if modFile.Size > 0 && versionFile.Size > 0 && modFile.Size != versionFile.Size {
		return false
	}
	if strings.TrimSpace(modFile.SHA1) != "" && !strings.EqualFold(modFile.SHA1, versionFile.SHA1) {
		return false
	}
	return true
}

func (a *App) ensureReady() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.startupErr != nil {
		return a.startupErr
	}
	if a.settingsService == nil || a.profileService == nil || a.catalogService == nil || a.minecraftService == nil || a.javaService == nil || a.modpackService == nil || a.modsService == nil || a.serverService == nil {
		return errors.New("application services are not initialized")
	}
	return nil
}
