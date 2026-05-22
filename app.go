package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
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
	running          map[string]*exec.Cmd
	startupErr       error
	headless         bool
}

const maxModrinthDependencyDepth = 12

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
		running: make(map[string]*exec.Cmd),
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
	a.startupErr = nil
}

func (a *App) AppInfo() domain.AppInfo {
	return domain.AppInfo{
		Name:    platform.AppName,
		Version: "0.1.0",
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
		javaPath, _, javaErr := a.javaPathForProfile(profile, currentSettings.JavaPath)
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
		return fmt.Errorf("profile is already running")
	}
	a.running[profileID] = nil
	return nil
}

func (a *App) markLaunchRunning(profileID string, command *exec.Cmd) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	a.running[profileID] = command
}

func (a *App) releaseLaunch(profileID string) {
	a.launchMu.Lock()
	defer a.launchMu.Unlock()
	delete(a.running, profileID)
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

func (a *App) emitLaunchEvent(event domain.LaunchEvent) {
	if a.ctx == nil || a.headless {
		return
	}
	wailsruntime.EventsEmit(a.ctx, "launch:event", event)
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
	if a.settingsService == nil || a.profileService == nil || a.catalogService == nil || a.minecraftService == nil || a.javaService == nil || a.modpackService == nil || a.modsService == nil {
		return errors.New("application services are not initialized")
	}
	return nil
}
