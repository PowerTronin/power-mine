package domain

type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type VersionOption struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Type   string `json:"type,omitempty"`
	Stable bool   `json:"stable"`
	Latest bool   `json:"latest"`
}

type VersionCatalog struct {
	MinecraftVersions       []VersionOption `json:"minecraftVersions"`
	FabricLoaderVersions    []VersionOption `json:"fabricLoaderVersions"`
	QuiltLoaderVersions     []VersionOption `json:"quiltLoaderVersions"`
	ForgeLoaderVersions     []VersionOption `json:"forgeLoaderVersions"`
	NeoForgeLoaderVersions  []VersionOption `json:"neoForgeLoaderVersions"`
	MinecraftSource         string          `json:"minecraftSource"`
	FabricLoaderSource      string          `json:"fabricLoaderSource"`
	QuiltLoaderSource       string          `json:"quiltLoaderSource"`
	ForgeLoaderSource       string          `json:"forgeLoaderSource"`
	NeoForgeLoaderSource    string          `json:"neoForgeLoaderSource"`
	MinecraftUpdatedAt      string          `json:"minecraftUpdatedAt,omitempty"`
	FabricLoaderUpdatedAt   string          `json:"fabricLoaderUpdatedAt,omitempty"`
	QuiltLoaderUpdatedAt    string          `json:"quiltLoaderUpdatedAt,omitempty"`
	ForgeLoaderUpdatedAt    string          `json:"forgeLoaderUpdatedAt,omitempty"`
	NeoForgeLoaderUpdatedAt string          `json:"neoForgeLoaderUpdatedAt,omitempty"`
	Warnings                []string        `json:"warnings,omitempty"`
}

type LoaderType string

const (
	LoaderVanilla  LoaderType = "vanilla"
	LoaderFabric   LoaderType = "fabric"
	LoaderQuilt    LoaderType = "quilt"
	LoaderForge    LoaderType = "forge"
	LoaderNeoForge LoaderType = "neoforge"
)

type LoaderConfig struct {
	Type    LoaderType `json:"type"`
	Version string     `json:"version,omitempty"`
}

type AccountMode string

const (
	AccountOffline   AccountMode = "offline"
	AccountMicrosoft AccountMode = "microsoft"
)

type AccountConfig struct {
	Mode        AccountMode `json:"mode"`
	OfflineName string      `json:"offlineName,omitempty"`
	OfflineUUID string      `json:"offlineUuid,omitempty"`
}

type MemorySettings struct {
	MinMB int `json:"minMB"`
	MaxMB int `json:"maxMB"`
}

type NetworkSettings struct {
	RetryCount       int `json:"retryCount"`
	MetadataTTLHours int `json:"metadataTtlHours"`
}

type Settings struct {
	DataDir       string          `json:"dataDir"`
	JavaPath      string          `json:"javaPath"`
	Account       AccountConfig   `json:"account"`
	DefaultMemory MemorySettings  `json:"defaultMemory"`
	Network       NetworkSettings `json:"network"`
}

type InstallState struct {
	Status      string `json:"status"`
	Installed   bool   `json:"installed"`
	Message     string `json:"message,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	BaseVersion string `json:"baseVersion,omitempty"`
}

type InstallProgress struct {
	ProfileID string `json:"profileId"`
	Stage     string `json:"stage"`
	Message   string `json:"message"`
	Current   int    `json:"current"`
	Total     int    `json:"total"`
	Percent   int    `json:"percent"`
	Done      bool   `json:"done"`
	Error     string `json:"error,omitempty"`
}

type JavaStatus struct {
	Path      string `json:"path"`
	OK        bool   `json:"ok"`
	Version   string `json:"version,omitempty"`
	Message   string `json:"message"`
	CheckedAt string `json:"checkedAt"`
}

type JavaInstallProgress struct {
	Stage    string `json:"stage"`
	Message  string `json:"message"`
	Current  int64  `json:"current"`
	Total    int64  `json:"total"`
	Percent  int    `json:"percent"`
	Done     bool   `json:"done"`
	Error    string `json:"error,omitempty"`
	JavaPath string `json:"javaPath,omitempty"`
	Version  string `json:"version,omitempty"`
}

type ProfileJavaRuntime struct {
	ProfileID     string `json:"profileId"`
	RequiredMajor int    `json:"requiredMajor"`
	Installed     bool   `json:"installed"`
	JavaPath      string `json:"javaPath,omitempty"`
	Version       string `json:"version,omitempty"`
	Message       string `json:"message"`
}

type ModFile struct {
	FileName    string `json:"fileName"`
	DisplayName string `json:"displayName"`
	Path        string `json:"path"`
	Enabled     bool   `json:"enabled"`
	Size        int64  `json:"size"`
	UpdatedAt   string `json:"updatedAt"`
	SHA1        string `json:"sha1,omitempty"`
}

type ModList struct {
	ProfileID string    `json:"profileId"`
	ModsDir   string    `json:"modsDir"`
	Mods      []ModFile `json:"mods"`
}

type GameLogFile struct {
	FileName    string `json:"fileName"`
	DisplayName string `json:"displayName"`
	Kind        string `json:"kind"`
	Size        int64  `json:"size"`
	UpdatedAt   string `json:"updatedAt"`
	Compressed  bool   `json:"compressed"`
}

type GameLogList struct {
	ProfileID string        `json:"profileId"`
	LogsDir   string        `json:"logsDir"`
	Files     []GameLogFile `json:"files"`
}

type GameLogContent struct {
	ProfileID   string `json:"profileId"`
	FileName    string `json:"fileName"`
	DisplayName string `json:"displayName"`
	Kind        string `json:"kind"`
	Content     string `json:"content"`
	Size        int64  `json:"size"`
	Truncated   bool   `json:"truncated"`
	MaxBytes    int64  `json:"maxBytes"`
}

type ModrinthProject struct {
	ProjectID      string   `json:"projectId"`
	Slug           string   `json:"slug"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Body           string   `json:"body,omitempty"`
	Author         string   `json:"author"`
	IconURL        string   `json:"iconUrl,omitempty"`
	Downloads      int      `json:"downloads"`
	Followers      int      `json:"followers,omitempty"`
	LatestVersion  string   `json:"latestVersion,omitempty"`
	ClientSide     string   `json:"clientSide,omitempty"`
	ServerSide     string   `json:"serverSide,omitempty"`
	LicenseName    string   `json:"licenseName,omitempty"`
	SourceURL      string   `json:"sourceUrl,omitempty"`
	IssuesURL      string   `json:"issuesUrl,omitempty"`
	WikiURL        string   `json:"wikiUrl,omitempty"`
	DiscordURL     string   `json:"discordUrl,omitempty"`
	Categories     []string `json:"categories,omitempty"`
	GameVersions   []string `json:"gameVersions,omitempty"`
	Loaders        []string `json:"loaders,omitempty"`
	DisplayVersion string   `json:"displayVersion,omitempty"`
}

type ModrinthSearchResult struct {
	ProfileID        string            `json:"profileId"`
	Query            string            `json:"query"`
	MinecraftVersion string            `json:"minecraftVersion"`
	Loader           string            `json:"loader"`
	TotalHits        int               `json:"totalHits"`
	Hits             []ModrinthProject `json:"hits"`
}

type ModrinthVersionFile struct {
	URL      string `json:"url"`
	FileName string `json:"fileName"`
	Size     int64  `json:"size"`
	Primary  bool   `json:"primary"`
	SHA1     string `json:"sha1,omitempty"`
}

type ModrinthVersion struct {
	ID            string               `json:"id"`
	ProjectID     string               `json:"projectId"`
	Name          string               `json:"name"`
	VersionNumber string               `json:"versionNumber"`
	VersionType   string               `json:"versionType"`
	DatePublished string               `json:"datePublished,omitempty"`
	Changelog     string               `json:"changelog,omitempty"`
	GameVersions  []string             `json:"gameVersions"`
	Loaders       []string             `json:"loaders"`
	File          ModrinthVersionFile  `json:"file"`
	Dependencies  []ModrinthDependency `json:"dependencies,omitempty"`
}

type ModrinthDependency struct {
	VersionID      string `json:"versionId,omitempty"`
	ProjectID      string `json:"projectId,omitempty"`
	FileName       string `json:"fileName,omitempty"`
	DependencyType string `json:"dependencyType"`
}

type ModrinthInstalledFile struct {
	ProjectID      string `json:"projectId"`
	VersionID      string `json:"versionId"`
	VersionName    string `json:"versionName"`
	VersionNumber  string `json:"versionNumber"`
	FileName       string `json:"fileName"`
	DisplayName    string `json:"displayName"`
	DependencyType string `json:"dependencyType,omitempty"`
	AlreadyPresent bool   `json:"alreadyPresent,omitempty"`
}

type ModrinthSkippedDependency struct {
	Dependency ModrinthDependency `json:"dependency"`
	Reason     string             `json:"reason"`
}

type ModrinthRequiredDependency struct {
	ProjectID      string `json:"projectId"`
	ProjectTitle   string `json:"projectTitle"`
	VersionID      string `json:"versionId"`
	VersionName    string `json:"versionName"`
	VersionNumber  string `json:"versionNumber"`
	FileName       string `json:"fileName"`
	DisplayName    string `json:"displayName"`
	AlreadyPresent bool   `json:"alreadyPresent,omitempty"`
}

type ModrinthInstallPlan struct {
	ProfileID            string                       `json:"profileId"`
	ProjectID            string                       `json:"projectId"`
	ProjectTitle         string                       `json:"projectTitle"`
	VersionID            string                       `json:"versionId"`
	VersionName          string                       `json:"versionName"`
	VersionNumber        string                       `json:"versionNumber"`
	FileName             string                       `json:"fileName"`
	RequiredDependencies []ModrinthRequiredDependency `json:"requiredDependencies,omitempty"`
	SkippedDependencies  []ModrinthSkippedDependency  `json:"skippedDependencies,omitempty"`
}

type ModrinthInstallResult struct {
	ProfileID           string                      `json:"profileId"`
	ProjectID           string                      `json:"projectId"`
	ProjectTitle        string                      `json:"projectTitle"`
	VersionID           string                      `json:"versionId"`
	VersionName         string                      `json:"versionName"`
	VersionNumber       string                      `json:"versionNumber"`
	FileName            string                      `json:"fileName"`
	ModList             ModList                     `json:"modList"`
	Dependencies        []ModrinthDependency        `json:"dependencies,omitempty"`
	InstalledFiles      []ModrinthInstalledFile     `json:"installedFiles,omitempty"`
	SkippedDependencies []ModrinthSkippedDependency `json:"skippedDependencies,omitempty"`
}

type ModrinthInstalledProject struct {
	ProfileID     string                  `json:"profileId"`
	ProjectID     string                  `json:"projectId"`
	ProjectTitle  string                  `json:"projectTitle"`
	VersionID     string                  `json:"versionId"`
	VersionName   string                  `json:"versionName"`
	VersionNumber string                  `json:"versionNumber"`
	FileName      string                  `json:"fileName"`
	Files         []ModrinthInstalledFile `json:"files,omitempty"`
	InstalledAt   string                  `json:"installedAt,omitempty"`
}

type ModrinthUpdatePlan struct {
	ProfileID            string                       `json:"profileId"`
	ProjectID            string                       `json:"projectId"`
	ProjectTitle         string                       `json:"projectTitle"`
	Tracked              bool                         `json:"tracked"`
	CurrentVersionID     string                       `json:"currentVersionId"`
	CurrentVersionName   string                       `json:"currentVersionName"`
	CurrentVersionNumber string                       `json:"currentVersionNumber"`
	CurrentFileName      string                       `json:"currentFileName"`
	LatestVersionID      string                       `json:"latestVersionId"`
	LatestVersionName    string                       `json:"latestVersionName"`
	LatestVersionNumber  string                       `json:"latestVersionNumber"`
	LatestFileName       string                       `json:"latestFileName"`
	UpdateAvailable      bool                         `json:"updateAvailable"`
	RequiredDependencies []ModrinthRequiredDependency `json:"requiredDependencies,omitempty"`
	SkippedDependencies  []ModrinthSkippedDependency  `json:"skippedDependencies,omitempty"`
	CheckError           string                       `json:"checkError,omitempty"`
}

type ModrinthUpdateResult struct {
	ProfileID           string                      `json:"profileId"`
	ProjectID           string                      `json:"projectId"`
	ProjectTitle        string                      `json:"projectTitle"`
	Updated             bool                        `json:"updated"`
	OldFileName         string                      `json:"oldFileName"`
	NewFileName         string                      `json:"newFileName"`
	ModList             ModList                     `json:"modList"`
	InstalledFiles      []ModrinthInstalledFile     `json:"installedFiles,omitempty"`
	DeletedFiles        []ModrinthDeleteFile        `json:"deletedFiles,omitempty"`
	SkippedFiles        []ModrinthDeleteFile        `json:"skippedFiles,omitempty"`
	SkippedDependencies []ModrinthSkippedDependency `json:"skippedDependencies,omitempty"`
}

type ModrinthDeleteFile struct {
	ProjectID      string `json:"projectId"`
	ProjectTitle   string `json:"projectTitle"`
	VersionID      string `json:"versionId"`
	VersionName    string `json:"versionName"`
	VersionNumber  string `json:"versionNumber"`
	FileName       string `json:"fileName"`
	DisplayName    string `json:"displayName"`
	DependencyType string `json:"dependencyType,omitempty"`
	Reason         string `json:"reason,omitempty"`
}

type ModrinthDeletePlan struct {
	ProfileID    string               `json:"profileId"`
	ProjectID    string               `json:"projectId"`
	ProjectTitle string               `json:"projectTitle"`
	Files        []ModrinthDeleteFile `json:"files,omitempty"`
	SkippedFiles []ModrinthDeleteFile `json:"skippedFiles,omitempty"`
	Tracked      bool                 `json:"tracked"`
}

type ModrinthDeleteResult struct {
	ProfileID    string               `json:"profileId"`
	ProjectID    string               `json:"projectId"`
	ProjectTitle string               `json:"projectTitle"`
	DeletedFiles []ModrinthDeleteFile `json:"deletedFiles,omitempty"`
	SkippedFiles []ModrinthDeleteFile `json:"skippedFiles,omitempty"`
	ModList      ModList              `json:"modList"`
}

type LaunchStatus string

const (
	LaunchStarting LaunchStatus = "starting"
	LaunchRunning  LaunchStatus = "running"
	LaunchStopped  LaunchStatus = "stopped"
	LaunchFailed   LaunchStatus = "failed"
)

type LaunchState struct {
	ProfileID string       `json:"profileId"`
	Status    LaunchStatus `json:"status"`
	Message   string       `json:"message"`
	ExitCode  int          `json:"exitCode,omitempty"`
	StartedAt string       `json:"startedAt,omitempty"`
	EndedAt   string       `json:"endedAt,omitempty"`
}

type LaunchEvent struct {
	ProfileID string       `json:"profileId"`
	Status    LaunchStatus `json:"status"`
	Stream    string       `json:"stream,omitempty"`
	Message   string       `json:"message"`
	ExitCode  int          `json:"exitCode,omitempty"`
	Time      string       `json:"time"`
}

type Profile struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	MinecraftVersion string         `json:"minecraftVersion"`
	Loader           LoaderConfig   `json:"loader"`
	Account          AccountConfig  `json:"account,omitempty"`
	GameDir          string         `json:"gameDir"`
	Memory           MemorySettings `json:"memory"`
	Install          InstallState   `json:"install"`
	CreatedAt        string         `json:"createdAt"`
	UpdatedAt        string         `json:"updatedAt"`
}

type ProfileInput struct {
	Name             string         `json:"name"`
	MinecraftVersion string         `json:"minecraftVersion"`
	Loader           LoaderConfig   `json:"loader"`
	Account          AccountConfig  `json:"account,omitempty"`
	GameDir          string         `json:"gameDir,omitempty"`
	Memory           MemorySettings `json:"memory"`
}

type ProfileList struct {
	SelectedProfileID string    `json:"selectedProfileId"`
	Profiles          []Profile `json:"profiles"`
}

type ModpackImportResult struct {
	Profile            Profile `json:"profile"`
	Name               string  `json:"name"`
	VersionID          string  `json:"versionId"`
	FilesInstalled     int     `json:"filesInstalled"`
	FilesSkipped       int     `json:"filesSkipped"`
	OverridesInstalled int     `json:"overridesInstalled"`
}

type ModpackExportResult struct {
	ProfileID         string `json:"profileId"`
	Name              string `json:"name"`
	VersionID         string `json:"versionId"`
	Path              string `json:"path"`
	FilesExported     int    `json:"filesExported"`
	OverridesExported int    `json:"overridesExported"`
}
