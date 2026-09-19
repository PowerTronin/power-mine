import {lazy, Suspense, useEffect, useMemo, useRef, useState, type FormEvent} from 'react';
import './App.css';
import {
    AppInfo,
    CreateLocalServer,
    CreateProfile,
    DeleteProfile,
    DeleteModrinthModFiles,
    DeleteProfileMod,
    ExportModrinthModpack,
    ExportProfileLogs,
    GetAccount,
    GetCachedVersionCatalog,
    GetModrinthProject,
    GetProfileJavaRuntime,
    GetSettings,
    ImportProfileMod,
    ImportModrinthModpack,
    InstallJava,
    InstallLocalServer,
    InstallModrinthModFiles,
    InstallModrinthModVersionFiles,
    InstallProfile,
    LaunchProfile,
    ListInstalledModrinthProjects,
    ListModrinthProjectVersions,
    ListModrinthUpdates,
    ListProfileGameLogs,
    ListProfileMods,
    ListProfiles,
    ListLocalServers,
    OpenDetachedLogsWindow,
    OpenLocalServerFolder,
    OpenLocalServerSettings,
    OpenLocalServerTerminal,
    OpenProfileModsFolder,
    OpenProfileLogsFolder,
    PlanModrinthInstall,
    PlanModrinthInstallVersion,
    PlanModrinthDelete,
    PlanModrinthUpdate,
    PlanModrinthUpdateVersion,
    PlanModrinthUpdateFile,
    ReadProfileGameLog,
    RefreshVersionCatalog,
    RepairLocalServer,
    RepairProfile,
    SaveAccount,
    SaveSettings,
    SearchModrinthMods,
    SelectProfile,
    SendLocalServerCommand,
    SetProfileModEnabled,
    StartLocalServer,
    StopLocalServer,
    SyncDetachedLogsWindow,
    UpdateModrinthModFile,
    UpdateModrinthModFiles,
    UpdateModrinthModVersionFiles,
    UpdateProfile,
    ValidateJava
} from "../wailsjs/go/main/App";
import {domain} from "../wailsjs/go/models";
import {EventsOn} from "../wailsjs/runtime/runtime";

type Screen = 'home' | 'library' | 'create' | 'account' | 'logs' | 'settings' | 'browse';

const MarkdownContent = lazy(() => import('./MarkdownContent'));

type SettingsDraft = {
    dataDir: string;
    javaPath: string;
    defaultMemory: {
        minMB: number;
        maxMB: number;
    };
    network: {
        retryCount: number;
        metadataTtlHours: number;
    };
};

type AccountDraft = {
    mode: string;
    offlineName: string;
    offlineUuid: string;
};

type ProfileSettingsDraft = {
    gameDir: string;
    minMB: number;
    maxMB: number;
};

type LocalServerForm = {
    name: string;
    minecraftVersion: string;
    serverDir: string;
    minMB: number;
    maxMB: number;
    port: number;
    eulaAccepted: boolean;
};

type InstallProgress = {
    profileId: string;
    stage: string;
    message: string;
    current: number;
    total: number;
    percent: number;
    done: boolean;
    error?: string;
};

type LocalServerProgress = {
    serverId: string;
    stage: string;
    message: string;
    current: number;
    total: number;
    percent: number;
    done: boolean;
    error?: string;
};

type JavaInstallProgress = {
    stage: string;
    message: string;
    current: number;
    total: number;
    percent: number;
    done: boolean;
    error?: string;
    javaPath?: string;
    version?: string;
};

type LauncherLog = {
    id: string;
    time: string;
    level: 'info' | 'error' | 'success';
    source: string;
    message: string;
    profileId?: string;
    serverId?: string;
};

type LaunchEvent = {
    profileId: string;
    status: 'starting' | 'running' | 'stopped' | 'failed';
    stream?: string;
    message: string;
    exitCode?: number;
    time: string;
};

type LaunchState = {
    profileId: string;
    status: 'starting' | 'running' | 'stopped' | 'failed';
    message: string;
    exitCode?: number;
    startedAt?: string;
    endedAt?: string;
};

type LocalServerEvent = {
    serverId: string;
    status: 'starting' | 'running' | 'stopped' | 'failed';
    stream?: string;
    message: string;
    exitCode?: number;
    time: string;
};

type LocalServerRunState = {
    serverId: string;
    status: 'starting' | 'running' | 'stopped' | 'failed';
    message: string;
    exitCode?: number;
    startedAt?: string;
    endedAt?: string;
};

type LogLevelFilter = 'all' | LauncherLog['level'];
type LogsMode = 'launcher' | 'game';
type InstallationFilter = 'all' | 'clients' | 'servers';
type LibrarySelectionType = 'profile' | 'server';

const navItems: Array<{ id: Screen; label: string; mark: string }> = [
    {id: 'home', label: 'Home', mark: 'H'},
    {id: 'library', label: 'Library', mark: 'L'},
    {id: 'create', label: 'Create', mark: '+'},
    {id: 'logs', label: 'Logs', mark: 'G'},
    {id: 'settings', label: 'Settings', mark: 'S'},
    {id: 'browse', label: 'Browse', mark: 'B'},
];

const defaultCreateForm = {
    name: 'New Vanilla Profile',
    minecraftVersion: '1.21.5',
    loaderType: 'vanilla',
    loaderVersion: 'latest',
    gameDir: '',
    minMB: 1024,
    maxMB: 4096,
};

const defaultLocalServerForm: LocalServerForm = {
    name: 'Local Vanilla Server',
    minecraftVersion: '1.21.5',
    serverDir: '',
    minMB: 1024,
    maxMB: 2048,
    port: 25565,
    eulaAccepted: false,
};

const defaultAccount: AccountDraft = {
    mode: 'offline',
    offlineName: 'Player',
    offlineUuid: '',
};

const MAX_LOCAL_SERVER_TERMINAL_EVENTS = 1000;

function App() {
    const [screen, setScreen] = useState<Screen>('home');
    const [info, setInfo] = useState<domain.AppInfo | null>(null);
    const [settings, setSettings] = useState<SettingsDraft | null>(null);
    const [account, setAccount] = useState<AccountDraft>(defaultAccount);
    const [profiles, setProfiles] = useState<domain.Profile[]>([]);
    const [localServers, setLocalServers] = useState<domain.LocalServer[]>([]);
    const [minecraftVersions, setMinecraftVersions] = useState<domain.VersionOption[]>([]);
    const [fabricLoaderVersions, setFabricLoaderVersions] = useState<domain.VersionOption[]>([]);
    const [quiltLoaderVersions, setQuiltLoaderVersions] = useState<domain.VersionOption[]>([]);
    const [forgeLoaderVersions, setForgeLoaderVersions] = useState<domain.VersionOption[]>([]);
    const [neoForgeLoaderVersions, setNeoForgeLoaderVersions] = useState<domain.VersionOption[]>([]);
    const [installProgress, setInstallProgress] = useState<Record<string, InstallProgress>>({});
    const [javaInstallProgress, setJavaInstallProgress] = useState<JavaInstallProgress | null>(null);
    const [javaStatus, setJavaStatus] = useState<domain.JavaStatus | null>(null);
    const [profileJavaRuntimes, setProfileJavaRuntimes] = useState<Record<string, domain.ProfileJavaRuntime>>({});
    const [profileModLists, setProfileModLists] = useState<Record<string, domain.ModList>>({});
    const [profileGameLogLists, setProfileGameLogLists] = useState<Record<string, domain.GameLogList>>({});
    const [profileGameLogContents, setProfileGameLogContents] = useState<Record<string, domain.GameLogContent>>({});
    const [launchStates, setLaunchStates] = useState<Record<string, LaunchState>>({});
    const [localServerProgress, setLocalServerProgress] = useState<Record<string, LocalServerProgress>>({});
    const [localServerRunStates, setLocalServerRunStates] = useState<Record<string, LocalServerRunState>>({});
    const [launcherLogs, setLauncherLogs] = useState<LauncherLog[]>([]);
    const [selectedProfileId, setSelectedProfileId] = useState('');
    const [selectedLocalServerId, setSelectedLocalServerId] = useState('');
    const [librarySelectionType, setLibrarySelectionType] = useState<LibrarySelectionType>('profile');
    const [profileSettingsId, setProfileSettingsId] = useState('');
    const [localServerSettingsId, setLocalServerSettingsId] = useState('');
    const [profileSettingsDraft, setProfileSettingsDraft] = useState<ProfileSettingsDraft | null>(null);
    const [modActionKey, setModActionKey] = useState('');
    const [gameLogActionKey, setGameLogActionKey] = useState('');
    const [logsWindowOpen, setLogsWindowOpen] = useState(false);
    const [nativeLogsWindowActive, setNativeLogsWindowActive] = useState(false);
    const [terminalServerId, setTerminalServerId] = useState('');
    const [serverTerminalActionKey, setServerTerminalActionKey] = useState('');
    const [localServerTerminalEvents, setLocalServerTerminalEvents] = useState<Record<string, LocalServerEvent[]>>({});
    const [browseProfileId, setBrowseProfileId] = useState('');
    const [browseQuery, setBrowseQuery] = useState('');
    const [browseResults, setBrowseResults] = useState<domain.ModrinthSearchResult | null>(null);
    const [browseLoading, setBrowseLoading] = useState(false);
    const [browseInstallKey, setBrowseInstallKey] = useState('');
    const [browseDeleteKey, setBrowseDeleteKey] = useState('');
    const [browseUpdateKey, setBrowseUpdateKey] = useState('');
    const [browseDetailsProject, setBrowseDetailsProject] = useState<domain.ModrinthProject | null>(null);
    const [browseDetailsLoading, setBrowseDetailsLoading] = useState(false);
    const [browseProjectVersions, setBrowseProjectVersions] = useState<Record<string, domain.ModrinthVersion[]>>({});
    const [browseVersionsLoadingKey, setBrowseVersionsLoadingKey] = useState('');
    const [installedModrinthProjects, setInstalledModrinthProjects] = useState<Record<string, boolean>>({});
    const [modrinthUpdatePlans, setModrinthUpdatePlans] = useState<Record<string, domain.ModrinthUpdatePlan[]>>({});
    const [pendingModrinthInstall, setPendingModrinthInstall] = useState<domain.ModrinthInstallPlan | null>(null);
    const [pendingModrinthDelete, setPendingModrinthDelete] = useState<domain.ModrinthDeletePlan | null>(null);
    const [pendingModrinthUpdate, setPendingModrinthUpdate] = useState<domain.ModrinthUpdatePlan | null>(null);
    const [message, setMessage] = useState('');
    const [error, setError] = useState('');
    const [versionCatalogStatus, setVersionCatalogStatus] = useState('Catalog not loaded');
    const [versionCatalogWarning, setVersionCatalogWarning] = useState('');
    const [modpackImporting, setModpackImporting] = useState(false);
    const [appRefreshing, setAppRefreshing] = useState(false);
    const [createForm, setCreateForm] = useState(defaultCreateForm);
    const [localServerForm, setLocalServerForm] = useState<LocalServerForm>(defaultLocalServerForm);
    const [localServerActionKey, setLocalServerActionKey] = useState('');

    const selectedProfile = useMemo(
        () => profiles.find((profile) => profile.id === selectedProfileId) ?? profiles[0],
        [profiles, selectedProfileId]
    );
    const browseProfile = useMemo(
        () => profiles.find((profile) => profile.id === browseProfileId) ?? selectedProfile,
        [profiles, browseProfileId, selectedProfile]
    );
    const selectedLocalServer = useMemo(
        () => localServers.find((server) => server.id === selectedLocalServerId) ?? localServers[0],
        [localServers, selectedLocalServerId]
    );
    const settingsProfile = useMemo(
        () => profiles.find((profile) => profile.id === profileSettingsId),
        [profiles, profileSettingsId]
    );
    const settingsLocalServer = useMemo(
        () => localServers.find((server) => server.id === localServerSettingsId),
        [localServers, localServerSettingsId]
    );
    const terminalLocalServer = useMemo(
        () => localServers.find((server) => server.id === terminalServerId),
        [localServers, terminalServerId]
    );
    const settingsProfileModList = settingsProfile ? profileModLists[settingsProfile.id] : undefined;
    const settingsProfileUpdatePlans = settingsProfile ? modrinthUpdatePlans[settingsProfile.id] ?? [] : [];
    const settingsProfileJavaRuntime = settingsProfile ? profileJavaRuntimes[settingsProfile.id] : undefined;
    const libraryDetailType: LibrarySelectionType = (() => {
        if (librarySelectionType === 'server' && selectedLocalServer) {
            return 'server';
        }
        if (librarySelectionType === 'profile' && selectedProfile) {
            return 'profile';
        }
        if (selectedProfile) {
            return 'profile';
        }
        if (selectedLocalServer) {
            return 'server';
        }
        return librarySelectionType;
    })();
    const librarySelectedProfile = libraryDetailType === 'profile' ? selectedProfile : undefined;
    const librarySelectedLocalServer = libraryDetailType === 'server' ? selectedLocalServer : undefined;
    const libraryProfileProgress = librarySelectedProfile ? installProgress[librarySelectedProfile.id] : undefined;
    const libraryProfileLaunch = librarySelectedProfile ? launchStates[librarySelectedProfile.id] : undefined;
    const libraryProfileJavaRuntime = librarySelectedProfile ? profileJavaRuntimes[librarySelectedProfile.id] : undefined;
    const libraryProfileModList = librarySelectedProfile ? profileModLists[librarySelectedProfile.id] : undefined;
    const libraryProfileUpdatePlans = librarySelectedProfile ? modrinthUpdatePlans[librarySelectedProfile.id] ?? [] : [];
    const libraryLocalServerProgress = librarySelectedLocalServer ? localServerProgress[librarySelectedLocalServer.id] : undefined;
    const libraryLocalServerRun = librarySelectedLocalServer ? localServerRunStates[librarySelectedLocalServer.id] : undefined;
    const libraryTotalCount = profiles.length + localServers.length;
    const selectedLogs = useMemo(
        () => launcherLogs.filter((log) => {
            if (log.profileId) {
                return !selectedProfile || log.profileId === selectedProfile.id;
            }
            if (log.serverId) {
                return !selectedLocalServer || log.serverId === selectedLocalServer.id;
            }
            return true;
        }),
        [launcherLogs, selectedProfile, selectedLocalServer]
    );
    const createImportProgress = useMemo(
        () => activeCreateImportProgress(installProgress, modpackImporting),
        [installProgress, modpackImporting]
    );
    const browseProfileForModrinth = browseProfile?.id;

    useEffect(() => {
        refreshApp();
        refreshVersionOptions();
        validateJava();
    }, []);

    useEffect(() => {
        if (profiles.length === 0) {
            setBrowseProfileId('');
            return;
        }
        const hasBrowseProfile = profiles.some((profile) => profile.id === browseProfileId);
        if (!hasBrowseProfile) {
            const selectedExists = profiles.some((profile) => profile.id === selectedProfileId);
            setBrowseProfileId(selectedExists ? selectedProfileId : profiles[0].id);
        }
    }, [profiles, selectedProfileId, browseProfileId]);

    useEffect(() => {
        if (localServers.length === 0) {
            setSelectedLocalServerId('');
            return;
        }
        if (!localServers.some((server) => server.id === selectedLocalServerId)) {
            setSelectedLocalServerId(localServers[0].id);
        }
    }, [localServers, selectedLocalServerId]);

    useEffect(() => {
        if (browseProfileForModrinth) {
            refreshInstalledModrinthProjects(browseProfileForModrinth);
            refreshModrinthUpdates(browseProfileForModrinth);
        }
    }, [browseProfileForModrinth]);

    useEffect(() => {
        if (selectedProfile?.id) {
            refreshProfileJavaRuntime(selectedProfile.id);
            refreshProfileMods(selectedProfile.id);
            refreshModrinthUpdates(selectedProfile.id);
        }
    }, [selectedProfile?.id, selectedProfile?.minecraftVersion, selectedProfile?.install?.status]);

    useEffect(() => {
        return EventsOn('install:progress', (event: InstallProgress) => {
            if (!event?.profileId) {
                return;
            }
            setInstallProgress((current) => ({
                ...current,
                [event.profileId]: event,
            }));
            if (event.error) {
                setError(event.error);
            }
            appendLog({
                level: event.error ? 'error' : event.done ? 'success' : 'info',
                source: 'Install',
                message: progressMessage(event),
                profileId: event.profileId,
            });
        });
    }, []);

    useEffect(() => {
        return EventsOn('java:progress', (event: JavaInstallProgress) => {
            if (!event) {
                return;
            }
            setJavaInstallProgress(event);
            if (event.error) {
                setError(event.error);
            }
            appendLog({
                level: event.error ? 'error' : event.done ? 'success' : 'info',
                source: 'Java Installer',
                message: javaProgressMessage(event),
            });
        });
    }, []);

    useEffect(() => {
        return EventsOn('launch:event', (event: LaunchEvent) => {
            if (!event?.profileId) {
                return;
            }
            setLaunchStates((current) => ({
                ...current,
                [event.profileId]: {
                    profileId: event.profileId,
                    status: event.status,
                    message: event.message,
                    exitCode: event.exitCode,
                    endedAt: event.status === 'stopped' || event.status === 'failed' ? event.time : current[event.profileId]?.endedAt,
                    startedAt: current[event.profileId]?.startedAt ?? event.time,
                },
            }));
            appendLog({
                level: event.status === 'failed' ? 'error' : event.status === 'stopped' ? 'success' : 'info',
                source: event.stream ? `Game ${event.stream}` : 'Launch',
                message: event.exitCode !== undefined ? `${event.message} (exit ${event.exitCode})` : event.message,
                profileId: event.profileId,
            });
        });
    }, []);

    useEffect(() => {
        return EventsOn('server:progress', (event: LocalServerProgress) => {
            if (!event?.serverId) {
                return;
            }
            setLocalServerProgress((current) => ({
                ...current,
                [event.serverId]: event,
            }));
            if (event.error) {
                setError(event.error);
            }
            appendLog({
                level: event.error ? 'error' : event.done ? 'success' : 'info',
                source: 'Server install',
                message: localServerProgressMessage(event),
                serverId: event.serverId,
            });
        });
    }, []);

    useEffect(() => {
        return EventsOn('server:event', (event: LocalServerEvent) => {
            if (!event?.serverId) {
                return;
            }
            setLocalServerTerminalEvents((current) => {
                const events = [...(current[event.serverId] ?? []), event].slice(-MAX_LOCAL_SERVER_TERMINAL_EVENTS);
                return {
                    ...current,
                    [event.serverId]: events,
                };
            });
            if (!event.stream) {
                setLocalServerRunStates((current) => ({
                    ...current,
                    [event.serverId]: {
                        serverId: event.serverId,
                        status: event.status,
                        message: event.message,
                        exitCode: event.exitCode,
                        endedAt: event.status === 'stopped' || event.status === 'failed' ? event.time : current[event.serverId]?.endedAt,
                        startedAt: current[event.serverId]?.startedAt ?? event.time,
                    },
                }));
            }
            appendLog({
                level: event.status === 'failed' ? 'error' : event.status === 'stopped' ? 'success' : 'info',
                source: event.stream ? `Server ${event.stream}` : 'Server',
                message: event.exitCode !== undefined ? `${event.message} (exit ${event.exitCode})` : event.message,
                serverId: event.serverId,
            });
        });
    }, []);

    useEffect(() => {
        if (!terminalServerId) {
            return;
        }
        if (!localServers.some((server) => server.id === terminalServerId)) {
            setTerminalServerId('');
        }
    }, [terminalServerId, localServers]);

    useEffect(() => {
        if (!nativeLogsWindowActive) {
            return;
        }
        SyncDetachedLogsWindow(detachedLogsSnapshot(launcherLogs, profiles, localServers)).catch((err) => {
            setError(errorText(err));
        });
    }, [nativeLogsWindowActive, launcherLogs, profiles, localServers]);

    function appendLog(entry: Omit<LauncherLog, 'id' | 'time'>) {
        const next: LauncherLog = {
            ...entry,
            id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
            time: new Date().toLocaleTimeString(),
        };
        setLauncherLogs((current) => [next, ...current]);
    }

    async function openDetachedLogsWindow() {
        setError('');
        try {
            await OpenDetachedLogsWindow(detachedLogsSnapshot(launcherLogs, profiles, localServers));
            setNativeLogsWindowActive(true);
            setLogsWindowOpen(false);
            appendLog({
                level: 'success',
                source: 'Logs',
                message: 'Native logs window opened.',
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            setLogsWindowOpen(true);
            appendLog({
                level: 'error',
                source: 'Logs',
                message: `Native logs window failed: ${text}. Using in-app logs window.`,
            });
        }
    }

    async function refreshApp() {
        try {
            setError('');
            const [nextInfo, nextSettings, nextAccount, profileList, serverList] = await Promise.all([
                AppInfo(),
                GetSettings(),
                GetAccount(),
                ListProfiles(),
                ListLocalServers()
            ]);
            setInfo(nextInfo);
            setSettings(settingsDraft(nextSettings));
            setAccount(accountDraft(nextAccount));
            setProfiles(profileList.profiles ?? []);
            setLocalServers(serverList.servers ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            setBrowseProfileId((current) => current || profileList.selectedProfileId || profileList.profiles?.[0]?.id || '');
            setSelectedLocalServerId((current) => current || serverList.servers?.[0]?.id || '');
            setCreateForm((current) => ({
                ...current,
                minMB: nextSettings.defaultMemory.minMB,
                maxMB: nextSettings.defaultMemory.maxMB,
            }));
            setLocalServerForm((current) => ({
                ...current,
                minMB: nextSettings.defaultMemory.minMB,
                maxMB: nextSettings.defaultMemory.maxMB,
            }));
            appendLog({
                level: 'info',
                source: 'App',
                message: 'Loaded local settings and profiles.',
            });
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'App',
                message: `Failed to load app state: ${errorText(err)}`,
            });
        }
    }

    async function refreshVisibleState() {
        const profileId = selectedProfile?.id ?? selectedProfileId;
        try {
            setAppRefreshing(true);
            setError('');
            setMessage('');
            await refreshApp();
            await validateJava();
            if (profileId) {
                await Promise.all([
                    refreshProfileJavaRuntime(profileId),
                    refreshProfileMods(profileId),
                    refreshInstalledModrinthProjects(profileId),
                    refreshModrinthUpdates(profileId),
                ]);
                if (screen === 'logs') {
                    await refreshProfileGameLogs(profileId);
                }
            }
            if (screen === 'create') {
                await refreshVersionOptions();
            }
            setMessage('Launcher state refreshed.');
            appendLog({
                level: 'success',
                source: 'App',
                message: 'Manual refresh complete.',
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'App',
                message: `Manual refresh failed: ${text}`,
                profileId,
            });
        } finally {
            setAppRefreshing(false);
        }
    }

    async function refreshVersionOptions() {
        try {
            setVersionCatalogStatus('Loading cached catalog');
            const cached = await GetCachedVersionCatalog();
            applyVersionCatalog(cached);
            setVersionCatalogStatus(versionCatalogStatusText(cached));
            setVersionCatalogWarning((cached.warnings ?? []).join(' '));
            appendLog({
                level: 'info',
                source: 'Catalog',
                message: `Loaded cached catalog: ${versionCatalogSummary(cached)}.`,
            });
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Catalog',
                message: `Catalog cache load failed: ${errorText(err)}`,
            });
        }
        void refreshVersionCatalogFromNetwork();
    }

    async function refreshVersionCatalogFromNetwork() {
        try {
            setVersionCatalogStatus('Updating catalog');
            const catalog = await RefreshVersionCatalog();
            applyVersionCatalog(catalog);
            setVersionCatalogStatus(versionCatalogStatusText(catalog));
            setVersionCatalogWarning((catalog.warnings ?? []).join(' '));
            appendLog({
                level: (catalog.warnings?.length ?? 0) === 0 ? 'success' : 'info',
                source: 'Catalog',
                message: `Version catalog ready: ${versionCatalogSummary(catalog)}.`,
            });
            for (const warning of catalog.warnings ?? []) {
                appendLog({
                    level: 'error',
                    source: 'Catalog',
                    message: warning,
                });
            }
        } catch (err) {
            const text = `Version catalog refresh failed: ${errorText(err)}`;
            setVersionCatalogStatus('Catalog update failed');
            setVersionCatalogWarning(text);
            appendLog({
                level: 'error',
                source: 'Catalog',
                message: text,
            });
        }
    }

    function applyVersionCatalog(catalog: domain.VersionCatalog) {
        const nextMinecraftVersions = catalog.minecraftVersions ?? [];
        const nextFabricLoaderVersions = catalog.fabricLoaderVersions?.length
            ? catalog.fabricLoaderVersions
            : fallbackFabricLoaderVersions();
        const nextQuiltLoaderVersions = catalog.quiltLoaderVersions?.length
            ? catalog.quiltLoaderVersions
            : fallbackQuiltLoaderVersions();
        const nextForgeLoaderVersions = catalog.forgeLoaderVersions?.length
            ? catalog.forgeLoaderVersions
            : fallbackForgeLoaderVersions();
        const nextNeoForgeLoaderVersions = catalog.neoForgeLoaderVersions?.length
            ? catalog.neoForgeLoaderVersions
            : fallbackNeoForgeLoaderVersions();
        if (nextMinecraftVersions.length > 0) {
            setMinecraftVersions(nextMinecraftVersions);
        }
        setFabricLoaderVersions(nextFabricLoaderVersions);
        setQuiltLoaderVersions(nextQuiltLoaderVersions);
        setForgeLoaderVersions(nextForgeLoaderVersions);
        setNeoForgeLoaderVersions(nextNeoForgeLoaderVersions);
        setCreateForm((current) => ({
            ...current,
            ...nextCreateVersionSelection(current, nextMinecraftVersions, nextFabricLoaderVersions, nextQuiltLoaderVersions, nextForgeLoaderVersions, nextNeoForgeLoaderVersions),
        }));
        setLocalServerForm((current) => ({
            ...current,
            minecraftVersion: pickCurrentValue(current.minecraftVersion, nextMinecraftVersions),
        }));
    }

    async function validateJava() {
        try {
            const status = await ValidateJava();
            setJavaStatus(status);
            appendLog({
                level: status.ok ? 'success' : 'error',
                source: 'Java',
                message: status.message,
            });
        } catch (err) {
            setJavaStatus(null);
            appendLog({
                level: 'error',
                source: 'Java',
                message: `Java validation failed: ${errorText(err)}`,
            });
        }
    }

    async function refreshProfileJavaRuntime(profileId: string) {
        try {
            const runtime = await GetProfileJavaRuntime(profileId);
            setProfileJavaRuntimes((current) => ({
                ...current,
                [profileId]: runtime,
            }));
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Java',
                message: `Profile Java runtime check failed: ${errorText(err)}`,
                profileId,
            });
        }
    }

    async function refreshProfileMods(profileId: string) {
        try {
            const list = await ListProfileMods(profileId);
            setProfileModLists((current) => ({
                ...current,
                [profileId]: list,
            }));
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Profile mods refresh failed: ${errorText(err)}`,
                profileId,
            });
        }
    }

    async function reloadProfileMods(profileId: string) {
        const action = `${profileId}:refresh`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            const list = await ListProfileMods(profileId);
            setProfileModLists((current) => ({...current, [profileId]: list}));
            setMessage('Mods refreshed.');
            appendLog({
                level: 'success',
                source: 'Mods',
                message: `Loaded ${list.mods?.length ?? 0} local mods.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Mods refresh failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function installJava(version = 21) {
        try {
            setError('');
            setMessage('');
            setJavaInstallProgress({
                stage: 'start',
                message: `Installing Java ${version}`,
                current: 0,
                total: 0,
                percent: 0,
                done: false,
                version: version.toString(),
            });
            appendLog({
                level: 'info',
                source: 'Java Installer',
                message: `Java ${version} install requested.`,
            });
            const status = await InstallJava(version);
            setMessage(status.message);
            appendLog({
                level: status.ok ? 'success' : 'error',
                source: 'Java',
                message: status.message,
            });
            if (selectedProfile?.id) {
                await refreshProfileJavaRuntime(selectedProfile.id);
            }
        } catch (err) {
            const text = errorText(err);
            setError(text);
            setJavaInstallProgress((current) => ({
                stage: current?.stage ?? 'failed',
                message: 'Java install failed',
                current: current?.current ?? 0,
                total: current?.total ?? 0,
                percent: current?.percent ?? 0,
                done: true,
                error: text,
                version: current?.version ?? version.toString(),
            }));
            appendLog({
                level: 'error',
                source: 'Java Installer',
                message: `Java install failed: ${text}`,
            });
        }
    }

    async function createProfile(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        try {
            setError('');
            setMessage('');
            const loaderType = createForm.loaderType === 'fabric' || createForm.loaderType === 'quilt' || createForm.loaderType === 'forge' || createForm.loaderType === 'neoforge' ? createForm.loaderType : 'vanilla';
            const input = new domain.ProfileInput({
                name: createForm.name,
                minecraftVersion: createForm.minecraftVersion,
                loader: {
                    type: loaderType,
                    version: loaderType !== 'vanilla' ? createForm.loaderVersion : '',
                },
                gameDir: createForm.gameDir,
                memory: {
                    minMB: Number(createForm.minMB),
                    maxMB: Number(createForm.maxMB),
                },
            });
            const profile = await CreateProfile(input);
            const profileList = await SelectProfile(profile.id);
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            setMessage(`Created profile ${profile.name}.`);
            appendLog({
                level: 'success',
                source: 'Profile',
                message: `Created ${profile.name}. Account is ${accountLabel(account)}.`,
                profileId: profile.id,
            });
            setLibrarySelectionType('profile');
            navigateToScreen('library');
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Profile',
                message: `Create profile failed: ${errorText(err)}`,
            });
        }
    }

    async function refreshLocalServers() {
        try {
            const list = await ListLocalServers();
            setLocalServers(list.servers ?? []);
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Server',
                message: `Local server refresh failed: ${errorText(err)}`,
            });
        }
    }

    async function createLocalServer(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!localServerForm.eulaAccepted) {
            setError('Accept the Minecraft EULA before creating a local server.');
            return;
        }

        try {
            setError('');
            setMessage('');
            setLocalServerActionKey('server:create');
            const server = await CreateLocalServer(new domain.LocalServerInput({
                name: localServerForm.name,
                minecraftVersion: localServerForm.minecraftVersion,
                serverDir: localServerForm.serverDir,
                memory: {
                    minMB: Number(localServerForm.minMB),
                    maxMB: Number(localServerForm.maxMB),
                },
                port: Number(localServerForm.port),
                eulaAccepted: localServerForm.eulaAccepted,
            }));
            setLocalServers((current) => [...current, server]);
            setSelectedLocalServerId(server.id);
            setLibrarySelectionType('server');
            setLocalServerForm((current) => ({
                ...current,
                name: defaultLocalServerForm.name,
                serverDir: '',
                eulaAccepted: false,
            }));
            appendLog({
                level: 'success',
                source: 'Server',
                message: `Created ${server.name}.`,
                serverId: server.id,
            });
            await installLocalServer(server.id);
            navigateToScreen('library');
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Server',
                message: `Create local server failed: ${text}`,
            });
            await refreshLocalServers();
        } finally {
            setLocalServerActionKey('');
        }
    }

    async function installLocalServer(id: string, repair = false) {
        const action = `${id}:${repair ? 'repair' : 'install'}`;
        const status = repair ? 'repairing' : 'installing';
        const progressMessage = repair ? 'Repairing local server' : 'Installing local server';
        const successMessage = repair ? 'Local server repaired' : 'Local server ready';
        try {
            setError('');
            setMessage('');
            setLocalServerActionKey(action);
            setLocalServers((current) => current.map((server) => server.id === id
                ? domain.LocalServer.createFrom({
                    ...server,
                    install: {
                        ...server.install,
                        status,
                        message: progressMessage,
                    },
                })
                : server
            ));
            appendLog({
                level: 'info',
                source: repair ? 'Server repair' : 'Server install',
                message: repair ? 'Local server repair requested.' : 'Local server install requested.',
                serverId: id,
            });
            const server = repair ? await RepairLocalServer(id) : await InstallLocalServer(id);
            setLocalServers((current) => replaceLocalServer(current, server));
            setLocalServerProgress((current) => ({
                ...current,
                [id]: {
                    serverId: id,
                    stage: 'complete',
                    message: server.install?.message || successMessage,
                    current: 1,
                    total: 1,
                    percent: 100,
                    done: true,
                },
            }));
            setSelectedLocalServerId(server.id);
            setLibrarySelectionType('server');
            setMessage(server.install?.message || `${successMessage}.`);
            appendLog({
                level: server.install?.status === 'failed' ? 'error' : 'success',
                source: repair ? 'Server repair' : 'Server install',
                message: server.install?.message || `${successMessage}.`,
                serverId: id,
            });
            return server;
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: repair ? 'Server repair' : 'Server install',
                message: `${repair ? 'Local server repair' : 'Local server install'} failed: ${text}`,
                serverId: id,
            });
            await refreshLocalServers();
            return undefined;
        } finally {
            setLocalServerActionKey((current) => current === action ? '' : current);
        }
    }

    async function repairLocalServer(id: string) {
        return installLocalServer(id, true);
    }

    async function startLocalServer(id: string) {
        try {
            setError('');
            setMessage('');
            setLocalServerActionKey(`${id}:start`);
            setLocalServerRunStates((current) => ({
                ...current,
                [id]: {
                    serverId: id,
                    status: 'starting',
                    message: 'Starting local server',
                    startedAt: new Date().toISOString(),
                },
            }));
            const state = await StartLocalServer(id);
            setLocalServerRunStates((current) => ({
                ...current,
                [id]: localServerRunStateFromDomain(state),
            }));
            appendLog({
                level: 'info',
                source: 'Server',
                message: state.message,
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            setLocalServerRunStates((current) => ({
                ...current,
                [id]: {
                    serverId: id,
                    status: 'failed',
                    message: text,
                    endedAt: new Date().toISOString(),
                },
            }));
            appendLog({
                level: 'error',
                source: 'Server',
                message: `Start local server failed: ${text}`,
                serverId: id,
            });
        } finally {
            setLocalServerActionKey('');
        }
    }

    async function stopLocalServer(id: string) {
        try {
            setError('');
            setMessage('');
            setLocalServerActionKey(`${id}:stop`);
            const state = await StopLocalServer(id);
            setLocalServerRunStates((current) => ({
                ...current,
                [id]: localServerRunStateFromDomain(state),
            }));
            appendLog({
                level: 'info',
                source: 'Server',
                message: state.message,
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Server',
                message: `Stop local server failed: ${text}`,
                serverId: id,
            });
        } finally {
            setLocalServerActionKey('');
        }
    }

    async function openLocalServerFolder(id: string) {
        try {
            setError('');
            setLocalServerActionKey(`${id}:open`);
            await OpenLocalServerFolder(id);
            appendLog({
                level: 'info',
                source: 'Server',
                message: 'Local server folder opened.',
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Server',
                message: `Open local server folder failed: ${text}`,
                serverId: id,
            });
        } finally {
            setLocalServerActionKey('');
        }
    }

    async function openLocalServerSettingsFile(id: string) {
        try {
            setError('');
            setLocalServerActionKey(`${id}:settings`);
            await OpenLocalServerSettings(id);
            appendLog({
                level: 'info',
                source: 'Server settings',
                message: 'Local server raw settings opened.',
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Server settings',
                message: `Open local server settings failed: ${text}`,
                serverId: id,
            });
        } finally {
            setLocalServerActionKey('');
        }
    }

    function openLocalServerSettings(server: domain.LocalServer) {
        setSelectedLocalServerId(server.id);
        setLocalServerSettingsId(server.id);
    }

    function closeLocalServerSettings() {
        setLocalServerSettingsId('');
    }

    async function openLocalServerTerminal(id: string) {
        setError('');
        setSelectedLocalServerId(id);
        try {
            await OpenLocalServerTerminal(id);
            setTerminalServerId((current) => current === id ? '' : current);
            appendLog({
                level: 'success',
                source: 'Server terminal',
                message: 'Native local server terminal opened.',
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            setTerminalServerId(id);
            appendLog({
                level: 'error',
                source: 'Server terminal',
                message: `Native local server terminal failed: ${text}. Using in-app terminal.`,
                serverId: id,
            });
        }
    }

    async function sendLocalServerTerminalCommand(id: string, command: string) {
        const action = `${id}:terminal-command`;
        try {
            setError('');
            setServerTerminalActionKey(action);
            await SendLocalServerCommand(id, command);
            appendLog({
                level: 'success',
                source: 'Server terminal',
                message: `Command sent: ${command}`,
                serverId: id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Server terminal',
                message: `Send local server command failed: ${text}`,
                serverId: id,
            });
            throw err;
        } finally {
            setServerTerminalActionKey((current) => current === action ? '' : current);
        }
    }

    async function importModrinthModpack() {
        try {
            setError('');
            setMessage('');
            setModpackImporting(true);
            appendLog({
                level: 'info',
                source: 'Modpack',
                message: 'Modrinth modpack import requested.',
            });
            const result = await ImportModrinthModpack();
            if (!result.profile?.id) {
                appendLog({
                    level: 'info',
                    source: 'Modpack',
                    message: 'Modrinth modpack import cancelled.',
                });
                return;
            }

            const profileList = await SelectProfile(result.profile.id);
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId || result.profile.id);
            await refreshProfileJavaRuntime(result.profile.id);
            await reloadProfileMods(result.profile.id);
            setMessage(modpackImportMessage(result));
            appendLog({
                level: 'success',
                source: 'Modpack',
                message: modpackImportMessage(result),
                profileId: result.profile.id,
            });
            setLibrarySelectionType('profile');
            navigateToScreen('library');
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modpack',
                message: `Modpack import failed: ${text}`,
            });
            await refreshApp();
        } finally {
            setModpackImporting(false);
        }
    }

    async function selectProfile(id: string) {
        try {
            setError('');
            setLibrarySelectionType('profile');
            const profileList = await SelectProfile(id);
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            closeProfileSettings();
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Profile',
                message: `Select profile failed: ${errorText(err)}`,
                profileId: id,
            });
        }
    }

    function selectLocalServer(id: string) {
        setSelectedLocalServerId(id);
        setLibrarySelectionType('server');
    }

    async function deleteProfile(id: string) {
        try {
            setError('');
            setMessage('');
            await DeleteProfile(id);
            const profileList = await ListProfiles();
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            if (profileSettingsId === id) {
                closeProfileSettings();
            }
            setMessage('Profile deleted.');
            appendLog({
                level: 'info',
                source: 'Profile',
                message: 'Profile deleted.',
                profileId: id,
            });
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Profile',
                message: `Delete profile failed: ${errorText(err)}`,
                profileId: id,
            });
        }
    }

    function openProfileSettings(profile: domain.Profile) {
        setProfileSettingsId(profile.id);
        setProfileSettingsDraft(profileSettingsDraftFrom(profile));
        refreshProfileMods(profile.id);
    }

    function closeProfileSettings() {
        setProfileSettingsId('');
        setProfileSettingsDraft(null);
    }

    async function saveProfileSettings(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const profile = profiles.find((item) => item.id === profileSettingsId);
        if (!profile || !profileSettingsDraft) {
            return;
        }
        try {
            setError('');
            setMessage('');
            const saved = await UpdateProfile(profile.id, new domain.ProfileInput({
                name: profile.name,
                minecraftVersion: profile.minecraftVersion,
                loader: {
                    type: profile.loader.type,
                    version: profile.loader.version ?? '',
                },
                gameDir: profileSettingsDraft.gameDir,
                memory: {
                    minMB: Number(profileSettingsDraft.minMB),
                    maxMB: Number(profileSettingsDraft.maxMB),
                },
            }));
            const profileList = await SelectProfile(saved.id);
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            setProfileSettingsDraft(profileSettingsDraftFrom(saved));
            setMessage('Profile settings saved.');
            appendLog({
                level: 'success',
                source: 'Profile',
                message: `Saved settings for ${saved.name}.`,
                profileId: saved.id,
            });
            await refreshProfileJavaRuntime(saved.id);
            await refreshProfileMods(saved.id);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Profile',
                message: `Save profile settings failed: ${text}`,
                profileId: profile.id,
            });
        }
    }

    async function importProfileMod(profileId: string) {
        const action = `${profileId}:import`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            const list = await ImportProfileMod(profileId);
            setProfileModLists((current) => ({...current, [profileId]: list}));
            setMessage('Mods refreshed.');
            appendLog({
                level: 'success',
                source: 'Mods',
                message: 'Mod import completed.',
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Mod import failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function exportProfileModpack(profileId: string) {
        const action = `${profileId}:export-mrpack`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            appendLog({
                level: 'info',
                source: 'Modpack',
                message: 'Modrinth modpack export requested.',
                profileId,
            });
            const result = await ExportModrinthModpack(profileId);
            if (!result.path) {
                appendLog({
                    level: 'info',
                    source: 'Modpack',
                    message: 'Modrinth modpack export cancelled.',
                    profileId,
                });
                return;
            }
            const text = modpackExportMessage(result);
            setMessage(text);
            appendLog({
                level: 'success',
                source: 'Modpack',
                message: text,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modpack',
                message: `Modpack export failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function toggleProfileMod(profileId: string, fileName: string, enabled: boolean) {
        const action = `${profileId}:toggle:${fileName}`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            const list = await SetProfileModEnabled(profileId, fileName, enabled);
            setProfileModLists((current) => ({...current, [profileId]: list}));
            setMessage(enabled ? 'Mod enabled.' : 'Mod disabled.');
            appendLog({
                level: 'success',
                source: 'Mods',
                message: `${fileName} ${enabled ? 'enabled' : 'disabled'}.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Mod toggle failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function bulkToggleProfileMods(profileId: string, fileNames: string[], enabled: boolean) {
        const action = `${profileId}:bulk-toggle`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            let list: domain.ModList | null = null;
            for (const fileName of fileNames) {
                list = await SetProfileModEnabled(profileId, fileName, enabled);
            }
            if (list) {
                setProfileModLists((current) => ({...current, [profileId]: list as domain.ModList}));
            }
            const label = enabled ? 'enabled' : 'disabled';
            setMessage(`${fileNames.length} mod${fileNames.length === 1 ? '' : 's'} ${label}.`);
            appendLog({
                level: 'success',
                source: 'Mods',
                message: `${fileNames.length} selected mods ${label}.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Bulk mod toggle failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function removeProfileMod(profileId: string, fileName: string) {
        const action = `${profileId}:delete:${fileName}`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            const list = await DeleteProfileMod(profileId, fileName);
            setProfileModLists((current) => ({...current, [profileId]: list}));
            setMessage('Mod deleted.');
            appendLog({
                level: 'info',
                source: 'Mods',
                message: `${fileName} deleted.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Delete mod failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function openProfileModsFolder(profileId: string) {
        const action = `${profileId}:open`;
        try {
            setError('');
            setMessage('');
            setModActionKey(action);
            await OpenProfileModsFolder(profileId);
            await refreshProfileMods(profileId);
            setMessage('Mods folder opened.');
            appendLog({
                level: 'info',
                source: 'Mods',
                message: 'Mods folder opened.',
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Mods',
                message: `Open mods folder failed: ${text}`,
                profileId,
            });
        } finally {
            setModActionKey('');
        }
    }

    async function refreshProfileGameLogs(profileId: string) {
        const action = `${profileId}:logs:refresh`;
        try {
            setError('');
            setGameLogActionKey(action);
            const list = await ListProfileGameLogs(profileId);
            setProfileGameLogLists((current) => ({...current, [profileId]: list}));
            appendLog({
                level: 'success',
                source: 'Game Logs',
                message: `Loaded ${list.files?.length ?? 0} game log files.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Game Logs',
                message: `Game logs refresh failed: ${text}`,
                profileId,
            });
        } finally {
            setGameLogActionKey('');
        }
    }

    async function readProfileGameLog(profileId: string, fileName: string) {
        const action = `${profileId}:logs:read:${fileName}`;
        try {
            setError('');
            setGameLogActionKey(action);
            const content = await ReadProfileGameLog(profileId, fileName);
            setProfileGameLogContents((current) => ({...current, [gameLogContentKey(profileId, fileName)]: content}));
            appendLog({
                level: 'info',
                source: 'Game Logs',
                message: `Opened ${fileName}.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Game Logs',
                message: `Read game log failed: ${text}`,
                profileId,
            });
        } finally {
            setGameLogActionKey('');
        }
    }

    async function openProfileLogsFolder(profileId: string) {
        const action = `${profileId}:logs:open`;
        try {
            setError('');
            setGameLogActionKey(action);
            await OpenProfileLogsFolder(profileId);
            await refreshProfileGameLogs(profileId);
            appendLog({
                level: 'info',
                source: 'Game Logs',
                message: 'Logs folder opened.',
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Game Logs',
                message: `Open logs folder failed: ${text}`,
                profileId,
            });
        } finally {
            setGameLogActionKey('');
        }
    }

    async function exportProfileLogs(profileId: string) {
        const action = `${profileId}:logs:export`;
        try {
            setError('');
            setMessage('');
            setGameLogActionKey(action);
            const result = await ExportProfileLogs(profileId, launcherLogExportText(launcherLogs, profiles, localServers));
            if (!result.path) {
                appendLog({
                    level: 'info',
                    source: 'Game Logs',
                    message: 'Log export cancelled.',
                    profileId,
                });
                return;
            }
            await refreshProfileGameLogs(profileId);
            const text = logExportMessage(result);
            setMessage(text);
            appendLog({
                level: 'success',
                source: 'Game Logs',
                message: text,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Game Logs',
                message: `Log export failed: ${text}`,
                profileId,
            });
        } finally {
            setGameLogActionKey('');
        }
    }

    function changeBrowseProfile(profileId: string) {
        setBrowseProfileId(profileId);
        setBrowseResults(null);
        setBrowseDetailsProject(null);
        setPendingModrinthInstall(null);
        setPendingModrinthDelete(null);
        setPendingModrinthUpdate(null);
        setBrowseInstallKey('');
        setBrowseDeleteKey('');
        setBrowseUpdateKey('');
        if (profileId) {
            refreshInstalledModrinthProjects(profileId);
            refreshModrinthUpdates(profileId);
        }
    }

    async function searchModrinth(event?: FormEvent<HTMLFormElement>) {
        event?.preventDefault();
        if (!browseProfile) {
            setError('Select a profile before browsing mods.');
            return;
        }
        await searchModrinthForProfile(browseProfile.id, browseQuery);
    }

    async function searchModrinthForProfile(profileId: string, query: string) {
        const profile = profiles.find((item) => item.id === profileId);
        try {
            setError('');
            setMessage('');
            setBrowseLoading(true);
            const result = await SearchModrinthMods(profileId, query);
            setBrowseResults(result);
            setBrowseDetailsProject(null);
            await Promise.all([
                refreshInstalledModrinthProjects(profileId),
                refreshModrinthUpdates(profileId),
            ]);
            appendLog({
                level: 'success',
                source: 'Modrinth',
                message: `Found ${result.totalHits} compatible mods for ${profile?.name ?? profileId}.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth search failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseLoading(false);
        }
    }

    async function refreshInstalledModrinthProjects(profileId: string) {
        try {
            const projectIds = await ListInstalledModrinthProjects(profileId);
            setInstalledModrinthProjects((current) => {
                const next: Record<string, boolean> = {};
                for (const [key, value] of Object.entries(current)) {
                    if (!key.startsWith(`${profileId}:`)) {
                        next[key] = value;
                    }
                }
                for (const projectId of projectIds ?? []) {
                    next[`${profileId}:${projectId}`] = true;
                }
                return next;
            });
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Installed project refresh failed: ${errorText(err)}`,
                profileId,
            });
        }
    }

    async function refreshModrinthUpdates(profileId: string) {
        try {
            const plans = await ListModrinthUpdates(profileId);
            setModrinthUpdatePlans((current) => ({...current, [profileId]: plans ?? []}));
            appendLog({
                level: 'info',
                source: 'Modrinth',
                message: `Checked updates for ${plans?.length ?? 0} mods.`,
                profileId,
            });
        } catch (err) {
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth update check failed: ${errorText(err)}`,
                profileId,
            });
        }
    }

    async function installModrinthMod(projectId: string) {
        if (!browseProfile) {
            setError('Select a profile before installing mods.');
            return;
        }

        try {
            setError('');
            setMessage('');
            setBrowseInstallKey(projectId);
            const plan = await PlanModrinthInstall(browseProfile.id, projectId);
            const requiredDependencies = plan.requiredDependencies ?? [];
            if (requiredDependencies.length > 0) {
                setPendingModrinthInstall(plan);
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: `Install confirmation required for ${plan.projectTitle || projectId}.`,
                    profileId: browseProfile.id,
                });
                return;
            }
            await performModrinthInstall(plan);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth install failed: ${text}`,
                profileId: browseProfile.id,
            });
        } finally {
            setBrowseInstallKey('');
        }
    }

    async function installModrinthVersion(projectId: string, versionId: string) {
        if (!browseProfile) {
            setError('Select a profile before installing mods.');
            return;
        }

        const actionKey = modrinthVersionActionKey(projectId, versionId);
        try {
            setError('');
            setMessage('');
            setBrowseInstallKey(actionKey);
            const plan = await PlanModrinthInstallVersion(browseProfile.id, projectId, versionId);
            const requiredDependencies = plan.requiredDependencies ?? [];
            if (requiredDependencies.length > 0) {
                setPendingModrinthInstall(plan);
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: `Install confirmation required for ${plan.projectTitle || projectId} ${plan.versionNumber}.`,
                    profileId: browseProfile.id,
                });
                return;
            }
            await performModrinthInstall(plan);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth version install failed: ${text}`,
                profileId: browseProfile.id,
            });
        } finally {
            setBrowseInstallKey('');
        }
    }

    async function confirmPendingModrinthInstall(dependencyIDs: string[]) {
        if (!pendingModrinthInstall) {
            return;
        }
        const plan = pendingModrinthInstall;
        setPendingModrinthInstall(null);
        await performModrinthInstall(plan, dependencyIDs);
    }

    function cancelPendingModrinthInstall() {
        if (!pendingModrinthInstall) {
            return;
        }
        setMessage('Mod installation cancelled.');
        appendLog({
            level: 'info',
            source: 'Modrinth',
            message: `Cancelled install for ${pendingModrinthInstall.projectTitle || pendingModrinthInstall.projectId}.`,
            profileId: pendingModrinthInstall.profileId,
        });
        setPendingModrinthInstall(null);
        setBrowseInstallKey('');
    }

    async function performModrinthInstall(plan: domain.ModrinthInstallPlan, dependencyIDs?: string[]) {
        try {
            setError('');
            setMessage('');
            setBrowseInstallKey(modrinthVersionActionKey(plan.projectId, plan.versionId));
            const selectedDependencies = dependencyIDs ?? requiredDependencyIDs(plan.requiredDependencies ?? []);
            const result = await InstallModrinthModVersionFiles(plan.profileId, plan.projectId, plan.versionId, selectedDependencies);
            setProfileModLists((current) => ({...current, [plan.profileId]: result.modList}));
            setInstalledModrinthProjects((current) => ({
                ...current,
                ...markInstalledModrinthProjects(plan.profileId, plan.projectId, result.installedFiles ?? []),
            }));
            setMessage(modrinthInstallMessage(result));
            appendLog({
                level: 'success',
                source: 'Modrinth',
                message: modrinthInstallLogMessage(result),
                profileId: plan.profileId,
            });
            const dependencyDetails = modrinthDependencyInstallDetails(result);
            if (dependencyDetails) {
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: dependencyDetails,
                    profileId: plan.profileId,
                });
            }
            await refreshModrinthUpdates(plan.profileId);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth install failed: ${text}`,
                profileId: plan.profileId,
            });
        } finally {
            setBrowseInstallKey('');
        }
    }

    async function updateModrinthMod(profileId: string, projectId: string) {
        try {
            setError('');
            setMessage('');
            setBrowseUpdateKey(projectId);
            const plan = await PlanModrinthUpdate(profileId, projectId);
            if (!plan.updateAvailable) {
                setMessage(`${plan.projectTitle || projectId} is already up to date.`);
                await refreshModrinthUpdates(profileId);
                return;
            }
            const requiredDependencies = plan.requiredDependencies ?? [];
            if (requiredDependencies.length > 0) {
                setPendingModrinthUpdate(plan);
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: `Update confirmation required for ${plan.projectTitle || projectId}.`,
                    profileId,
                });
                return;
            }
            await performModrinthUpdate(plan);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth update failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseUpdateKey('');
        }
    }

    async function updateModrinthModFile(profileId: string, fileName: string) {
        try {
            setError('');
            setMessage('');
            setBrowseUpdateKey(fileName);
            const plan = await PlanModrinthUpdateFile(profileId, fileName);
            if (!plan.updateAvailable) {
                setMessage(`${plan.projectTitle || fileName} is already up to date.`);
                await refreshModrinthUpdates(profileId);
                return;
            }
            const requiredDependencies = plan.requiredDependencies ?? [];
            if (requiredDependencies.length > 0) {
                setPendingModrinthUpdate(plan);
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: `Update confirmation required for ${plan.projectTitle || fileName}.`,
                    profileId,
                });
                return;
            }
            await performModrinthUpdate(plan);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth update failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseUpdateKey('');
        }
    }

    async function updateModrinthVersion(profileId: string, projectId: string, versionId: string) {
        const actionKey = modrinthVersionActionKey(projectId, versionId);
        try {
            setError('');
            setMessage('');
            setBrowseUpdateKey(actionKey);
            const plan = await PlanModrinthUpdateVersion(profileId, projectId, versionId);
            if (!plan.updateAvailable) {
                setMessage(`${plan.projectTitle || projectId} ${plan.latestVersionNumber || versionId} is already installed.`);
                await refreshModrinthUpdates(profileId);
                return;
            }
            const requiredDependencies = plan.requiredDependencies ?? [];
            if (requiredDependencies.length > 0) {
                setPendingModrinthUpdate(plan);
                appendLog({
                    level: 'info',
                    source: 'Modrinth',
                    message: `Version switch confirmation required for ${plan.projectTitle || projectId}.`,
                    profileId,
                });
                return;
            }
            await performModrinthUpdate(plan);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth version update failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseUpdateKey('');
        }
    }

    async function confirmPendingModrinthUpdate(dependencyIDs: string[]) {
        if (!pendingModrinthUpdate) {
            return;
        }
        const plan = pendingModrinthUpdate;
        setPendingModrinthUpdate(null);
        await performModrinthUpdate(plan, dependencyIDs);
    }

    function cancelPendingModrinthUpdate() {
        if (!pendingModrinthUpdate) {
            return;
        }
        setMessage('Mod update cancelled.');
        appendLog({
            level: 'info',
            source: 'Modrinth',
            message: `Cancelled update for ${pendingModrinthUpdate.projectTitle || pendingModrinthUpdate.projectId}.`,
            profileId: pendingModrinthUpdate.profileId,
        });
        setPendingModrinthUpdate(null);
        setBrowseUpdateKey('');
    }

    async function performModrinthUpdate(plan: domain.ModrinthUpdatePlan, dependencyIDs?: string[]) {
        try {
            setError('');
            setMessage('');
            setBrowseUpdateKey(plan.tracked ? modrinthVersionActionKey(plan.projectId, plan.latestVersionId) : plan.currentFileName);
            const selectedDependencies = dependencyIDs ?? requiredDependencyIDs(plan.requiredDependencies ?? []);
            const result = plan.tracked
                ? await UpdateModrinthModVersionFiles(plan.profileId, plan.projectId, plan.latestVersionId, selectedDependencies)
                : await UpdateModrinthModFile(plan.profileId, plan.currentFileName, selectedDependencies);
            setProfileModLists((current) => ({...current, [plan.profileId]: result.modList}));
            setInstalledModrinthProjects((current) => ({
                ...current,
                ...markInstalledModrinthProjects(plan.profileId, result.projectId || plan.projectId, result.installedFiles ?? []),
            }));
            setMessage(modrinthUpdateMessage(result));
            appendLog({
                level: result.updated ? 'success' : 'info',
                source: 'Modrinth',
                message: modrinthUpdateLogMessage(result),
                profileId: plan.profileId,
            });
            await refreshModrinthUpdates(plan.profileId);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth update failed: ${text}`,
                profileId: plan.profileId,
            });
        } finally {
            setBrowseUpdateKey('');
        }
    }

    async function deleteModrinthMod(projectId: string) {
        if (!browseProfile) {
            setError('Select a profile before deleting mods.');
            return;
        }

        try {
            setError('');
            setMessage('');
            setBrowseDeleteKey(projectId);
            const plan = await PlanModrinthDelete(browseProfile.id, projectId);
            setPendingModrinthDelete(plan);
            setBrowseDeleteKey('');
            appendLog({
                level: 'info',
                source: 'Modrinth',
                message: `Delete confirmation required for ${plan.projectTitle || projectId}.`,
                profileId: browseProfile.id,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth delete planning failed: ${text}`,
                profileId: browseProfile.id,
            });
            setBrowseDeleteKey('');
        }
    }

    async function confirmPendingModrinthDelete(fileNames: string[]) {
        if (!pendingModrinthDelete) {
            return;
        }
        const plan = pendingModrinthDelete;
        setPendingModrinthDelete(null);
        await performModrinthDelete(plan, fileNames);
    }

    function cancelPendingModrinthDelete() {
        if (!pendingModrinthDelete) {
            return;
        }
        setMessage('Mod deletion cancelled.');
        appendLog({
            level: 'info',
            source: 'Modrinth',
            message: `Cancelled delete for ${pendingModrinthDelete.projectTitle || pendingModrinthDelete.projectId}.`,
            profileId: pendingModrinthDelete.profileId,
        });
        setPendingModrinthDelete(null);
        setBrowseDeleteKey('');
    }

    async function performModrinthDelete(plan: domain.ModrinthDeletePlan, fileNames: string[]) {
        try {
            setError('');
            setMessage('');
            setBrowseDeleteKey(plan.projectId);
            const result = await DeleteModrinthModFiles(plan.profileId, plan.projectId, fileNames);
            setProfileModLists((current) => ({...current, [plan.profileId]: result.modList}));
            setInstalledModrinthProjects((current) => ({
                ...current,
                ...unmarkDeletedModrinthProjects(plan.profileId, result),
            }));
            setMessage(modrinthDeleteMessage(result));
            appendLog({
                level: 'success',
                source: 'Modrinth',
                message: modrinthDeleteLogMessage(result),
                profileId: plan.profileId,
            });
            await refreshModrinthUpdates(plan.profileId);
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Modrinth delete failed: ${text}`,
                profileId: plan.profileId,
            });
        } finally {
            setBrowseDeleteKey('');
        }
    }

    async function openModrinthProject(projectId: string) {
        await openModrinthProjectForProfile(browseProfile?.id, projectId);
    }

    async function openModrinthProjectForProfile(profileId: string | undefined, projectId: string) {
        try {
            setError('');
            setBrowseDetailsLoading(true);
            const project = await GetModrinthProject(projectId);
            setBrowseDetailsProject(project);
            appendLog({
                level: 'info',
                source: 'Modrinth',
                message: `Opened ${project.title}.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Open project failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseDetailsLoading(false);
        }
    }

    async function loadModrinthProjectVersions(profileId: string, projectId: string) {
        const key = modrinthProjectProfileKey(profileId, projectId);
        try {
            setError('');
            setBrowseVersionsLoadingKey(key);
            const versions = await ListModrinthProjectVersions(profileId, projectId);
            setBrowseProjectVersions((current) => ({...current, [key]: versions ?? []}));
            appendLog({
                level: 'info',
                source: 'Modrinth',
                message: `Loaded ${versions?.length ?? 0} compatible versions.`,
                profileId,
            });
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Modrinth',
                message: `Version list failed: ${text}`,
                profileId,
            });
        } finally {
            setBrowseVersionsLoadingKey((current) => current === key ? '' : current);
        }
    }

    async function browseProfileMod(profileId: string, projectId: string, query: string) {
        const cleanQuery = modBrowseQuery(query);
        navigateToScreen('browse');
        setBrowseProfileId(profileId);
        setBrowseQuery(cleanQuery);
        setBrowseResults(null);
        setBrowseDetailsProject(null);
        setPendingModrinthInstall(null);
        setPendingModrinthDelete(null);
        setPendingModrinthUpdate(null);
        setBrowseInstallKey('');
        setBrowseDeleteKey('');
        setBrowseUpdateKey('');
        await refreshInstalledModrinthProjects(profileId);
        await refreshModrinthUpdates(profileId);
        if (projectId) {
            await openModrinthProjectForProfile(profileId, projectId);
            return;
        }
        if (cleanQuery) {
            await searchModrinthForProfile(profileId, cleanQuery);
        }
    }

    async function installProfile(id: string) {
        await runProfileInstall(id, false);
    }

    async function repairProfile(id: string) {
        await runProfileInstall(id, true);
    }

    async function runProfileInstall(id: string, repair: boolean) {
        const action = repair ? 'Repair' : 'Install';
        const busyStatus = repair ? 'repairing' : 'installing';
        const busyMessage = repair ? 'Checking and repairing base files' : 'Installing vanilla base files';
        try {
            setError('');
            setMessage('');
            setProfiles((current) => current.map((profile) => profile.id === id
                ? domain.Profile.createFrom({
                    ...profile,
                    install: {
                        ...profile.install,
                        status: busyStatus,
                        message: busyMessage,
                    },
                })
                : profile
            ));
            appendLog({
                level: 'info',
                source: action,
                message: `${action} requested.`,
                profileId: id,
            });
            const profile = repair ? await RepairProfile(id) : await InstallProfile(id);
            const profileList = await ListProfiles();
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId || profile.id);
            setMessage(profile.install?.message || `${action} complete.`);
            appendLog({
                level: profile.install?.status === 'failed' ? 'error' : 'success',
                source: action,
                message: profile.install?.message || `${action} complete.`,
                profileId: id,
            });
            await validateJava();
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: action,
                message: `${action} failed: ${errorText(err)}`,
                profileId: id,
            });
            await refreshApp();
        }
    }

    async function launchProfile(id: string) {
        try {
            setError('');
            setMessage('');
            setLaunchStates((current) => ({
                ...current,
                [id]: {
                    profileId: id,
                    status: 'starting',
                    message: 'Starting Minecraft',
                    startedAt: new Date().toISOString(),
                },
            }));
            appendLog({
                level: 'info',
                source: 'Launch',
                message: 'Launch requested.',
                profileId: id,
            });
            const state = await LaunchProfile(id);
            setLaunchStates((current) => ({
                ...current,
                [id]: {
                    profileId: state.profileId,
                    status: state.status as LaunchState['status'],
                    message: state.message,
                    exitCode: state.exitCode,
                    startedAt: state.startedAt,
                    endedAt: state.endedAt,
                },
            }));
        } catch (err) {
            setError(errorText(err));
            setLaunchStates((current) => ({
                ...current,
                [id]: {
                    profileId: id,
                    status: 'failed',
                    message: errorText(err),
                    endedAt: new Date().toISOString(),
                },
            }));
            appendLog({
                level: 'error',
                source: 'Launch',
                message: `Launch failed: ${errorText(err)}`,
                profileId: id,
            });
        }
    }

    async function saveSettings(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        if (!settings) {
            return;
        }
        try {
            setError('');
            setMessage('');
            const saved = await SaveSettings(new domain.Settings({
                ...settings,
                account,
            }));
            setSettings(settingsDraft(saved));
            setAccount(accountDraft(saved.account));
            setMessage('Settings saved.');
            appendLog({
                level: 'success',
                source: 'Settings',
                message: 'Settings saved.',
            });
            await validateJava();
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Settings',
                message: `Save settings failed: ${errorText(err)}`,
            });
        }
    }

    async function saveAccount(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        try {
            setError('');
            setMessage('');
            const saved = await SaveAccount(new domain.AccountConfig(account));
            setAccount(accountDraft(saved));
            setMessage('Account saved.');
            appendLog({
                level: 'success',
                source: 'Account',
                message: `Account saved as ${accountLabel(saved)}.`,
            });
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Account',
                message: `Save account failed: ${errorText(err)}`,
            });
        }
    }

    function renderLocalServerPanel() {
        return (
            <LocalServerQuickPanel
                servers={localServers}
                form={localServerForm}
                minecraftVersions={minecraftVersions}
                actionKey={localServerActionKey}
                onFormChange={setLocalServerForm}
                onCreate={createLocalServer}
            />
        );
    }

    function navigateToScreen(nextScreen: Screen) {
        closeProfileSettings();
        closeLocalServerSettings();
        setScreen(nextScreen);
    }

    return (
        <div className="app-shell">
            <aside className="rail">
                <div className="rail-main">
                    <div className="brand">
                        <div className="brand-mark">PM</div>
                        <div>
                            <strong>{info?.name ?? 'Power Mine'}</strong>
                            <span>version: {info?.version ?? '0.2.0'}</span>
                        </div>
                    </div>

                    <nav className="nav-list" aria-label="Primary navigation">
                        {navItems.map((item) => {
                            const active = screen === item.id;
                            return (
                                <button
                                    key={item.id}
                                    type="button"
                                    className={active ? 'nav-item active' : 'nav-item'}
                                    onClick={() => navigateToScreen(item.id)}
                                    aria-current={active ? 'page' : undefined}
                                >
                                    <span className="nav-mark">{item.mark}</span>
                                    <strong>{item.label}</strong>
                                    {active && <em aria-hidden="true"/>}
                                </button>
                            );
                        })}
                    </nav>
                </div>

                <button
                    className={screen === 'account' ? 'rail-card rail-card-button active' : 'rail-card rail-card-button'}
                    type="button"
                    onClick={() => navigateToScreen('account')}
                    aria-current={screen === 'account' ? 'page' : undefined}
                    aria-label="Open account settings"
                >
                    <p className="eyebrow">Account</p>
                    <strong>{accountLabel(account)}</strong>
                    <span>{account.mode === 'microsoft' ? 'mode: microsoft' : 'mode: offline'}</span>
                </button>
            </aside>

            <main className="workspace">
                <header className="topbar">
                    <div>
                        <p className="eyebrow">Power Mine // Launcher</p>
                        <h1>{titleFor(screen)}</h1>
                        <p className="topbar-subtitle">Minecraft Java control surface for profiles, mods, logs, and runtime checks.</p>
                    </div>
                    <div className="topbar-meta">
                        <div className="account-pill">
                            <span className="status-dot"/>
                            {accountLabel(account)}
                        </div>
                        <div className="account-pill muted-pill">
                            build {info?.version ?? '0.2.0'}
                        </div>
                        <button type="button" onClick={refreshVisibleState} disabled={appRefreshing}>
                            {appRefreshing ? 'Refreshing' : 'Refresh'}
                        </button>
                    </div>
                </header>

                {error && <div className="banner error">{error}</div>}
                {message && <div className="banner success">{message}</div>}

                {screen === 'home' && (
                    <HomePanel
                        profiles={profiles}
                        selectedProfile={selectedProfile}
                        selectedProfileId={selectedProfileId}
                        localServers={localServers}
                        selectedLocalServer={selectedLocalServer}
                        selectedLocalServerId={selectedLocalServerId}
                        localServerForm={localServerForm}
                        account={account}
                        javaStatus={javaStatus}
                        javaPath={settings?.javaPath ?? 'java'}
                        installProgress={installProgress}
                        localServerProgress={localServerProgress}
                        javaInstallProgress={javaInstallProgress}
                        profileJavaRuntimes={profileJavaRuntimes}
                        launchStates={launchStates}
                        localServerRunStates={localServerRunStates}
                        localServerActionKey={localServerActionKey}
                        minecraftVersions={minecraftVersions}
                        selectedLogs={selectedLogs}
                        onSelectProfile={selectProfile}
                        onSelectLocalServer={selectLocalServer}
                        onLocalServerFormChange={setLocalServerForm}
                        onCreateLocalServer={createLocalServer}
                        onInstallLocalServer={installLocalServer}
                        onRepairLocalServer={repairLocalServer}
                        onStartLocalServer={startLocalServer}
                        onStopLocalServer={stopLocalServer}
                        onOpenLocalServerFolder={openLocalServerFolder}
                        onOpenLocalServerSettings={openLocalServerSettings}
                        onOpenLocalServerTerminal={openLocalServerTerminal}
                        onInstallProfile={installProfile}
                        onRepairProfile={repairProfile}
                        onInstallJava={installJava}
                        onLaunchProfile={launchProfile}
                        onOpenBrowse={(profileId) => {
                            setBrowseProfileId(profileId);
                            navigateToScreen('browse');
                        }}
                        onOpenCreate={() => navigateToScreen('create')}
                        onOpenProfileSettings={openProfileSettings}
                        onOpenLogs={() => navigateToScreen('logs')}
                    />
                )}

                {screen === 'library' && (
                    <section className="library-layout">
                        <div className="profile-list">
                            {libraryTotalCount === 0 && <EmptyState title="No installations" action="Create a profile or local server."/>}
                            {profiles.map((profile) => (
                                <button
                                    key={profile.id}
                                    className={libraryDetailType === 'profile' && librarySelectedProfile?.id === profile.id ? 'profile-row active' : 'profile-row'}
                                    type="button"
                                    onClick={() => selectProfile(profile.id)}
                                >
                                    <div className="installation-title">
                                        <strong>{profile.name}</strong>
                                        <span>Client</span>
                                    </div>
                                    <span>{profileSubtitle(profile)}</span>
                                    <small>{installStatusText(profile)}</small>
                                </button>
                            ))}
                            {localServers.map((server) => {
                                const run = localServerRunStates[server.id];
                                const active = libraryDetailType === 'server' && librarySelectedLocalServer?.id === server.id;

                                return (
                                    <button
                                        key={`server:${server.id}`}
                                        className={active ? 'profile-row active' : 'profile-row'}
                                        type="button"
                                        onClick={() => selectLocalServer(server.id)}
                                    >
                                        <div className="installation-title">
                                            <strong>{server.name}</strong>
                                            <span>Server</span>
                                        </div>
                                        <span>{localServerSubtitle(server)}</span>
                                        <small>{localServerStatusText(server, run)}</small>
                                    </button>
                                );
                            })}
                        </div>
                        {libraryDetailType === 'server' ? (
                            <LocalServerDetail
                                server={librarySelectedLocalServer}
                                progress={libraryLocalServerProgress}
                                run={libraryLocalServerRun}
                                actionKey={localServerActionKey}
                                onInstall={installLocalServer}
                                onRepair={repairLocalServer}
                                onStart={startLocalServer}
                                onStop={stopLocalServer}
                                onOpenFolder={openLocalServerFolder}
                                onOpenSettings={openLocalServerSettings}
                                onOpenTerminal={openLocalServerTerminal}
                            />
                        ) : (
                            <ProfileDetail
                                profile={librarySelectedProfile}
                                progress={libraryProfileProgress}
                                launch={libraryProfileLaunch}
                                javaRuntime={libraryProfileJavaRuntime}
                                javaInstallProgress={javaInstallProgress}
                                modList={libraryProfileModList}
                                modrinthUpdatePlans={libraryProfileUpdatePlans}
                                modActionKey={modActionKey}
                                settingsOpen={!!librarySelectedProfile && profileSettingsId === librarySelectedProfile.id}
                                settingsDraft={profileSettingsDraft}
                                onDelete={deleteProfile}
                                onInstall={installProfile}
                                onRepair={repairProfile}
                                onInstallJava={installJava}
                                onLaunch={launchProfile}
                                onOpenSettings={openProfileSettings}
                                onCloseSettings={closeProfileSettings}
                                onSettingsDraftChange={setProfileSettingsDraft}
                                onSaveSettings={saveProfileSettings}
                                onRefreshMods={reloadProfileMods}
                                onImportMod={importProfileMod}
                                onExportModpack={exportProfileModpack}
                                onOpenModsFolder={openProfileModsFolder}
                                onCheckModrinthUpdates={refreshModrinthUpdates}
                                onUpdateModrinthProject={updateModrinthMod}
                                onUpdateModrinthFile={updateModrinthModFile}
                                onBrowseMod={browseProfileMod}
                                onToggleMod={toggleProfileMod}
                                onBulkToggleMods={bulkToggleProfileMods}
                                onDeleteMod={removeProfileMod}
                            />
                        )}
                    </section>
                )}

                {screen === 'create' && (
                    <section className="create-layout">
                    <form className="form-grid" onSubmit={createProfile}>
                        <div className="wide catalog-status">
                            <div>
                                <p className="eyebrow">Version catalog</p>
                                <strong>{versionCatalogStatus}</strong>
                                <p>{versionCatalogWarning || 'Minecraft and loader metadata are cached for offline startup.'}</p>
                            </div>
                            <div className="inline-actions">
                                <button type="button" onClick={importModrinthModpack} disabled={modpackImporting}>
                                    {modpackImporting ? 'Importing .mrpack' : 'Import .mrpack'}
                                </button>
                                <button type="button" onClick={refreshVersionOptions}>Refresh</button>
                            </div>
                        </div>
                        {(modpackImporting || createImportProgress) && (
                            <section className="wide import-progress">
                                <div>
                                    <p className="eyebrow">Modpack import</p>
                                    <strong>{createImportProgress ? progressMessage(createImportProgress) : 'Waiting for modpack selection'}</strong>
                                    <p>{createImportProgress?.stage ?? 'Select a .mrpack file to start importing.'}</p>
                                </div>
                                <ProgressBar progress={createImportProgress}/>
                            </section>
                        )}
                        <label>
                            Profile name
                            <input
                                value={createForm.name}
                                onChange={(event) => setCreateForm({...createForm, name: event.target.value})}
                            />
                        </label>
                        <VersionPicker
                            label="Minecraft version"
                            value={createForm.minecraftVersion}
                            versions={minecraftVersions}
                            disabled={minecraftVersions.length === 0}
                            onChange={(minecraftVersion) => setCreateForm({
                                ...createForm,
                                minecraftVersion,
                                loaderVersion: pickCurrentValue(
                                    createForm.loaderVersion || 'latest',
                                    loaderVersionOptions(createForm.loaderType, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, minecraftVersion)
                                ),
                            })}
                        />
                        <label>
                            Loader
                            <select
                                value={createForm.loaderType}
                                onChange={(event) => setCreateForm({
                                    ...createForm,
                                    loaderType: event.target.value,
                                    loaderVersion: pickCurrentValue(
                                        createForm.loaderVersion || 'latest',
                                        loaderVersionOptions(event.target.value, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, createForm.minecraftVersion)
                                    ),
                                })}
                            >
                                <option value="fabric">Fabric</option>
                                <option value="quilt">Quilt</option>
                                <option value="forge">Forge</option>
                                <option value="neoforge">NeoForge</option>
                                <option value="vanilla">Vanilla</option>
                            </select>
                        </label>
                        {createForm.loaderType !== 'vanilla' && (
                            <label>
                                Loader version
                                <select
                                    value={createForm.loaderVersion}
                                    onChange={(event) => setCreateForm({...createForm, loaderVersion: event.target.value})}
                                >
                                    {loaderVersionOptions(createForm.loaderType, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, createForm.minecraftVersion).map((version) => (
                                        <option key={version.id} value={version.id}>
                                            {version.label}
                                        </option>
                                    ))}
                                </select>
                            </label>
                        )}
                        <label className="wide">
                            Game directory
                            <input
                                value={createForm.gameDir}
                                placeholder="Default instance directory"
                                onChange={(event) => setCreateForm({...createForm, gameDir: event.target.value})}
                            />
                        </label>
                        <label>
                            Min memory MB
                            <input
                                type="number"
                                min="512"
                                step="256"
                                value={createForm.minMB}
                                onChange={(event) => setCreateForm({...createForm, minMB: Number(event.target.value)})}
                            />
                        </label>
                        <label>
                            Max memory MB
                            <input
                                type="number"
                                min="512"
                                step="256"
                                value={createForm.maxMB}
                                onChange={(event) => setCreateForm({...createForm, maxMB: Number(event.target.value)})}
                            />
                        </label>
                        <div className="form-actions wide">
                            <button className="primary" type="submit">Create profile</button>
                        </div>
                    </form>
                    {renderLocalServerPanel()}
                    </section>
                )}

                {screen === 'account' && (
                    <form className="form-grid" onSubmit={saveAccount}>
                        <div className="wide settings-status">
                            <div>
                                <p className="eyebrow">Current account</p>
                                <strong>{accountLabel(account)}</strong>
                                <p>{account.offlineUuid || 'Offline UUID will be generated on save.'}</p>
                            </div>
                        </div>
                        <label>
                            Account mode
                            <select
                                value={account.mode}
                                onChange={(event) => setAccount({...account, mode: event.target.value})}
                            >
                                <option value="offline">Offline profile</option>
                            </select>
                        </label>
                        <label>
                            Offline player name
                            <input
                                value={account.offlineName}
                                minLength={3}
                                maxLength={16}
                                pattern="[A-Za-z0-9_]+"
                                onChange={(event) => setAccount({...account, offlineName: event.target.value})}
                            />
                        </label>
                        <label className="wide">
                            Offline UUID
                            <input value={account.offlineUuid || 'Generated on save'} readOnly/>
                        </label>
                        <div className="form-actions wide">
                            <button className="primary" type="submit">Save account</button>
                        </div>
                    </form>
                )}

                {screen === 'logs' && (
                    <ClassicLogsPanel
                        logs={launcherLogs}
                        profiles={profiles}
                        localServers={localServers}
                        gameLogLists={profileGameLogLists}
                        gameLogContents={profileGameLogContents}
                        gameLogActionKey={gameLogActionKey}
                        exportProfileId={selectedProfile?.id ?? selectedProfileId}
                        onClearLogs={() => setLauncherLogs([])}
                        onRefreshGameLogs={refreshProfileGameLogs}
                        onReadGameLog={readProfileGameLog}
                        onOpenLogsFolder={openProfileLogsFolder}
                        onOpenWindow={openDetachedLogsWindow}
                        onExportLogs={exportProfileLogs}
                    />
                )}

                {screen === 'settings' && settings && (
                    <form className="form-grid" onSubmit={saveSettings}>
                        <label className="wide">
                            App data directory
                            <input value={settings.dataDir} readOnly/>
                        </label>
                        <label className="wide">
                            Java executable
                            <input
                                value={settings.javaPath}
                                onChange={(event) => setSettings({...settings, javaPath: event.target.value})}
                            />
                        </label>
                        <div className="wide settings-status">
                            <div>
                                <p className="eyebrow">Java status</p>
                                <strong>{javaStatusText(javaStatus, settings.javaPath)}</strong>
                                <p>{javaStatus?.message ?? 'Java has not been checked yet.'}</p>
                            </div>
                            <div className="settings-actions">
                                <button type="button" onClick={validateJava}>Check Java</button>
                            </div>
                        </div>
                        <label>
                            Default min memory MB
                            <input
                                type="number"
                                min="512"
                                step="256"
                                value={settings.defaultMemory.minMB}
                                onChange={(event) => setSettings({
                                    ...settings,
                                    defaultMemory: {...settings.defaultMemory, minMB: Number(event.target.value)}
                                })}
                            />
                        </label>
                        <label>
                            Default max memory MB
                            <input
                                type="number"
                                min="512"
                                step="256"
                                value={settings.defaultMemory.maxMB}
                                onChange={(event) => setSettings({
                                    ...settings,
                                    defaultMemory: {...settings.defaultMemory, maxMB: Number(event.target.value)}
                                })}
                            />
                        </label>
                        <label>
                            Retry count
                            <input
                                type="number"
                                min="0"
                                value={settings.network.retryCount}
                                onChange={(event) => setSettings({
                                    ...settings,
                                    network: {...settings.network, retryCount: Number(event.target.value)}
                                })}
                            />
                        </label>
                        <label>
                            Metadata TTL hours
                            <input
                                type="number"
                                min="1"
                                value={settings.network.metadataTtlHours}
                                onChange={(event) => setSettings({
                                    ...settings,
                                    network: {...settings.network, metadataTtlHours: Number(event.target.value)}
                                })}
                            />
                        </label>
                        <div className="form-actions wide">
                            <button className="primary" type="submit">Save settings</button>
                        </div>
                    </form>
                )}

                {screen === 'browse' && (
                    <BrowsePanel
                        profiles={profiles}
                        profile={browseProfile}
                        query={browseQuery}
                        results={browseResults}
                        loading={browseLoading}
                        detailsProject={browseDetailsProject}
                        detailsLoading={browseDetailsLoading}
                        installingProjectId={browseInstallKey}
                        deletingProjectId={browseDeleteKey}
                        updatingProjectId={browseUpdateKey}
                        installedProjects={installedModrinthProjects}
                        updatePlans={browseProfile?.id ? modrinthUpdatePlans[browseProfile.id] ?? [] : []}
                        projectVersions={browseProjectVersions}
                        versionsLoadingKey={browseVersionsLoadingKey}
                        onProfileChange={changeBrowseProfile}
                        onQueryChange={setBrowseQuery}
                        onSearch={searchModrinth}
                        onOpenDetails={openModrinthProject}
                        onLoadVersions={loadModrinthProjectVersions}
                        onBackToResults={() => setBrowseDetailsProject(null)}
                        onInstall={installModrinthMod}
                        onInstallVersion={installModrinthVersion}
                        onUpdate={(projectId) => browseProfile && updateModrinthMod(browseProfile.id, projectId)}
                        onUpdateVersion={(projectId, versionId) => browseProfile && updateModrinthVersion(browseProfile.id, projectId, versionId)}
                        onDelete={deleteModrinthMod}
                    />
                )}
                {settingsProfile && profileSettingsDraft && (
                    <ProfileSettingsDialog
                        profile={settingsProfile}
                        draft={profileSettingsDraft}
                        javaRuntime={settingsProfileJavaRuntime}
                        javaInstallProgress={javaInstallProgress}
                        modList={settingsProfileModList}
                        modrinthUpdatePlans={settingsProfileUpdatePlans}
                        modActionKey={modActionKey}
                        onClose={closeProfileSettings}
                        onDraftChange={setProfileSettingsDraft}
                        onInstallJava={installJava}
                        onSave={saveProfileSettings}
                        onRefreshMods={reloadProfileMods}
                        onImportMod={importProfileMod}
                        onExportModpack={exportProfileModpack}
                        onOpenModsFolder={openProfileModsFolder}
                        onCheckModrinthUpdates={refreshModrinthUpdates}
                        onUpdateModrinthProject={updateModrinthMod}
                        onUpdateModrinthFile={updateModrinthModFile}
                        onBrowseMod={browseProfileMod}
                        onToggleMod={toggleProfileMod}
                        onBulkToggleMods={bulkToggleProfileMods}
                        onDeleteMod={removeProfileMod}
                    />
                )}
                {settingsLocalServer && (
                    <LocalServerSettingsDialog
                        server={settingsLocalServer}
                        progress={localServerProgress[settingsLocalServer.id]}
                        run={localServerRunStates[settingsLocalServer.id]}
                        actionKey={localServerActionKey}
                        onClose={closeLocalServerSettings}
                        onInstall={installLocalServer}
                        onRepair={repairLocalServer}
                        onStart={startLocalServer}
                        onStop={stopLocalServer}
                        onOpenFolder={openLocalServerFolder}
                        onOpenRawSettings={openLocalServerSettingsFile}
                        onOpenTerminal={openLocalServerTerminal}
                    />
                )}
                {pendingModrinthInstall && (
                    <ModrinthDependencyConfirmDialog
                        plan={pendingModrinthInstall}
                        confirming={browseInstallKey === modrinthVersionActionKey(pendingModrinthInstall.projectId, pendingModrinthInstall.versionId)}
                        onCancel={cancelPendingModrinthInstall}
                        onConfirm={confirmPendingModrinthInstall}
                    />
                )}
                {pendingModrinthDelete && (
                    <ModrinthDeleteConfirmDialog
                        plan={pendingModrinthDelete}
                        deleting={browseDeleteKey === pendingModrinthDelete.projectId}
                        onCancel={cancelPendingModrinthDelete}
                        onConfirm={confirmPendingModrinthDelete}
                    />
                )}
                {pendingModrinthUpdate && (
                    <ModrinthUpdateConfirmDialog
                        plan={pendingModrinthUpdate}
                        updating={browseUpdateKey === (pendingModrinthUpdate.tracked
                            ? modrinthVersionActionKey(pendingModrinthUpdate.projectId, pendingModrinthUpdate.latestVersionId)
                            : pendingModrinthUpdate.currentFileName)}
                        onCancel={cancelPendingModrinthUpdate}
                        onConfirm={confirmPendingModrinthUpdate}
                    />
                )}
                {logsWindowOpen && (
                    <DetachedLogsWindow
                        logs={launcherLogs}
                        profiles={profiles}
                        localServers={localServers}
                        onClose={() => setLogsWindowOpen(false)}
                    />
                )}
                {terminalLocalServer && (
                    <ServerTerminalWindow
                        server={terminalLocalServer}
                        events={localServerTerminalEvents[terminalLocalServer.id] ?? []}
                        run={localServerRunStates[terminalLocalServer.id]}
                        actionKey={serverTerminalActionKey}
                        onClose={() => setTerminalServerId('')}
                        onSendCommand={sendLocalServerTerminalCommand}
                    />
                )}
            </main>
        </div>
    );
}

function VersionPicker({
    label,
    value,
    versions,
    disabled,
    onChange
}: {
    label: string;
    value: string;
    versions: domain.VersionOption[];
    disabled?: boolean;
    onChange: (value: string) => void;
}) {
    const [query, setQuery] = useState('');
    const [open, setOpen] = useState(false);
    const selected = versions.find((version) => version.id === value);
    const selectedLabel = selected?.label ?? value;
    const normalizedQuery = query.trim().toLowerCase();
    const matches = versions
        .filter((version) => version.id !== value)
        .filter((version) => {
            if (!normalizedQuery) {
                return true;
            }
            const haystack = `${version.id} ${version.label} ${version.type ?? ''}`.toLowerCase();
            return haystack.includes(normalizedQuery);
        })
        .slice(0, 80);
    const showOptions = open && !disabled;

    function selectVersion(nextValue: string) {
        onChange(nextValue);
        setQuery('');
        setOpen(false);
    }

    return (
        <div className="version-picker">
            <span>{label}</span>
            <input
                value={query}
                disabled={disabled}
                placeholder={selectedLabel || 'Search versions'}
                onFocus={() => setOpen(true)}
                onChange={(event) => {
                    setQuery(event.target.value);
                    setOpen(true);
                }}
                onBlur={() => window.setTimeout(() => setOpen(false), 120)}
            />
            <small>Current: {selectedLabel || 'none'}</small>
            {showOptions && (
                <div className="version-picker-popover">
                    {selected && (
                        <button type="button" className="version-picker-option current" onMouseDown={(event) => {
                            event.preventDefault();
                            selectVersion(selected.id);
                        }} onClick={() => selectVersion(selected.id)}>
                            <strong>{selected.label}</strong>
                            <span>Current</span>
                        </button>
                    )}
                    {matches.map((version) => (
                        <button type="button" className="version-picker-option" key={version.id} onMouseDown={(event) => {
                            event.preventDefault();
                            selectVersion(version.id);
                        }} onClick={() => selectVersion(version.id)}>
                            <strong>{version.label}</strong>
                            <span>{version.type || (version.stable ? 'Stable' : 'Version')}</span>
                        </button>
                    ))}
                    {matches.length === 0 && <p className="version-picker-empty">No versions match this search.</p>}
                </div>
            )}
        </div>
    );
}

function HomePanel({
    profiles,
    selectedProfile,
    selectedProfileId,
    localServers,
    selectedLocalServer,
    selectedLocalServerId,
    localServerForm,
    account,
    javaStatus,
    javaPath,
    installProgress,
    localServerProgress,
    javaInstallProgress,
    profileJavaRuntimes,
    launchStates,
    localServerRunStates,
    localServerActionKey,
    minecraftVersions,
    selectedLogs,
    onSelectProfile,
    onSelectLocalServer,
    onLocalServerFormChange,
    onCreateLocalServer,
    onInstallLocalServer,
    onRepairLocalServer,
    onStartLocalServer,
    onStopLocalServer,
    onOpenLocalServerFolder,
    onOpenLocalServerSettings,
    onOpenLocalServerTerminal,
    onInstallProfile,
    onRepairProfile,
    onInstallJava,
    onLaunchProfile,
    onOpenBrowse,
    onOpenCreate,
    onOpenProfileSettings,
    onOpenLogs
}: {
    profiles: domain.Profile[];
    selectedProfile?: domain.Profile;
    selectedProfileId: string;
    localServers: domain.LocalServer[];
    selectedLocalServer?: domain.LocalServer;
    selectedLocalServerId: string;
    localServerForm: LocalServerForm;
    account: AccountDraft;
    javaStatus: domain.JavaStatus | null;
    javaPath: string;
    installProgress: Record<string, InstallProgress>;
    localServerProgress: Record<string, LocalServerProgress>;
    javaInstallProgress: JavaInstallProgress | null;
    profileJavaRuntimes: Record<string, domain.ProfileJavaRuntime>;
    launchStates: Record<string, LaunchState>;
    localServerRunStates: Record<string, LocalServerRunState>;
    localServerActionKey: string;
    minecraftVersions: domain.VersionOption[];
    selectedLogs: LauncherLog[];
    onSelectProfile: (id: string) => void;
    onSelectLocalServer: (id: string) => void;
    onLocalServerFormChange: (form: LocalServerForm) => void;
    onCreateLocalServer: (event: FormEvent<HTMLFormElement>) => void;
    onInstallLocalServer: (id: string) => void;
    onRepairLocalServer: (id: string) => void;
    onStartLocalServer: (id: string) => void;
    onStopLocalServer: (id: string) => void;
    onOpenLocalServerFolder: (id: string) => void;
    onOpenLocalServerSettings: (server: domain.LocalServer) => void;
    onOpenLocalServerTerminal: (id: string) => void;
    onInstallProfile: (id: string) => void;
    onRepairProfile: (id: string) => void;
    onInstallJava: (version: number) => void;
    onLaunchProfile: (id: string) => void;
    onOpenBrowse: (profileId: string) => void;
    onOpenCreate: () => void;
    onOpenProfileSettings: (profile: domain.Profile) => void;
    onOpenLogs: () => void;
}) {
    const selectedProgress = selectedProfile ? installProgress[selectedProfile.id] : undefined;
    const selectedLaunch = selectedProfile ? launchStates[selectedProfile.id] : undefined;
    const selectedJavaRuntime = selectedProfile ? profileJavaRuntimes[selectedProfile.id] : undefined;
    const selectedPlayReason = playDisabledReason(selectedProfile, selectedLaunch, selectedJavaRuntime);
    const selectedInstalling = selectedProfile ? isInstalling(selectedProfile, installProgress) : false;
    const selectedInstallVisible = selectedProfile ? shouldShowInstallButton(selectedProfile, installProgress[selectedProfile.id]) : false;
    const selectedRepairVisible = selectedProfile ? shouldShowRepairButton(selectedProfile, installProgress[selectedProfile.id]) : false;
    const javaRequired = selectedJavaRuntime && !selectedJavaRuntime.installed;
    const javaBusy = isJavaInstalling(javaInstallProgress);
    const installedCount = profiles.filter((profile) => profile.install?.status === 'installed').length;
    const runningCount = Object.values(launchStates).filter((launch) => launch.status === 'running' || launch.status === 'starting').length;
    const runningServerCount = Object.values(localServerRunStates).filter((run) => run.status === 'running' || run.status === 'starting').length;
    const installedServerCount = localServers.filter((server) => server.install?.status === 'installed').length;
    const readyCount = profiles.filter((profile) => !playDisabledReason(
        profile,
        launchStates[profile.id],
        profileJavaRuntimes[profile.id]
    )).length;

    return (
        <section className="home-grid">
            <div className="launch-panel">
                <div className="launch-main">
                    <p className="eyebrow">Home</p>
                    <h2>{selectedProfile?.name ?? 'Create your first installation'}</h2>
                    <p>{selectedProfile ? profileSubtitle(selectedProfile) : 'No launch target yet.'}</p>
                    {profiles.length > 0 && (
                        <p className="launch-note">Choose clients and servers from the installation cards below.</p>
                    )}
                </div>
                <div className="quick-actions">
                    {selectedProfile ? (
                        <>
                            <button
                                className="primary"
                                type="button"
                                disabled={!!selectedPlayReason}
                                onClick={() => onLaunchProfile(selectedProfile.id)}
                            >
                                {selectedLaunch?.status === 'running' || selectedLaunch?.status === 'starting' ? 'Running' : 'Play'}
                            </button>
                            {selectedInstallVisible && (
                                <button
                                    type="button"
                                    disabled={selectedInstalling}
                                    onClick={() => onInstallProfile(selectedProfile.id)}
                                >
                                    {selectedInstalling ? 'Installing' : 'Install'}
                                </button>
                            )}
                            {selectedRepairVisible && (
                                <button
                                    type="button"
                                    disabled={selectedInstalling}
                                    onClick={() => onRepairProfile(selectedProfile.id)}
                                >
                                    {selectedInstalling ? 'Repairing' : 'Repair'}
                                </button>
                            )}
                            {javaRequired && (
                                <button
                                    type="button"
                                    disabled={javaBusy}
                                    onClick={() => onInstallJava(selectedJavaRuntime.requiredMajor)}
                                >
                                    {javaBusy ? 'Installing Java' : `Install Java ${selectedJavaRuntime.requiredMajor}`}
                                </button>
                            )}
                            <button type="button" onClick={() => onOpenProfileSettings(selectedProfile)}>Settings</button>
                            <button type="button" onClick={() => onOpenBrowse(selectedProfile.id)}>Browse mods</button>
                            <button type="button" onClick={onOpenLogs}>Logs</button>
                            {selectedPlayReason && <p className="action-hint">{selectedPlayReason}</p>}
                        </>
                    ) : (
                        <button className="primary" type="button" onClick={onOpenCreate}>Create profile</button>
                    )}
                </div>
            </div>

            <div className="stats-row home-stats">
                <StatusTile label="Profiles" value={profiles.length.toString()}/>
                <StatusTile label="Installed" value={`${installedCount}/${profiles.length}`}/>
                <StatusTile label="Ready to play" value={readyCount.toString()}/>
                <StatusTile label="Running" value={(runningCount + runningServerCount).toString()}/>
                <StatusTile label="Local servers" value={`${installedServerCount}/${localServers.length}`}/>
                <StatusTile label="Account" value={accountLabel(account)}/>
                <StatusTile label="Java" value={javaStatusText(javaStatus, javaPath)}/>
            </div>

            <LocalServerQuickPanel
                servers={localServers}
                form={localServerForm}
                minecraftVersions={minecraftVersions}
                actionKey={localServerActionKey}
                onFormChange={onLocalServerFormChange}
                onCreate={onCreateLocalServer}
            />

            {selectedProfile ? (
                <div className="home-secondary">
                    <ProfileStatusPanel
                        profile={selectedProfile}
                        progress={selectedProgress}
                        launch={selectedLaunch}
                        javaRuntime={selectedJavaRuntime}
                    />
                    <LauncherLogPanel logs={selectedLogs.slice(0, 7)} compact onOpenLogs={onOpenLogs}/>
                </div>
            ) : (
                <EmptyState title="No profiles" action="Create an installation to start playing."/>
            )}

            <HomeInstallationsPanel
                profiles={profiles}
                selectedProfileId={selectedProfile?.id ?? selectedProfileId}
                localServers={localServers}
                selectedLocalServerId={selectedLocalServer?.id ?? selectedLocalServerId}
                installProgress={installProgress}
                localServerProgress={localServerProgress}
                profileJavaRuntimes={profileJavaRuntimes}
                launchStates={launchStates}
                localServerRunStates={localServerRunStates}
                localServerActionKey={localServerActionKey}
                onSelectProfile={onSelectProfile}
                onSelectLocalServer={onSelectLocalServer}
                onInstallProfile={onInstallProfile}
                onRepairProfile={onRepairProfile}
                onLaunchProfile={onLaunchProfile}
                onOpenProfileSettings={onOpenProfileSettings}
                onInstallLocalServer={onInstallLocalServer}
                onRepairLocalServer={onRepairLocalServer}
                onStartLocalServer={onStartLocalServer}
                onStopLocalServer={onStopLocalServer}
                onOpenLocalServerFolder={onOpenLocalServerFolder}
                onOpenLocalServerSettings={onOpenLocalServerSettings}
                onOpenLocalServerTerminal={onOpenLocalServerTerminal}
            />
        </section>
    );
}

function LocalServerQuickPanel({
    servers,
    form,
    minecraftVersions,
    actionKey,
    onFormChange,
    onCreate
}: {
    servers: domain.LocalServer[];
    form: LocalServerForm;
    minecraftVersions: domain.VersionOption[];
    actionKey: string;
    onFormChange: (form: LocalServerForm) => void;
    onCreate: (event: FormEvent<HTMLFormElement>) => void;
}) {
    const creating = actionKey === 'server:create';

    return (
        <section className="dashboard-panel local-server-panel">
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Local server</p>
                    <h2>Create vanilla server</h2>
                </div>
                <span className="server-count">{servers.length} saved</span>
            </div>
            <form className="local-server-form" onSubmit={onCreate}>
                <label>
                    Server name
                    <input
                        value={form.name}
                        onChange={(event) => onFormChange({...form, name: event.target.value})}
                    />
                </label>
                <VersionPicker
                    label="Minecraft version"
                    value={form.minecraftVersion}
                    versions={minecraftVersions}
                    disabled={minecraftVersions.length === 0}
                    onChange={(minecraftVersion) => onFormChange({...form, minecraftVersion})}
                />
                <label>
                    Port
                    <input
                        type="number"
                        min="1"
                        max="65535"
                        value={form.port}
                        onChange={(event) => onFormChange({...form, port: Number(event.target.value)})}
                    />
                </label>
                <label>
                    Min memory MB
                    <input
                        type="number"
                        min="512"
                        step="256"
                        value={form.minMB}
                        onChange={(event) => onFormChange({...form, minMB: Number(event.target.value)})}
                    />
                </label>
                <label>
                    Max memory MB
                    <input
                        type="number"
                        min="512"
                        step="256"
                        value={form.maxMB}
                        onChange={(event) => onFormChange({...form, maxMB: Number(event.target.value)})}
                    />
                </label>
                <label className="wide">
                    Server directory
                    <input
                        value={form.serverDir}
                        placeholder="Default local server directory"
                        onChange={(event) => onFormChange({...form, serverDir: event.target.value})}
                    />
                </label>
                <label className="eula-check wide">
                    <input
                        type="checkbox"
                        checked={form.eulaAccepted}
                        onChange={(event) => onFormChange({...form, eulaAccepted: event.target.checked})}
                    />
                    <span>I accept the Minecraft EULA for this local server.</span>
                </label>
                <div className="local-server-create wide">
                    <button className="primary" type="submit" disabled={creating || !form.eulaAccepted}>
                        {creating ? 'Creating server' : 'Create and install'}
                    </button>
                    {!form.eulaAccepted && <p>Required before writing eula.txt.</p>}
                </div>
            </form>
            <p className="local-server-footnote">
                Created servers live in the Installations list with client profiles.
            </p>
        </section>
    );
}

function HomeInstallationsPanel({
    profiles,
    selectedProfileId,
    localServers,
    selectedLocalServerId,
    installProgress,
    localServerProgress,
    profileJavaRuntimes,
    launchStates,
    localServerRunStates,
    localServerActionKey,
    onSelectProfile,
    onSelectLocalServer,
    onInstallProfile,
    onRepairProfile,
    onLaunchProfile,
    onOpenProfileSettings,
    onInstallLocalServer,
    onRepairLocalServer,
    onStartLocalServer,
    onStopLocalServer,
    onOpenLocalServerFolder,
    onOpenLocalServerSettings,
    onOpenLocalServerTerminal
}: {
    profiles: domain.Profile[];
    selectedProfileId: string;
    localServers: domain.LocalServer[];
    selectedLocalServerId: string;
    installProgress: Record<string, InstallProgress>;
    localServerProgress: Record<string, LocalServerProgress>;
    profileJavaRuntimes: Record<string, domain.ProfileJavaRuntime>;
    launchStates: Record<string, LaunchState>;
    localServerRunStates: Record<string, LocalServerRunState>;
    localServerActionKey: string;
    onSelectProfile: (id: string) => void;
    onSelectLocalServer: (id: string) => void;
    onInstallProfile: (id: string) => void;
    onRepairProfile: (id: string) => void;
    onLaunchProfile: (id: string) => void;
    onOpenProfileSettings: (profile: domain.Profile) => void;
    onInstallLocalServer: (id: string) => void;
    onRepairLocalServer: (id: string) => void;
    onStartLocalServer: (id: string) => void;
    onStopLocalServer: (id: string) => void;
    onOpenLocalServerFolder: (id: string) => void;
    onOpenLocalServerSettings: (server: domain.LocalServer) => void;
    onOpenLocalServerTerminal: (id: string) => void;
}) {
    const [filter, setFilter] = useState<InstallationFilter>('all');
    const visibleProfiles = filter === 'servers' ? [] : profiles;
    const visibleServers = filter === 'clients' ? [] : localServers;
    const totalCount = profiles.length + localServers.length;

    return (
        <section className="dashboard-panel installation-overview">
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Installations</p>
                    <h2>Quick access</h2>
                </div>
                <div className="installation-filter" role="tablist" aria-label="Filter installations">
                    <button className={filter === 'all' ? 'active' : ''} type="button" onClick={() => setFilter('all')}>
                        All {totalCount}
                    </button>
                    <button className={filter === 'clients' ? 'active' : ''} type="button" onClick={() => setFilter('clients')}>
                        Clients {profiles.length}
                    </button>
                    <button className={filter === 'servers' ? 'active' : ''} type="button" onClick={() => setFilter('servers')}>
                        Servers {localServers.length}
                    </button>
                </div>
            </div>
            <div className="home-profile-list">
                {totalCount === 0 && (
                    <div className="local-server-empty">
                        <strong>No installations yet</strong>
                        <p>Create a client profile or a local server to pin it here.</p>
                    </div>
                )}
                {totalCount > 0 && visibleProfiles.length === 0 && visibleServers.length === 0 && (
                    <div className="local-server-empty">
                        <strong>No matches</strong>
                        <p>Switch the filter to see other installation types.</p>
                    </div>
                )}
                {visibleProfiles.map((profile) => {
                    const progress = installProgress[profile.id];
                    const launch = launchStates[profile.id];
                    const javaRuntime = profileJavaRuntimes[profile.id];
                    const playReason = playDisabledReason(profile, launch, javaRuntime);
                    const installing = isInstalling(profile, installProgress);
                    const installVisible = shouldShowInstallButton(profile, progress);
                    const repairVisible = shouldShowRepairButton(profile, progress);
                    const active = profile.id === selectedProfileId;

                    return (
                        <article key={profile.id} className={active ? 'home-profile-row active' : 'home-profile-row'}>
                            <div className="home-profile-info">
                                <div className="installation-title">
                                    <strong>{profile.name}</strong>
                                    <span>Client</span>
                                </div>
                                <span>{profileSubtitle(profile)}</span>
                                <small>
                                    {installStatusText(profile)}
                                    {launch ? ` / ${launchStatusText(launch)}` : ''}
                                    {progress && !progress.done ? ` / ${progressMessage(progress)}` : ''}
                                </small>
                            </div>
                            <div className="home-profile-actions">
                                <button className="small" type="button" disabled={active} onClick={() => onSelectProfile(profile.id)}>
                                    {active ? 'Selected' : 'Select'}
                                </button>
                                {installVisible && (
                                    <button className="small" type="button" disabled={installing} onClick={() => onInstallProfile(profile.id)}>
                                        {installing ? 'Installing' : 'Install'}
                                    </button>
                                )}
                                {repairVisible && (
                                    <button className="small" type="button" disabled={installing} onClick={() => onRepairProfile(profile.id)}>
                                        {installing ? 'Repairing' : 'Repair'}
                                    </button>
                                )}
                                <button className="small primary" type="button" disabled={!!playReason} onClick={() => onLaunchProfile(profile.id)}>
                                    {launch?.status === 'running' || launch?.status === 'starting' ? 'Running' : 'Play'}
                                </button>
                                <button className="small" type="button" onClick={() => onOpenProfileSettings(profile)}>Settings</button>
                            </div>
                        </article>
                    );
                })}
                {visibleServers.map((server) => {
                    const progress = localServerProgress[server.id];
                    const run = localServerRunStates[server.id];
                    const visibleProgress = shouldShowLocalServerProgress(server, progress);
                    const active = server.id === selectedLocalServerId;
                    const installing = isLocalServerInstalling(server, progress);
                    const repairing = server.install?.status === 'repairing';
                    const running = run?.status === 'running' || run?.status === 'starting';
                    const startReason = localServerStartDisabledReason(server, progress, run);
                    const installVisible = shouldShowLocalServerInstallButton(server, progress);
                    const repairVisible = shouldShowLocalServerRepairButton(server, progress);
                    const busy = localServerActionKey.startsWith(`${server.id}:`);

                    return (
                        <article key={`server:${server.id}`} className={active ? 'home-profile-row active' : 'home-profile-row'}>
                            <div className="home-profile-info">
                                <div className="installation-title">
                                    <strong>{server.name}</strong>
                                    <span>Server</span>
                                </div>
                                <span>{localServerSubtitle(server)}</span>
                                <small>{localServerStatusText(server, run)}</small>
                            </div>
                            <div className="home-profile-actions">
                                <button className="small" type="button" disabled={active} onClick={() => onSelectLocalServer(server.id)}>
                                    {active ? 'Selected' : 'Select'}
                                </button>
                                {installVisible && (
                                    <button className="small" type="button" disabled={installing || busy} onClick={() => onInstallLocalServer(server.id)}>
                                        {installing ? 'Installing' : 'Install'}
                                    </button>
                                )}
                                {repairVisible && (
                                    <button className="small" type="button" disabled={installing || busy || running} onClick={() => onRepairLocalServer(server.id)}>
                                        {repairing ? 'Repairing' : 'Repair'}
                                    </button>
                                )}
                                {running ? (
                                    <button className="small danger" type="button" disabled={busy} onClick={() => onStopLocalServer(server.id)}>
                                        Stop
                                    </button>
                                ) : (
                                    <button className="small primary" type="button" disabled={!!startReason || busy} onClick={() => onStartLocalServer(server.id)}>
                                        Start
                                    </button>
                                )}
                                <button className="small" type="button" disabled={busy} onClick={() => onOpenLocalServerFolder(server.id)}>
                                    Folder
                                </button>
                                <button className="small" type="button" onClick={() => onOpenLocalServerSettings(server)}>
                                    Settings
                                </button>
                                <button className="small" type="button" disabled={busy} onClick={() => onOpenLocalServerTerminal(server.id)}>
                                    Terminal
                                </button>
                            </div>
                            {visibleProgress && (
                                <div className="local-server-detail">
                                    <ProgressBar progress={progress}/>
                                    <p>{localServerProgressMessage(progress)}</p>
                                </div>
                            )}
                        </article>
                    );
                })}
            </div>
        </section>
    );
}

function LocalServerDetail({
    server,
    progress,
    run,
    actionKey,
    onInstall,
    onRepair,
    onStart,
    onStop,
    onOpenFolder,
    onOpenSettings,
    onOpenTerminal
}: {
    server?: domain.LocalServer;
    progress?: LocalServerProgress;
    run?: LocalServerRunState;
    actionKey: string;
    onInstall: (id: string) => void;
    onRepair: (id: string) => void;
    onStart: (id: string) => void;
    onStop: (id: string) => void;
    onOpenFolder: (id: string) => void;
    onOpenSettings: (server: domain.LocalServer) => void;
    onOpenTerminal: (id: string) => void;
}) {
    if (!server) {
        return <EmptyState title="Select an installation" action="No client profile or server selected."/>;
    }

    const installing = isLocalServerInstalling(server, progress);
    const repairing = server.install?.status === 'repairing';
    const running = run?.status === 'running' || run?.status === 'starting';
    const startReason = localServerStartDisabledReason(server, progress, run);
    const installVisible = shouldShowLocalServerInstallButton(server, progress);
    const repairVisible = shouldShowLocalServerRepairButton(server, progress);
    const busy = actionKey.startsWith(`${server.id}:`);
    const visibleProgress = shouldShowLocalServerProgress(server, progress);

    return (
        <section className="profile-detail">
            <div>
                <p className="eyebrow">Server</p>
                <h2>{server.name}</h2>
                <p>{localServerSubtitle(server)}</p>
            </div>
            <div className="install-status">
                <div>
                    <span>Server status</span>
                    <strong>{localServerInstallStatusText(server)}</strong>
                    {server.install?.lastError && <p>{server.install.lastError}</p>}
                    {visibleProgress && <p>{localServerProgressMessage(progress)}</p>}
                    {run && <p>{localServerRunStatusText(run)}</p>}
                </div>
                {visibleProgress && <ProgressBar progress={progress}/>}
            </div>
            <dl className="detail-grid compact">
                <div>
                    <dt>Minecraft</dt>
                    <dd>{server.minecraftVersion}</dd>
                </div>
                <div>
                    <dt>Port</dt>
                    <dd>{server.port}</dd>
                </div>
                <div>
                    <dt>Memory</dt>
                    <dd>{server.memory.minMB} - {server.memory.maxMB} MB</dd>
                </div>
                <div>
                    <dt>EULA</dt>
                    <dd>{server.eulaAccepted ? 'Accepted' : 'Not accepted'}</dd>
                </div>
            </dl>
            <div className="install-status">
                <div>
                    <span>Server directory</span>
                    <p>{server.serverDir || 'Default local server directory'}</p>
                </div>
            </div>
            <div className="actions server-actions">
                {installVisible && (
                    <button className="install-action" type="button" disabled={installing || busy} onClick={() => onInstall(server.id)}>
                        {installing ? 'Installing' : 'Install'}
                    </button>
                )}
                {repairVisible && (
                    <button className="install-action" type="button" disabled={installing || busy || running} onClick={() => onRepair(server.id)}>
                        {repairing ? 'Repairing' : 'Repair'}
                    </button>
                )}
                {running ? (
                    <button className="danger server-run-action" type="button" disabled={busy} onClick={() => onStop(server.id)}>
                        Stop
                    </button>
                ) : (
                    <button className="primary server-run-action" type="button" disabled={!!startReason || busy} onClick={() => onStart(server.id)}>
                        Start
                    </button>
                )}
                <button type="button" disabled={busy} onClick={() => onOpenFolder(server.id)}>Folder</button>
                <button type="button" onClick={() => onOpenSettings(server)}>Settings</button>
                <button type="button" disabled={busy} onClick={() => onOpenTerminal(server.id)}>Terminal</button>
                {startReason && <p className="action-hint">{startReason}</p>}
            </div>
        </section>
    );
}

function ProfileDetail({
    profile,
    progress,
    launch,
    javaRuntime,
    javaInstallProgress,
    modList,
    modrinthUpdatePlans,
    modActionKey,
    settingsOpen,
    settingsDraft,
    onDelete,
    onInstall,
    onRepair,
    onInstallJava,
    onLaunch,
    onOpenSettings,
    onCloseSettings,
    onSettingsDraftChange,
    onSaveSettings,
    onRefreshMods,
    onImportMod,
    onExportModpack,
    onOpenModsFolder,
    onCheckModrinthUpdates,
    onUpdateModrinthProject,
    onUpdateModrinthFile,
    onBrowseMod,
    onToggleMod,
    onBulkToggleMods,
    onDeleteMod
}: {
    profile?: domain.Profile;
    progress?: InstallProgress;
    launch?: LaunchState;
    javaRuntime?: domain.ProfileJavaRuntime;
    javaInstallProgress: JavaInstallProgress | null;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    modActionKey: string;
    settingsOpen: boolean;
    settingsDraft: ProfileSettingsDraft | null;
    onDelete: (id: string) => void;
    onInstall: (id: string) => void;
    onRepair: (id: string) => void;
    onInstallJava: (version: number) => void;
    onLaunch: (id: string) => void;
    onOpenSettings: (profile: domain.Profile) => void;
    onCloseSettings: () => void;
    onSettingsDraftChange: (draft: ProfileSettingsDraft) => void;
    onSaveSettings: (event: FormEvent<HTMLFormElement>) => void;
    onRefreshMods: (profileId: string) => void;
    onImportMod: (profileId: string) => void;
    onExportModpack: (profileId: string) => void;
    onOpenModsFolder: (profileId: string) => void;
    onCheckModrinthUpdates: (profileId: string) => void;
    onUpdateModrinthProject: (profileId: string, projectId: string) => void;
    onUpdateModrinthFile: (profileId: string, fileName: string) => void;
    onBrowseMod: (profileId: string, projectId: string, query: string) => void;
    onToggleMod: (profileId: string, fileName: string, enabled: boolean) => void;
    onBulkToggleMods: (profileId: string, fileNames: string[], enabled: boolean) => void;
    onDeleteMod: (profileId: string, fileName: string) => void;
}) {
    if (!profile) {
        return <EmptyState title="Select a profile" action="No profile selected."/>;
    }

    const installing = isInstalling(profile, {[profile.id]: progress});
    const installVisible = shouldShowInstallButton(profile, progress);
    const repairVisible = shouldShowRepairButton(profile, progress);
    const playReason = playDisabledReason(profile, launch, javaRuntime);
    const showJavaNotice = shouldShowJavaRuntimeNotice(javaRuntime, javaInstallProgress);

    return (
        <section className="profile-detail">
            <div>
                <p className="eyebrow">Profile</p>
                <h2>{profile.name}</h2>
                <p>{profileSubtitle(profile)}</p>
            </div>
            <div className="install-status">
                <div>
                    <span>Install status</span>
                    <strong>{installStatusText(profile)}</strong>
                    {profile.install?.lastError && <p>{profile.install.lastError}</p>}
                    {progress?.message && <p>{progressMessage(progress)}</p>}
                    {launch && <p>{launchStatusText(launch)}</p>}
                </div>
                <ProgressBar progress={progress}/>
            </div>
            {showJavaNotice && (
                <JavaRuntimePanel
                    runtime={javaRuntime}
                    progress={javaInstallProgress}
                    onInstallJava={onInstallJava}
                />
            )}
            <dl className="detail-grid compact">
                <div>
                    <dt>Minecraft</dt>
                    <dd>{profile.minecraftVersion}</dd>
                </div>
                <div>
                    <dt>Loader</dt>
                    <dd>{profile.loader.type}{profile.loader.version ? ` ${profile.loader.version}` : ''}</dd>
                </div>
            </dl>
            <div className="actions">
                <button
                    className="primary"
                    type="button"
                    disabled={!!playReason}
                    onClick={() => onLaunch(profile.id)}
                >
                    {launch?.status === 'running' || launch?.status === 'starting' ? 'Running' : 'Play'}
                </button>
                {installVisible && (
                    <button type="button" disabled={installing} onClick={() => onInstall(profile.id)}>
                        {installing ? 'Installing' : 'Install'}
                    </button>
                )}
                {repairVisible && (
                    <button type="button" disabled={installing} onClick={() => onRepair(profile.id)}>
                        {installing ? 'Repairing' : 'Repair'}
                    </button>
                )}
                <button
                    type="button"
                    onClick={() => settingsOpen ? onCloseSettings() : onOpenSettings(profile)}
                >
                    {settingsOpen ? 'Close settings' : 'Settings'}
                </button>
                <button className="danger" type="button" onClick={() => onDelete(profile.id)}>Delete</button>
                {playReason && <p className="action-hint">{playReason}</p>}
            </div>
        </section>
    );
}

function ProfileSettingsDialog({
    profile,
    draft,
    javaRuntime,
    javaInstallProgress,
    modList,
    modrinthUpdatePlans,
    modActionKey,
    onClose,
    onDraftChange,
    onInstallJava,
    onSave,
    onRefreshMods,
    onImportMod,
    onExportModpack,
    onOpenModsFolder,
    onCheckModrinthUpdates,
    onUpdateModrinthProject,
    onUpdateModrinthFile,
    onBrowseMod,
    onToggleMod,
    onBulkToggleMods,
    onDeleteMod
}: {
    profile: domain.Profile;
    draft: ProfileSettingsDraft;
    javaRuntime?: domain.ProfileJavaRuntime;
    javaInstallProgress: JavaInstallProgress | null;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    modActionKey: string;
    onClose: () => void;
    onDraftChange: (draft: ProfileSettingsDraft) => void;
    onInstallJava: (version: number) => void;
    onSave: (event: FormEvent<HTMLFormElement>) => void;
    onRefreshMods: (profileId: string) => void;
    onImportMod: (profileId: string) => void;
    onExportModpack: (profileId: string) => void;
    onOpenModsFolder: (profileId: string) => void;
    onCheckModrinthUpdates: (profileId: string) => void;
    onUpdateModrinthProject: (profileId: string, projectId: string) => void;
    onUpdateModrinthFile: (profileId: string, fileName: string) => void;
    onBrowseMod: (profileId: string, projectId: string, query: string) => void;
    onToggleMod: (profileId: string, fileName: string, enabled: boolean) => void;
    onBulkToggleMods: (profileId: string, fileNames: string[], enabled: boolean) => void;
    onDeleteMod: (profileId: string, fileName: string) => void;
}) {
    return (
        <div className="modal-backdrop" role="presentation">
            <section className="confirm-dialog settings-dialog" role="dialog" aria-modal="true" aria-labelledby="profile-settings-title">
                <div className="settings-dialog-top">
                    <div>
                        <p className="eyebrow">Client settings</p>
                        <h2 id="profile-settings-title">{profile.name}</h2>
                    </div>
                    <button type="button" onClick={onClose}>Close</button>
                </div>
                <ProfileSettingsPanel
                    profile={profile}
                    draft={draft}
                    javaRuntime={javaRuntime}
                    javaInstallProgress={javaInstallProgress}
                    modList={modList}
                    modrinthUpdatePlans={modrinthUpdatePlans}
                    modActionKey={modActionKey}
                    onDraftChange={onDraftChange}
                    onInstallJava={onInstallJava}
                    onSave={onSave}
                    onRefreshMods={onRefreshMods}
                    onImportMod={onImportMod}
                    onExportModpack={onExportModpack}
                    onOpenModsFolder={onOpenModsFolder}
                    onCheckModrinthUpdates={onCheckModrinthUpdates}
                    onUpdateModrinthProject={onUpdateModrinthProject}
                    onUpdateModrinthFile={onUpdateModrinthFile}
                    onBrowseMod={onBrowseMod}
                    onToggleMod={onToggleMod}
                    onBulkToggleMods={onBulkToggleMods}
                    onDeleteMod={onDeleteMod}
                />
            </section>
        </div>
    );
}

function LocalServerSettingsDialog({
    server,
    progress,
    run,
    actionKey,
    onClose,
    onInstall,
    onRepair,
    onStart,
    onStop,
    onOpenFolder,
    onOpenRawSettings,
    onOpenTerminal
}: {
    server: domain.LocalServer;
    progress?: LocalServerProgress;
    run?: LocalServerRunState;
    actionKey: string;
    onClose: () => void;
    onInstall: (id: string) => void;
    onRepair: (id: string) => void;
    onStart: (id: string) => void;
    onStop: (id: string) => void;
    onOpenFolder: (id: string) => void;
    onOpenRawSettings: (id: string) => void;
    onOpenTerminal: (id: string) => void;
}) {
    const installing = isLocalServerInstalling(server, progress);
    const repairing = server.install?.status === 'repairing';
    const running = run?.status === 'running' || run?.status === 'starting';
    const startReason = localServerStartDisabledReason(server, progress, run);
    const installVisible = shouldShowLocalServerInstallButton(server, progress);
    const repairVisible = shouldShowLocalServerRepairButton(server, progress);
    const busy = actionKey.startsWith(`${server.id}:`);
    const rawSettingsDisabled = busy || server.install?.status !== 'installed';
    const visibleProgress = shouldShowLocalServerProgress(server, progress);

    return (
        <div className="modal-backdrop" role="presentation">
            <section className="confirm-dialog settings-dialog" role="dialog" aria-modal="true" aria-labelledby="server-settings-title">
                <div className="settings-dialog-top">
                    <div>
                        <p className="eyebrow">Server settings</p>
                        <h2 id="server-settings-title">{server.name}</h2>
                    </div>
                    <button type="button" onClick={onClose}>Close</button>
                </div>
                <div className="server-settings-panel">
                    <div className="settings-section wide">
                        <div>
                            <p className="eyebrow">Basic</p>
                            <h3>Runtime summary</h3>
                        </div>
                        <dl className="detail-grid settings-details">
                            <div>
                                <dt>Minecraft</dt>
                                <dd>{server.minecraftVersion}</dd>
                            </div>
                            <div>
                                <dt>Port</dt>
                                <dd>{server.port}</dd>
                            </div>
                            <div>
                                <dt>Memory</dt>
                                <dd>{server.memory.minMB} - {server.memory.maxMB} MB</dd>
                            </div>
                            <div>
                                <dt>EULA</dt>
                                <dd>{server.eulaAccepted ? 'Accepted' : 'Not accepted'}</dd>
                            </div>
                            <div>
                                <dt>Install status</dt>
                                <dd>{localServerInstallStatusText(server)}</dd>
                            </div>
                            <div>
                                <dt>Server ID</dt>
                                <dd>{server.id}</dd>
                            </div>
                        </dl>
                    </div>
                    <label className="wide">
                        Server directory
                        <input value={server.serverDir} disabled readOnly/>
                    </label>
                    <div className="settings-section wide">
                        <div>
                            <p className="eyebrow">Advanced</p>
                            <h3>server.properties</h3>
                        </div>
                        <p className="muted">
                            Inline editing is next; for now this opens the raw settings file without losing unknown keys.
                        </p>
                    </div>
                    {(visibleProgress || run || startReason) && (
                        <div className="local-server-detail wide">
                            {visibleProgress && <ProgressBar progress={progress}/>}
                            {run && <p>{localServerRunStatusText(run)}</p>}
                            {startReason && <p>{startReason}</p>}
                        </div>
                    )}
                    <div className="settings-dialog-actions wide">
                        {installVisible && (
                            <button type="button" disabled={installing || busy} onClick={() => onInstall(server.id)}>
                                {installing ? 'Installing' : 'Install'}
                            </button>
                        )}
                        {repairVisible && (
                            <button type="button" disabled={installing || busy || running} onClick={() => onRepair(server.id)}>
                                {repairing ? 'Repairing' : 'Repair'}
                            </button>
                        )}
                        {running ? (
                            <button className="danger" type="button" disabled={busy} onClick={() => onStop(server.id)}>
                                Stop
                            </button>
                        ) : (
                            <button className="primary" type="button" disabled={!!startReason || busy} onClick={() => onStart(server.id)}>
                                Start
                            </button>
                        )}
                        <button type="button" disabled={busy} onClick={() => onOpenFolder(server.id)}>Folder</button>
                        <button type="button" disabled={rawSettingsDisabled} onClick={() => onOpenRawSettings(server.id)}>
                            Open raw settings file
                        </button>
                        <button type="button" disabled={busy} onClick={() => onOpenTerminal(server.id)}>Terminal</button>
                    </div>
                </div>
            </section>
        </div>
    );
}

function BrowsePanel({
    profiles,
    profile,
    query,
    results,
    loading,
    detailsProject,
    detailsLoading,
    installingProjectId,
    deletingProjectId,
    updatingProjectId,
    installedProjects,
    updatePlans,
    projectVersions,
    versionsLoadingKey,
    onProfileChange,
    onQueryChange,
    onSearch,
    onOpenDetails,
    onLoadVersions,
    onBackToResults,
    onInstall,
    onInstallVersion,
    onUpdate,
    onUpdateVersion,
    onDelete
}: {
    profiles: domain.Profile[];
    profile?: domain.Profile;
    query: string;
    results: domain.ModrinthSearchResult | null;
    loading: boolean;
    detailsProject: domain.ModrinthProject | null;
    detailsLoading: boolean;
    installingProjectId: string;
    deletingProjectId: string;
    updatingProjectId: string;
    installedProjects: Record<string, boolean>;
    updatePlans: domain.ModrinthUpdatePlan[];
    projectVersions: Record<string, domain.ModrinthVersion[]>;
    versionsLoadingKey: string;
    onProfileChange: (profileId: string) => void;
    onQueryChange: (query: string) => void;
    onSearch: (event?: FormEvent<HTMLFormElement>) => void;
    onOpenDetails: (projectId: string) => void;
    onLoadVersions: (profileId: string, projectId: string) => void;
    onBackToResults: () => void;
    onInstall: (projectId: string) => void;
    onInstallVersion: (projectId: string, versionId: string) => void;
    onUpdate: (projectId: string) => void;
    onUpdateVersion: (projectId: string, versionId: string) => void;
    onDelete: (projectId: string) => void;
}) {
    const canBrowse = !!profile && (profile.loader.type === 'fabric' || profile.loader.type === 'quilt' || profile.loader.type === 'forge' || profile.loader.type === 'neoforge');
    const hits = results?.hits ?? [];
    const updateByProjectId = useMemo(() => modrinthUpdatePlansByProject(updatePlans), [updatePlans]);
    const isInstalled = (projectId: string) => !!profile && (
        !!installedProjects[`${profile.id}:${projectId}`] ||
        !!updateByProjectId[projectId]
    );

    return (
        <section className="browse-panel">
            <form className="browse-search" onSubmit={onSearch}>
                <div>
                    <p className="eyebrow">Modrinth</p>
                    <h2>{profile ? profile.name : 'No profile selected'}</h2>
                    <p>{profile ? profileSubtitle(profile) : 'Select a profile in Library.'}</p>
                </div>
                <label className="browse-profile-picker">
                    Installation
                    <select
                        value={profile?.id ?? ''}
                        disabled={loading || !!installingProjectId || !!deletingProjectId}
                        onChange={(event) => onProfileChange(event.target.value)}
                    >
                        {profiles.length === 0 && <option value="">No profiles</option>}
                        {profiles.map((option) => (
                            <option key={option.id} value={option.id}>
                                {option.name} - {profileSubtitle(option)}
                            </option>
                        ))}
                    </select>
                </label>
                <div className="browse-controls">
                    <input
                        value={query}
                        placeholder="Search mods"
                        disabled={loading}
                        onChange={(event) => onQueryChange(event.target.value)}
                    />
                    <button className="primary" type="submit" disabled={!canBrowse || loading}>
                        {loading ? 'Searching' : 'Search'}
                    </button>
                </div>
            </form>
            {profile && !canBrowse && (
                <p className="mod-note">Modrinth install requires Fabric, Quilt, Forge, or NeoForge profiles.</p>
            )}
            {results && (
                <div className="browse-summary">
                    <span>{results.totalHits} results</span>
                    <span>{results.minecraftVersion}</span>
                    <span>{results.loader}</span>
                </div>
            )}
            {detailsLoading && <p className="muted">Loading project details.</p>}
            {detailsProject && (
                <ModrinthProjectDetails
                    profile={profile}
                    project={detailsProject}
                    installing={modrinthProjectBusy(installingProjectId, detailsProject.projectId)}
                    deleting={deletingProjectId === detailsProject.projectId}
                    updating={modrinthProjectBusy(updatingProjectId, detailsProject.projectId)}
                    installActionKey={installingProjectId}
                    updateActionKey={updatingProjectId}
                    installed={isInstalled(detailsProject.projectId)}
                    updatePlan={updateByProjectId[detailsProject.projectId]}
                    versions={profile ? projectVersions[modrinthProjectProfileKey(profile.id, detailsProject.projectId)] ?? [] : []}
                    versionsLoading={!!profile && versionsLoadingKey === modrinthProjectProfileKey(profile.id, detailsProject.projectId)}
                    canInstall={canBrowse && !installingProjectId}
                    onBack={onBackToResults}
                    onLoadVersions={onLoadVersions}
                    onInstall={onInstall}
                    onInstallVersion={onInstallVersion}
                    onUpdate={onUpdate}
                    onUpdateVersion={onUpdateVersion}
                    onDelete={onDelete}
                />
            )}
            <div className="browse-results">
                {detailsProject ? null : hits.length === 0 && !loading && (
                    <p className="muted">{results ? 'No compatible mods found.' : 'No search results yet.'}</p>
                )}
                {!detailsProject && hits.map((project) => (
                    <article key={project.projectId} className="browse-result">
                        {(() => {
                            const installed = isInstalled(project.projectId);
                            const updatePlan = updateByProjectId[project.projectId];
                            const updateAvailable = !!updatePlan?.updateAvailable;
                            return (
                                <>
                        {project.iconUrl ? (
                            <img src={project.iconUrl} alt="" loading="lazy"/>
                        ) : (
                            <div className="browse-icon-fallback">{project.title.slice(0, 1).toUpperCase()}</div>
                        )}
                        <div className="browse-result-main">
                            <h3>{project.title}</h3>
                            <p>{project.description}</p>
                            <div className="browse-tags">
                                <span>{formatNumber(project.downloads)} downloads</span>
                                {project.author && <span>{project.author}</span>}
                                {project.displayVersion && <span>{project.displayVersion}</span>}
                            </div>
                        </div>
                        <div className="browse-result-actions">
                            <button
                                type="button"
                                disabled={detailsLoading}
                                onClick={() => onOpenDetails(project.projectId)}
                            >
                                Details
                            </button>
                            <button
                                className="primary"
                                type="button"
                                disabled={!canBrowse || !!installingProjectId || !!deletingProjectId || !!updatingProjectId || (installed && !updateAvailable)}
                                onClick={() => onInstall(project.projectId)}
                            >
                                {installed ? 'Installed' : installingProjectId === project.projectId ? 'Installing' : 'Install'}
                            </button>
                            {installed && updateAvailable && (
                                <button
                                    className="primary"
                                    type="button"
                                    disabled={!!installingProjectId || !!deletingProjectId || !!updatingProjectId}
                                    onClick={() => onUpdate(project.projectId)}
                                >
                                    {updatingProjectId === project.projectId ? 'Updating' : 'Update'}
                                </button>
                            )}
                            {installed && (
                                <button
                                    className="danger"
                                    type="button"
                                    disabled={!!installingProjectId || !!deletingProjectId || !!updatingProjectId}
                                    onClick={() => onDelete(project.projectId)}
                                >
                                    {deletingProjectId === project.projectId ? 'Deleting' : 'Delete'}
                                </button>
                            )}
                        </div>
                                </>
                            );
                        })()}
                    </article>
                ))}
            </div>
        </section>
    );
}

function ModrinthDependencyConfirmDialog({
    plan,
    confirming,
    onCancel,
    onConfirm
}: {
    plan: domain.ModrinthInstallPlan;
    confirming: boolean;
    onCancel: () => void;
    onConfirm: (dependencyIDs: string[]) => void;
}) {
    const dependencies = plan.requiredDependencies ?? [];
    const [selectedDependencies, setSelectedDependencies] = useState<Record<string, boolean>>(() => selectedInstallDependencies(dependencies));
    const selectedCount = dependencies.filter((dependency) => selectedDependencies[installDependencyKey(dependency)]).length;
    const missingCount = dependencies.filter((dependency) => !dependency.alreadyPresent && selectedDependencies[installDependencyKey(dependency)]).length;

    function toggleDependency(dependency: domain.ModrinthRequiredDependency) {
        const key = installDependencyKey(dependency);
        setSelectedDependencies((current) => ({...current, [key]: !current[key]}));
    }

    function selectedDependencyIDs() {
        return dependencies
            .filter((dependency) => selectedDependencies[installDependencyKey(dependency)])
            .map((dependency) => installDependencyKey(dependency));
    }

    return (
        <div className="modal-backdrop" role="presentation">
            <section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="modrinth-dependencies-title">
                <div>
                    <p className="eyebrow">Required dependencies</p>
                    <h2 id="modrinth-dependencies-title">Install dependencies?</h2>
                    <p>
                        Вы пытаетесь установить мод <strong>{plan.projectTitle || plan.fileName}</strong>,
                        у которого есть обязательные зависимости.
                    </p>
                </div>
                <div className="dependency-confirm-list">
                    {dependencies.map((dependency) => (
                        <label key={`${dependency.projectId}-${dependency.versionId}`} className="dependency-confirm-row selectable">
                            <input
                                type="checkbox"
                                checked={!!selectedDependencies[installDependencyKey(dependency)]}
                                disabled={confirming}
                                onChange={() => toggleDependency(dependency)}
                            />
                            <div>
                                <strong>{dependency.projectTitle || dependency.displayName || dependency.fileName}</strong>
                                <p>{dependency.versionNumber || dependency.versionName || dependency.fileName}</p>
                            </div>
                            {dependency.alreadyPresent && <span>already installed</span>}
                        </label>
                    ))}
                </div>
                <p className="muted">
                    {missingCount > 0
                        ? `Если вы согласны, выбранные недостающие зависимости скачаются автоматически. Выбрано: ${selectedCount}.`
                        : selectedCount > 0 ? 'Все выбранные required-зависимости уже есть в папке mods.' : 'Required-зависимости не выбраны.'}
                </p>
                <div className="confirm-actions">
                    <button type="button" disabled={confirming} onClick={onCancel}>Cancel</button>
                    <button className="primary" type="button" disabled={confirming} onClick={() => onConfirm(selectedDependencyIDs())}>
                        {confirming ? 'Installing' : 'Install automatically'}
                    </button>
                </div>
            </section>
        </div>
    );
}

function ModrinthDeleteConfirmDialog({
    plan,
    deleting,
    onCancel,
    onConfirm
}: {
    plan: domain.ModrinthDeletePlan;
    deleting: boolean;
    onCancel: () => void;
    onConfirm: (fileNames: string[]) => void;
}) {
    const files = plan.files ?? [];
    const skipped = plan.skippedFiles ?? [];
    const dependencyCount = files.filter((file) => !!file.dependencyType).length;
    const [selectedFiles, setSelectedFiles] = useState<Record<string, boolean>>(() => selectedDeleteFiles(files));
    const selectedCount = files.filter((file) => selectedFiles[file.fileName]).length;

    function toggleFile(fileName: string) {
        setSelectedFiles((current) => ({...current, [fileName]: !current[fileName]}));
    }

    function selectedFileNames() {
        return files.filter((file) => selectedFiles[file.fileName]).map((file) => file.fileName);
    }

    return (
        <div className="modal-backdrop" role="presentation">
            <section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="modrinth-delete-title">
                <div>
                    <p className="eyebrow">Delete from Browse</p>
                    <h2 id="modrinth-delete-title">Delete mod and dependencies?</h2>
                    <p>
                        Вы пытаетесь удалить мод <strong>{plan.projectTitle || plan.projectId}</strong>
                        {dependencyCount > 0 ? ' вместе с зависимостями.' : '.'}
                    </p>
                    {!plan.tracked && (
                        <p className="muted">
                            This install was not tracked by Power Mine, so the launcher built this plan from current Modrinth metadata.
                        </p>
                    )}
                </div>
                {files.length > 0 && (
                    <div className="dependency-confirm-list">
                        {files.map((file) => (
                            <label key={`delete-${file.projectId}-${file.fileName}`} className="dependency-confirm-row selectable">
                                <input
                                    type="checkbox"
                                    checked={!!selectedFiles[file.fileName]}
                                    disabled={deleting}
                                    onChange={() => toggleFile(file.fileName)}
                                />
                                <div>
                                    <strong>{file.displayName || file.projectTitle || file.fileName}</strong>
                                    <p>{file.dependencyType ? `required dependency / ${file.fileName}` : file.fileName}</p>
                                </div>
                                <span>will be deleted</span>
                            </label>
                        ))}
                    </div>
                )}
                {skipped.length > 0 && (
                    <div className="dependency-confirm-list">
                        {skipped.map((file) => (
                            <div key={`skip-${file.projectId}-${file.fileName}`} className="dependency-confirm-row">
                                <div>
                                    <strong>{file.displayName || file.projectTitle || file.fileName}</strong>
                                    <p>{file.reason || 'will be kept'}</p>
                                </div>
                                <span>kept</span>
                            </div>
                        ))}
                    </div>
                )}
                <p className="muted">
                    {selectedCount > 0
                        ? `Если вы согласны, выбранные файлы будут удалены из папки mods. Выбрано: ${selectedCount}.`
                        : 'Нет файлов для удаления.'}
                </p>
                <div className="confirm-actions">
                    <button type="button" disabled={deleting} onClick={onCancel}>Cancel</button>
                    <button className="danger" type="button" disabled={deleting || selectedCount === 0} onClick={() => onConfirm(selectedFileNames())}>
                        {deleting ? 'Deleting' : 'Delete listed files'}
                    </button>
                </div>
            </section>
        </div>
    );
}

function ModrinthUpdateConfirmDialog({
    plan,
    updating,
    onCancel,
    onConfirm
}: {
    plan: domain.ModrinthUpdatePlan;
    updating: boolean;
    onCancel: () => void;
    onConfirm: (dependencyIDs: string[]) => void;
}) {
    const dependencies = plan.requiredDependencies ?? [];
    const [selectedDependencies, setSelectedDependencies] = useState<Record<string, boolean>>(() => selectedInstallDependencies(dependencies));
    const selectedCount = dependencies.filter((dependency) => selectedDependencies[installDependencyKey(dependency)]).length;
    const missingCount = dependencies.filter((dependency) => !dependency.alreadyPresent && selectedDependencies[installDependencyKey(dependency)]).length;

    function toggleDependency(dependency: domain.ModrinthRequiredDependency) {
        const key = installDependencyKey(dependency);
        setSelectedDependencies((current) => ({...current, [key]: !current[key]}));
    }

    function selectedDependencyIDs() {
        return dependencies
            .filter((dependency) => selectedDependencies[installDependencyKey(dependency)])
            .map((dependency) => installDependencyKey(dependency));
    }

    return (
        <div className="modal-backdrop" role="presentation">
            <section className="confirm-dialog" role="dialog" aria-modal="true" aria-labelledby="modrinth-update-title">
                <div>
                    <p className="eyebrow">Update dependencies</p>
                    <h2 id="modrinth-update-title">Update mod and dependencies?</h2>
                    <p>
                        Вы обновляете <strong>{plan.projectTitle || plan.projectId}</strong>
                        {plan.currentVersionNumber && plan.latestVersionNumber ? ` с ${plan.currentVersionNumber} до ${plan.latestVersionNumber}` : ''}.
                    </p>
                </div>
                <div className="dependency-confirm-list">
                    {dependencies.map((dependency) => (
                        <label key={`${dependency.projectId}-${dependency.versionId}`} className="dependency-confirm-row selectable">
                            <input
                                type="checkbox"
                                checked={!!selectedDependencies[installDependencyKey(dependency)]}
                                disabled={updating}
                                onChange={() => toggleDependency(dependency)}
                            />
                            <div>
                                <strong>{dependency.projectTitle || dependency.displayName || dependency.fileName}</strong>
                                <p>{dependency.versionNumber || dependency.versionName || dependency.fileName}</p>
                            </div>
                            {dependency.alreadyPresent && <span>already installed</span>}
                        </label>
                    ))}
                </div>
                <p className="muted">
                    {missingCount > 0
                        ? `Если вы согласны, выбранные недостающие зависимости скачаются автоматически. Выбрано: ${selectedCount}.`
                        : selectedCount > 0 ? 'Все выбранные required-зависимости уже есть в папке mods.' : 'Required-зависимости не выбраны.'}
                </p>
                <div className="confirm-actions">
                    <button type="button" disabled={updating} onClick={onCancel}>Cancel</button>
                    <button className="primary" type="button" disabled={updating} onClick={() => onConfirm(selectedDependencyIDs())}>
                        {updating ? 'Updating' : 'Update'}
                    </button>
                </div>
            </section>
        </div>
    );
}

function ModrinthProjectDetails({
    profile,
    project,
    installing,
    deleting,
    updating,
    installActionKey,
    updateActionKey,
    installed,
    updatePlan,
    versions,
    versionsLoading,
    canInstall,
    onBack,
    onLoadVersions,
    onInstall,
    onInstallVersion,
    onUpdate,
    onUpdateVersion,
    onDelete
}: {
    profile?: domain.Profile;
    project: domain.ModrinthProject;
    installing: boolean;
    deleting: boolean;
    updating: boolean;
    installActionKey: string;
    updateActionKey: string;
    installed: boolean;
    updatePlan?: domain.ModrinthUpdatePlan;
    versions: domain.ModrinthVersion[];
    versionsLoading: boolean;
    canInstall: boolean;
    onBack: () => void;
    onLoadVersions: (profileId: string, projectId: string) => void;
    onInstall: (projectId: string) => void;
    onInstallVersion: (projectId: string, versionId: string) => void;
    onUpdate: (projectId: string) => void;
    onUpdateVersion: (projectId: string, versionId: string) => void;
    onDelete: (projectId: string) => void;
}) {
    const links = [
        {label: 'Source', value: project.sourceUrl},
        {label: 'Issues', value: project.issuesUrl},
        {label: 'Wiki', value: project.wikiUrl},
        {label: 'Discord', value: project.discordUrl},
    ].filter((link) => link.value);
    const followers = project.followers ?? 0;
    const loaders = project.loaders ?? [];
    const gameVersions = project.gameVersions ?? [];
    const categories = project.categories ?? [];
    const updateAvailable = !!updatePlan?.updateAvailable;
    const [tab, setTab] = useState<'description' | 'versions'>('description');
    const [requestedVersionsKey, setRequestedVersionsKey] = useState('');
    const versionsKey = profile ? modrinthProjectProfileKey(profile.id, project.projectId) : '';
    const projectBusy = modrinthProjectBusy(installActionKey, project.projectId) || modrinthProjectBusy(updateActionKey, project.projectId) || deleting;

    useEffect(() => {
        if (tab === 'versions' && profile && versions.length === 0 && !versionsLoading && requestedVersionsKey !== versionsKey) {
            setRequestedVersionsKey(versionsKey);
            onLoadVersions(profile.id, project.projectId);
        }
    }, [tab, profile?.id, project.projectId, versions.length, versionsLoading, requestedVersionsKey, versionsKey, onLoadVersions]);

    return (
        <article className="browse-detail">
            <div className="browse-detail-header">
                {project.iconUrl ? (
                    <img src={project.iconUrl} alt="" loading="lazy"/>
                ) : (
                    <div className="browse-icon-fallback">{project.title.slice(0, 1).toUpperCase()}</div>
                )}
                <div>
                    <p className="eyebrow">Modrinth project</p>
                    <h2>{project.title}</h2>
                    <p>{project.description}</p>
                </div>
                <div className="browse-detail-actions">
                    <button type="button" onClick={onBack}>Back</button>
                    <button
                        className="primary"
                        type="button"
                        disabled={!profile || !canInstall || (installed && !updateAvailable)}
                        onClick={() => onInstall(project.projectId)}
                    >
                        {installed ? 'Installed' : installing ? 'Installing' : 'Install'}
                    </button>
                    {installed && updateAvailable && (
                        <button
                            className="primary"
                            type="button"
                            disabled={updating || deleting || installing}
                            onClick={() => onUpdate(project.projectId)}
                        >
                            {updating ? 'Updating' : 'Update'}
                        </button>
                    )}
                    {installed && (
                        <button
                            className="danger"
                            type="button"
                            disabled={deleting || installing || updating}
                            onClick={() => onDelete(project.projectId)}
                        >
                            {deleting ? 'Deleting' : 'Delete'}
                        </button>
                    )}
                </div>
            </div>
            <div className="browse-detail-stats">
                <span>{formatNumber(project.downloads)} downloads</span>
                {followers > 0 && <span>{formatNumber(followers)} followers</span>}
                {project.licenseName && <span>{project.licenseName}</span>}
                {project.clientSide && <span>Client: {project.clientSide}</span>}
                {project.serverSide && <span>Server: {project.serverSide}</span>}
            </div>
            {(loaders.length > 0 || gameVersions.length > 0 || categories.length > 0) && (
                <div className="browse-detail-tags">
                    {loaders.slice(0, 6).map((loader) => <span key={`loader-${loader}`}>{loader}</span>)}
                    {gameVersions.slice(-8).map((version) => <span key={`version-${version}`}>{version}</span>)}
                    {categories.slice(0, 8).map((category) => <span key={`category-${category}`}>{category}</span>)}
                </div>
            )}
            {links.length > 0 && (
                <div className="browse-detail-links">
                    {links.map((link) => (
                        <a key={link.label} href={link.value} target="_blank" rel="noreferrer">{link.label}</a>
                    ))}
                </div>
            )}
            <div className="browse-detail-tabs">
                <button className={tab === 'description' ? 'active' : ''} type="button" onClick={() => setTab('description')}>
                    Description
                </button>
                <button className={tab === 'versions' ? 'active' : ''} type="button" onClick={() => setTab('versions')}>
                    Versions
                </button>
            </div>
            {tab === 'description' && project.body && (
                <div className="browse-detail-body">
                    <Suspense fallback={<p className="muted">Loading description...</p>}>
                        <MarkdownContent>{project.body}</MarkdownContent>
                    </Suspense>
                </div>
            )}
            {tab === 'versions' && (
                <ModrinthVersionList
                    profile={profile}
                    project={project}
                    versions={versions}
                    loading={versionsLoading}
                    installed={installed}
                    updatePlan={updatePlan}
                    projectBusy={projectBusy}
                    installActionKey={installActionKey}
                    updateActionKey={updateActionKey}
                    onLoadVersions={onLoadVersions}
                    onInstallVersion={onInstallVersion}
                    onUpdateVersion={onUpdateVersion}
                />
            )}
        </article>
    );
}

function ModrinthVersionList({
    profile,
    project,
    versions,
    loading,
    installed,
    updatePlan,
    projectBusy,
    installActionKey,
    updateActionKey,
    onLoadVersions,
    onInstallVersion,
    onUpdateVersion
}: {
    profile?: domain.Profile;
    project: domain.ModrinthProject;
    versions: domain.ModrinthVersion[];
    loading: boolean;
    installed: boolean;
    updatePlan?: domain.ModrinthUpdatePlan;
    projectBusy: boolean;
    installActionKey: string;
    updateActionKey: string;
    onLoadVersions: (profileId: string, projectId: string) => void;
    onInstallVersion: (projectId: string, versionId: string) => void;
    onUpdateVersion: (projectId: string, versionId: string) => void;
}) {
    if (!profile) {
        return <p className="muted">Select an installation to view compatible versions.</p>;
    }

    return (
        <div className="browse-version-panel">
            <div className="browse-version-toolbar">
                <div>
                    <strong>{versions.length} compatible version{versions.length === 1 ? '' : 's'}</strong>
                    <span>{profile.minecraftVersion} / {profile.loader.type}</span>
                </div>
                <button
                    className="small"
                    type="button"
                    disabled={loading}
                    onClick={() => onLoadVersions(profile.id, project.projectId)}
                >
                    {loading ? 'Loading' : 'Refresh'}
                </button>
            </div>
            {installed && updatePlan?.currentVersionNumber && (
                <p className="muted">Current installed version: {updatePlan.currentVersionNumber}</p>
            )}
            {loading && versions.length === 0 && <p className="muted">Loading compatible versions.</p>}
            {!loading && versions.length === 0 && <p className="muted">No compatible versions found for this installation.</p>}
            <div className="browse-version-list">
                {versions.map((version) => {
                    const actionKey = modrinthVersionActionKey(project.projectId, version.id);
                    const isCurrent = installed && modrinthVersionIsCurrent(updatePlan, version);
                    const installing = installActionKey === actionKey;
                    const updating = updateActionKey === actionKey;
                    const busy = installing || updating;
                    const actionLabel = isCurrent
                        ? 'Installed'
                        : installed
                            ? updating ? 'Switching' : 'Use version'
                            : installing ? 'Installing' : 'Install';
                    return (
                        <article key={version.id} className={isCurrent ? 'browse-version-row current' : 'browse-version-row'}>
                            <div className="browse-version-main">
                                <div>
                                    <h3>{version.versionNumber || version.name}</h3>
                                    <p>{version.name}</p>
                                </div>
                                <div className="browse-version-tags">
                                    {version.versionType && <span>{version.versionType}</span>}
                                    {version.datePublished && <span>{formatDateTime(version.datePublished)}</span>}
                                    {version.file?.size > 0 && <span>{formatBytes(version.file.size)}</span>}
                                    {version.file?.fileName && <span>{version.file.fileName}</span>}
                                </div>
                                {version.changelog && (
                                    <details className="browse-version-changelog">
                                        <summary>Changelog</summary>
                                        <MarkdownContent>{version.changelog}</MarkdownContent>
                                    </details>
                                )}
                            </div>
                            <button
                                className={isCurrent ? 'small' : 'small primary'}
                                type="button"
                                disabled={projectBusy || isCurrent}
                                onClick={() => installed
                                    ? onUpdateVersion(project.projectId, version.id)
                                    : onInstallVersion(project.projectId, version.id)}
                            >
                                {busy ? 'Working' : actionLabel}
                            </button>
                        </article>
                    );
                })}
            </div>
        </div>
    );
}

function ProfileSettingsPanel({
    profile,
    draft,
    javaRuntime,
    javaInstallProgress,
    modList,
    modrinthUpdatePlans,
    modActionKey,
    onDraftChange,
    onInstallJava,
    onSave,
    onRefreshMods,
    onImportMod,
    onExportModpack,
    onOpenModsFolder,
    onCheckModrinthUpdates,
    onUpdateModrinthProject,
    onUpdateModrinthFile,
    onBrowseMod,
    onToggleMod,
    onBulkToggleMods,
    onDeleteMod
}: {
    profile: domain.Profile;
    draft: ProfileSettingsDraft;
    javaRuntime?: domain.ProfileJavaRuntime;
    javaInstallProgress: JavaInstallProgress | null;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    modActionKey: string;
    onDraftChange: (draft: ProfileSettingsDraft) => void;
    onInstallJava: (version: number) => void;
    onSave: (event: FormEvent<HTMLFormElement>) => void;
    onRefreshMods: (profileId: string) => void;
    onImportMod: (profileId: string) => void;
    onExportModpack: (profileId: string) => void;
    onOpenModsFolder: (profileId: string) => void;
    onCheckModrinthUpdates: (profileId: string) => void;
    onUpdateModrinthProject: (profileId: string, projectId: string) => void;
    onUpdateModrinthFile: (profileId: string, fileName: string) => void;
    onBrowseMod: (profileId: string, projectId: string, query: string) => void;
    onToggleMod: (profileId: string, fileName: string, enabled: boolean) => void;
    onBulkToggleMods: (profileId: string, fileNames: string[], enabled: boolean) => void;
    onDeleteMod: (profileId: string, fileName: string) => void;
}) {
    const showJavaNotice = shouldShowJavaRuntimeNotice(javaRuntime, javaInstallProgress);

    return (
        <form className="profile-settings-panel" onSubmit={onSave}>
            <div className="profile-settings-header">
                <div>
                    <p className="eyebrow">Profile settings</p>
                    <h3>{profile.name}</h3>
                </div>
                <button className="primary" type="submit">Save</button>
            </div>
            <label className="wide">
                Game directory
                <input
                    value={draft.gameDir}
                    onChange={(event) => onDraftChange({...draft, gameDir: event.target.value})}
                />
            </label>
            <label>
                Min memory MB
                <input
                    type="number"
                    min="512"
                    step="256"
                    value={draft.minMB}
                    onChange={(event) => onDraftChange({...draft, minMB: Number(event.target.value)})}
                />
            </label>
            <label>
                Max memory MB
                <input
                    type="number"
                    min="512"
                    step="256"
                    value={draft.maxMB}
                    onChange={(event) => onDraftChange({...draft, maxMB: Number(event.target.value)})}
                />
            </label>
            <dl className="detail-grid settings-details wide">
                <div>
                    <dt>Minecraft</dt>
                    <dd>{profile.minecraftVersion}</dd>
                </div>
                <div>
                    <dt>Loader</dt>
                    <dd>{profile.loader.type}{profile.loader.version ? ` ${profile.loader.version}` : ''}</dd>
                </div>
                <div>
                    <dt>Install status</dt>
                    <dd>{installStatusText(profile)}</dd>
                </div>
                <div>
                    <dt>Profile ID</dt>
                    <dd>{profile.id}</dd>
                </div>
            </dl>
            {showJavaNotice && (
                <div className="wide">
                    <JavaRuntimePanel
                        runtime={javaRuntime}
                        progress={javaInstallProgress}
                        detailed
                        onInstallJava={onInstallJava}
                    />
                </div>
            )}
            <ModManager
                profile={profile}
                modList={modList}
                updatePlans={modrinthUpdatePlans}
                busyKey={modActionKey}
                onRefresh={onRefreshMods}
                onImport={onImportMod}
                onExport={onExportModpack}
                onOpenFolder={onOpenModsFolder}
                onCheckUpdates={onCheckModrinthUpdates}
                onUpdateProject={onUpdateModrinthProject}
                onUpdateFile={onUpdateModrinthFile}
                onBrowseMod={onBrowseMod}
                onToggle={onToggleMod}
                onBulkToggle={onBulkToggleMods}
                onDelete={onDeleteMod}
            />
        </form>
    );
}

function ModManager({
    profile,
    modList,
    updatePlans,
    busyKey,
    onRefresh,
    onImport,
    onExport,
    onOpenFolder,
    onCheckUpdates,
    onUpdateProject,
    onUpdateFile,
    onBrowseMod,
    onToggle,
    onBulkToggle,
    onDelete
}: {
    profile: domain.Profile;
    modList?: domain.ModList;
    updatePlans: domain.ModrinthUpdatePlan[];
    busyKey: string;
    onRefresh: (profileId: string) => void;
    onImport: (profileId: string) => void;
    onExport: (profileId: string) => void;
    onOpenFolder: (profileId: string) => void;
    onCheckUpdates: (profileId: string) => void;
    onUpdateProject: (profileId: string, projectId: string) => void;
    onUpdateFile: (profileId: string, fileName: string) => void;
    onBrowseMod: (profileId: string, projectId: string, query: string) => void;
    onToggle: (profileId: string, fileName: string, enabled: boolean) => void;
    onBulkToggle: (profileId: string, fileNames: string[], enabled: boolean) => void;
    onDelete: (profileId: string, fileName: string) => void;
}) {
    const mods = useMemo(() => modList?.mods ?? [], [modList]);
    const profileBusy = busyKey.startsWith(`${profile.id}:`);
    const isVanilla = profile.loader.type === 'vanilla';
    const updateByFileName = useMemo(() => modrinthUpdatePlansByFile(updatePlans), [updatePlans]);
    const [selectedMods, setSelectedMods] = useState<Record<string, boolean>>({});
    const selectedFileNames = mods.filter((mod) => selectedMods[mod.fileName]).map((mod) => mod.fileName);
    const selectedEnabled = mods.filter((mod) => selectedMods[mod.fileName] && mod.enabled).map((mod) => mod.fileName);
    const selectedDisabled = mods.filter((mod) => selectedMods[mod.fileName] && !mod.enabled).map((mod) => mod.fileName);
    const allSelected = mods.length > 0 && selectedFileNames.length === mods.length;

    useEffect(() => {
        setSelectedMods((current) => {
            const existing = new Set(mods.map((mod) => mod.fileName));
            const next: Record<string, boolean> = {};
            for (const [fileName, selected] of Object.entries(current)) {
                if (selected && existing.has(fileName)) {
                    next[fileName] = true;
                }
            }
            return next;
        });
    }, [mods]);

    function toggleSelectedMod(fileName: string) {
        setSelectedMods((current) => ({...current, [fileName]: !current[fileName]}));
    }

    function selectAllMods() {
        const next: Record<string, boolean> = {};
        for (const mod of mods) {
            next[mod.fileName] = true;
        }
        setSelectedMods(next);
    }

    function clearSelectedMods() {
        setSelectedMods({});
    }

    function bulkToggle(enabled: boolean) {
        const targets = enabled ? selectedDisabled : selectedEnabled;
        if (targets.length === 0) {
            return;
        }
        onBulkToggle(profile.id, targets, enabled);
    }

    return (
        <section className="mod-manager wide">
            <div className="mod-manager-header">
                <div>
                    <p className="eyebrow">Mods</p>
                    <h3>{mods.length} local mod{mods.length === 1 ? '' : 's'}</h3>
                </div>
                <div className="mod-manager-actions">
                    <button
                        className="small"
                        type="button"
                        disabled={profileBusy}
                        onClick={() => onRefresh(profile.id)}
                    >
                        Refresh
                    </button>
                    <button
                        className="small"
                        type="button"
                        disabled={profileBusy}
                        onClick={() => onCheckUpdates(profile.id)}
                    >
                        Check updates
                    </button>
                    <button
                        className="small"
                        type="button"
                        disabled={profileBusy}
                        onClick={() => onOpenFolder(profile.id)}
                    >
                        Open folder
                    </button>
                    <button
                        className="small"
                        type="button"
                        disabled={profileBusy}
                        onClick={() => onExport(profile.id)}
                    >
                        Export .mrpack
                    </button>
                    <button
                        className="small primary"
                        type="button"
                        disabled={profileBusy}
                        onClick={() => onImport(profile.id)}
                    >
                        Import .jar
                    </button>
                </div>
            </div>
            <p className="mod-manager-path">{modList?.modsDir ?? `${profile.gameDir}/mods`}</p>
            {isVanilla && <p className="mod-note">Vanilla profiles do not load mods until a mod loader is installed.</p>}
            {mods.length === 0 ? (
                <p className="muted">No local mods in this profile.</p>
            ) : (
                <>
                    <div className="mod-bulk-actions">
                        <label>
                            <input
                                type="checkbox"
                                checked={allSelected}
                                disabled={profileBusy}
                                onChange={(event) => event.target.checked ? selectAllMods() : clearSelectedMods()}
                            />
                            <span>{selectedFileNames.length} selected</span>
                        </label>
                        <button className="small" type="button" disabled={profileBusy || allSelected} onClick={selectAllMods}>
                            Select all
                        </button>
                        <button className="small" type="button" disabled={profileBusy || selectedFileNames.length === 0} onClick={clearSelectedMods}>
                            Clear
                        </button>
                        <button className="small" type="button" disabled={profileBusy || selectedDisabled.length === 0} onClick={() => bulkToggle(true)}>
                            Enable selected
                        </button>
                        <button className="small" type="button" disabled={profileBusy || selectedEnabled.length === 0} onClick={() => bulkToggle(false)}>
                            Disable selected
                        </button>
                    </div>
                    <div className="mod-list">
                        {mods.map((mod) => {
                        const toggleKey = `${profile.id}:toggle:${mod.fileName}`;
                        const deleteKey = `${profile.id}:delete:${mod.fileName}`;
                        const updatePlan = updateByFileName[modrinthModFileKey(mod.fileName)];
                        const updateStatus = modrinthUpdateStatus(updatePlan);
                        const browseLabel = mod.displayName || mod.fileName;
                        return (
                            <div key={mod.fileName} className={mod.enabled ? 'mod-row' : 'mod-row disabled'}>
                                <label className="mod-select">
                                    <input
                                        type="checkbox"
                                        checked={!!selectedMods[mod.fileName]}
                                        disabled={profileBusy}
                                        onChange={() => toggleSelectedMod(mod.fileName)}
                                    />
                                </label>
                                <div className="mod-name">
                                    <strong>{mod.displayName || mod.fileName}</strong>
                                    <span>{mod.fileName}</span>
                                </div>
                                <div className="mod-meta">
                                    <span className="mod-size">{formatBytes(mod.size)}</span>
                                    <span className="mod-updated">{formatDateTime(mod.updatedAt)}</span>
                                    <span className={updatePlan?.updateAvailable ? 'mod-update available' : 'mod-update'}>{updateStatus}</span>
                                </div>
                                <div className="mod-actions">
                                    {updatePlan?.updateAvailable && (
                                        <button
                                            className="small primary"
                                            type="button"
                                            disabled={profileBusy}
                                            onClick={() => updatePlan.tracked
                                                ? onUpdateProject(profile.id, updatePlan.projectId)
                                                : onUpdateFile(profile.id, updatePlan.currentFileName || mod.fileName)}
                                        >
                                            Update
                                        </button>
                                    )}
                                    <button
                                        className="small"
                                        type="button"
                                        disabled={profileBusy}
                                        onClick={() => onBrowseMod(profile.id, updatePlan?.projectId ?? '', browseLabel)}
                                    >
                                        Browse
                                    </button>
                                    <button
                                        className="small"
                                        type="button"
                                        disabled={profileBusy}
                                        onClick={() => onToggle(profile.id, mod.fileName, !mod.enabled)}
                                    >
                                        {busyKey === toggleKey ? 'Saving' : mod.enabled ? 'Disable' : 'Enable'}
                                    </button>
                                    <button
                                        className="small danger"
                                        type="button"
                                        disabled={profileBusy}
                                        onClick={() => onDelete(profile.id, mod.fileName)}
                                    >
                                        {busyKey === deleteKey ? 'Deleting' : 'Delete'}
                                    </button>
                                </div>
                            </div>
                        );
                        })}
                    </div>
                </>
            )}
        </section>
    );
}

function JavaRuntimePanel({
    runtime,
    progress,
    detailed = false,
    onInstallJava
}: {
    runtime?: domain.ProfileJavaRuntime;
    progress: JavaInstallProgress | null;
    detailed?: boolean;
    onInstallJava: (version: number) => void;
}) {
    const installing = isJavaInstalling(progress);
    const progressMatches = !!runtime?.requiredMajor && progress?.version === runtime.requiredMajor.toString();

    return (
        <div className="install-status">
            <div>
                <span>Java runtime</span>
                <strong>{runtime ? `Requires Java ${runtime.requiredMajor}` : 'Checking Java runtime'}</strong>
                <p>{javaRuntimeText(runtime)}</p>
                {runtime?.javaPath && <p>{runtime.javaPath}</p>}
                {progressMatches && progress && <p>{javaProgressMessage(progress)}</p>}
                {detailed && runtime && (
                    <dl className="java-detail-grid">
                        <div>
                            <dt>Required</dt>
                            <dd>Java {runtime.requiredMajor}</dd>
                        </div>
                        <div>
                            <dt>Status</dt>
                            <dd>{runtime.installed ? 'Installed' : 'Missing'}</dd>
                        </div>
                        <div>
                            <dt>Detected version</dt>
                            <dd>{runtime.version || 'Not detected'}</dd>
                        </div>
                        <div>
                            <dt>Java path</dt>
                            <dd>{runtime.javaPath || 'Not selected'}</dd>
                        </div>
                    </dl>
                )}
            </div>
            {runtime && !runtime.installed && (
                <button
                    type="button"
                    disabled={installing}
                    onClick={() => onInstallJava(runtime.requiredMajor)}
                >
                    {installing ? 'Installing Java' : `Install Java ${runtime.requiredMajor}`}
                </button>
            )}
            {progressMatches && progress && <ProgressBar progress={progress}/>}
        </div>
    );
}

function ProfileStatusPanel({
    profile,
    progress,
    launch,
    javaRuntime
}: {
    profile: domain.Profile;
    progress?: InstallProgress;
    launch?: LaunchState;
    javaRuntime?: domain.ProfileJavaRuntime;
}) {
    const playReason = playDisabledReason(profile, launch, javaRuntime);

    return (
        <section className="install-status dashboard-panel">
            <div>
                <span>Profile status</span>
                <strong>{installStatusText(profile)}</strong>
                <p>{progress ? progressMessage(progress) : profile.install?.message || 'No active install.'}</p>
                <p>{launch ? launchStatusText(launch) : 'Not running.'}</p>
                {playReason && <p>{playReason}</p>}
                <p>{javaRuntimeText(javaRuntime)}</p>
                {profile.install?.lastError && <p>{profile.install.lastError}</p>}
            </div>
            <ProgressBar progress={progress}/>
        </section>
    );
}

function ProgressBar({progress}: { progress?: { stage?: string; percent?: number } }) {
    return (
        <div className="progress-block">
            <div className="progress-meta">
                <span>{progress?.stage ?? 'idle'}</span>
                <strong>{progress?.percent ?? 0}%</strong>
            </div>
            <div className="progress-track" aria-label="Install progress">
                <div style={{width: `${progress?.percent ?? 0}%`}}/>
            </div>
        </div>
    );
}

function LauncherLogPanel({
    logs,
    compact = false,
    detailed = false,
    onOpenLogs
}: {
    logs: LauncherLog[];
    compact?: boolean;
    detailed?: boolean;
    onOpenLogs?: () => void;
}) {
    return (
        <section className={compact ? 'log-panel compact' : 'log-panel'}>
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Launcher logs</p>
                    <h2>{detailed ? 'Event stream' : 'Recent events'}</h2>
                </div>
                {onOpenLogs && (
                    <button className="small" type="button" onClick={onOpenLogs}>Open logs</button>
                )}
            </div>
            {logs.length === 0 ? (
                <p className="muted">No launcher events yet.</p>
            ) : (
                <div className="log-list">
                    {logs.map((log) => (
                        <div key={log.id} className={`log-row ${log.level}`}>
                            <span>{log.time}</span>
                            <strong>{log.source}</strong>
                            <p>{log.message}</p>
                        </div>
                    ))}
                </div>
            )}
        </section>
    );
}

function ClassicLogsPanel({
    logs,
    profiles,
    localServers,
    gameLogLists,
    gameLogContents,
    gameLogActionKey,
    exportProfileId,
    onClearLogs,
    onRefreshGameLogs,
    onReadGameLog,
    onOpenLogsFolder,
    onOpenWindow,
    onExportLogs
}: {
    logs: LauncherLog[];
    profiles: domain.Profile[];
    localServers: domain.LocalServer[];
    gameLogLists: Record<string, domain.GameLogList>;
    gameLogContents: Record<string, domain.GameLogContent>;
    gameLogActionKey: string;
    exportProfileId: string;
    onClearLogs: () => void;
    onRefreshGameLogs: (profileId: string) => void;
    onReadGameLog: (profileId: string, fileName: string) => void;
    onOpenLogsFolder: (profileId: string) => void;
    onOpenWindow?: () => void;
    onExportLogs: (profileId: string) => void;
}) {
    const [mode, setMode] = useState<LogsMode>('launcher');
    const [profileFilter, setProfileFilter] = useState('all');
    const [levelFilter, setLevelFilter] = useState<LogLevelFilter>('all');
    const [sourceFilter, setSourceFilter] = useState('all');
    const [query, setQuery] = useState('');
    const [copyStatus, setCopyStatus] = useState('');
    const [gameProfileId, setGameProfileId] = useState('');
    const [gameFileName, setGameFileName] = useState('live');

    const profilesById = useMemo(() => new Map(profiles.map((profile) => [profile.id, profile])), [profiles]);
    const localServersById = useMemo(() => new Map(localServers.map((server) => [server.id, server])), [localServers]);
    const gameProfile = useMemo(
        () => profiles.find((profile) => profile.id === gameProfileId) ?? profiles[0],
        [profiles, gameProfileId]
    );
    const gameLogList = gameProfile ? gameLogLists[gameProfile.id] : undefined;
    const gameFiles = gameLogList?.files ?? [];
    const exportProfile = profiles.find((profile) => profile.id === exportProfileId) ?? gameProfile;
    const liveGameLogs = useMemo(() => {
        if (!gameProfile) {
            return [];
        }
        return logs
            .filter((log) => log.profileId === gameProfile.id && log.source.startsWith('Game '))
            .reverse();
    }, [logs, gameProfile]);
    const selectedGameContent = gameProfile && gameFileName !== 'live'
        ? gameLogContents[gameLogContentKey(gameProfile.id, gameFileName)]
        : undefined;

    useEffect(() => {
        if (profiles.length === 0) {
            setGameProfileId('');
            return;
        }
        if (!profiles.some((profile) => profile.id === gameProfileId)) {
            setGameProfileId(profiles[0].id);
        }
    }, [profiles, gameProfileId]);

    useEffect(() => {
        if (mode === 'game' && gameProfile?.id && !gameLogLists[gameProfile.id]) {
            onRefreshGameLogs(gameProfile.id);
        }
    }, [mode, gameProfile?.id]);

    useEffect(() => {
        if (gameFileName !== 'live' && gameFiles.length > 0 && !gameFiles.some((file) => file.fileName === gameFileName)) {
            setGameFileName('live');
        }
    }, [gameFileName, gameFiles]);

    useEffect(() => {
        if (profileFilter.startsWith('server:')) {
            if (!localServersById.has(profileFilter.slice('server:'.length))) {
                setProfileFilter('all');
            }
            return;
        }
        if (profileFilter !== 'all' && profileFilter !== 'global' && !profilesById.has(profileFilter)) {
            setProfileFilter('all');
        }
    }, [profileFilter, profilesById, localServersById]);

    const baseLogs = useMemo(() => logs.filter((log) => {
        if (profileFilter === 'global' && (log.profileId || log.serverId)) {
            return false;
        }
        if (profileFilter.startsWith('server:')) {
            return log.serverId === profileFilter.slice('server:'.length);
        }
        if (profileFilter !== 'all' && profileFilter !== 'global' && log.profileId !== profileFilter) {
            return false;
        }
        if (levelFilter !== 'all' && log.level !== levelFilter) {
            return false;
        }
        const text = query.trim().toLowerCase();
        if (!text) {
            return true;
        }
        const profileName = logTargetName(log, profilesById, localServersById);
        return [
            log.time,
            log.level,
            log.source,
            log.message,
            profileName,
        ].some((part) => part.toLowerCase().includes(text));
    }), [logs, profileFilter, levelFilter, query, profilesById, localServersById]);

    const sourceEntries = useMemo(() => {
        const counts = new Map<string, number>();
        for (const log of baseLogs) {
            counts.set(log.source, (counts.get(log.source) ?? 0) + 1);
        }
        return Array.from(counts.entries())
            .sort(([left], [right]) => left.localeCompare(right))
            .map(([source, count]) => ({source, count}));
    }, [baseLogs]);

    useEffect(() => {
        if (sourceFilter !== 'all' && !sourceEntries.some((entry) => entry.source === sourceFilter)) {
            setSourceFilter('all');
        }
    }, [sourceFilter, sourceEntries]);

    const filteredLogs = useMemo(() => baseLogs.filter((log) => {
        return sourceFilter === 'all' || log.source === sourceFilter;
    }), [baseLogs, sourceFilter]);

    const consoleLogs = useMemo(() => [...filteredLogs].reverse(), [filteredLogs]);
    const errorCount = filteredLogs.filter((log) => log.level === 'error').length;
    const selectedSourceLabel = sourceFilter === 'all' ? 'All sources' : sourceFilter;

    async function copyVisibleLogs() {
        if (consoleLogs.length === 0) {
            setCopyStatus('Nothing to copy');
            return;
        }
        const text = consoleLogs.map((log) => formatLauncherLogLine(log, profilesById, localServersById)).join('\n');
        try {
            if (!navigator.clipboard?.writeText) {
                throw new Error('Clipboard is unavailable');
            }
            await navigator.clipboard.writeText(text);
            setCopyStatus('Copied');
        } catch (err) {
            setCopyStatus(errorText(err));
        }
    }

    function clearLogs() {
        onClearLogs();
        setCopyStatus('Cleared');
        setSourceFilter('all');
    }

    async function copyGameLog() {
        const text = gameFileName === 'live'
            ? liveGameLogs.map((log) => formatLauncherLogLine(log, profilesById, localServersById)).join('\n')
            : selectedGameContent?.content ?? '';
        if (!text) {
            setCopyStatus('Nothing to copy');
            return;
        }
        try {
            if (!navigator.clipboard?.writeText) {
                throw new Error('Clipboard is unavailable');
            }
            await navigator.clipboard.writeText(text);
            setCopyStatus('Copied');
        } catch (err) {
            setCopyStatus(errorText(err));
        }
    }

    function selectGameFile(fileName: string) {
        setGameFileName(fileName);
        setCopyStatus('');
        if (gameProfile?.id && fileName !== 'live' && !gameLogContents[gameLogContentKey(gameProfile.id, fileName)]) {
            onReadGameLog(gameProfile.id, fileName);
        }
    }

    return (
        <section className="classic-logs">
            <aside className="logs-sidebar">
                <div className="logs-mode-switch">
                    <button
                        type="button"
                        className={mode === 'launcher' ? 'active' : ''}
                        onClick={() => setMode('launcher')}
                    >
                        Launcher
                    </button>
                    <button
                        type="button"
                        className={mode === 'game' ? 'active' : ''}
                        onClick={() => setMode('game')}
                    >
                        Game files
                    </button>
                </div>
                {mode === 'launcher' ? (
                    <>
                <div className="logs-sidebar-heading">
                    <p className="eyebrow">Sources</p>
                    <strong>{logs.length} events</strong>
                </div>
                <button
                    type="button"
                    className={sourceFilter === 'all' ? 'log-source active' : 'log-source'}
                    onClick={() => setSourceFilter('all')}
                >
                    <span>All sources</span>
                    <strong>{baseLogs.length}</strong>
                </button>
                {sourceEntries.map((entry) => (
                    <button
                        key={entry.source}
                        type="button"
                        className={sourceFilter === entry.source ? 'log-source active' : 'log-source'}
                        onClick={() => setSourceFilter(entry.source)}
                    >
                        <span>{entry.source}</span>
                        <strong>{entry.count}</strong>
                    </button>
                ))}
                    </>
                ) : (
                    <>
                        <div className="logs-sidebar-heading">
                            <p className="eyebrow">Game logs</p>
                            <strong>{gameProfile?.name ?? 'No installation'}</strong>
                        </div>
                        <button
                            type="button"
                            className={gameFileName === 'live' ? 'log-source active' : 'log-source'}
                            disabled={!gameProfile}
                            onClick={() => selectGameFile('live')}
                        >
                            <span>Live output</span>
                            <strong>{liveGameLogs.length}</strong>
                        </button>
                        {gameFiles.map((file) => (
                            <button
                                key={file.fileName}
                                type="button"
                                className={gameFileName === file.fileName ? 'log-source active' : 'log-source'}
                                onClick={() => selectGameFile(file.fileName)}
                            >
                                <span>{gameLogFileLabel(file)}</span>
                                <strong>{gameLogKindLabel(file.kind)}</strong>
                            </button>
                        ))}
                    </>
                )}
            </aside>

            <section className="logs-workspace">
                {mode === 'launcher' ? (
                    <>
                <div className="logs-console-header">
                    <div>
                        <p className="eyebrow">Live log</p>
                        <h2>{selectedSourceLabel}</h2>
                        <p>{filteredLogs.length} visible events{errorCount > 0 ? ` / ${errorCount} errors` : ''}</p>
                    </div>
                    <div className="logs-actions">
                        {onOpenWindow && <button type="button" onClick={onOpenWindow}>Open native window</button>}
                        <button
                            type="button"
                            disabled={!exportProfile || gameLogActionKey === `${exportProfile?.id}:logs:export`}
                            onClick={() => exportProfile && onExportLogs(exportProfile.id)}
                        >
                            {exportProfile && gameLogActionKey === `${exportProfile.id}:logs:export` ? 'Exporting' : 'Export ZIP'}
                        </button>
                        <button type="button" onClick={copyVisibleLogs} disabled={consoleLogs.length === 0}>Copy visible</button>
                        <button className="danger" type="button" onClick={clearLogs} disabled={logs.length === 0}>Clear</button>
                    </div>
                </div>

                <div className="logs-toolbar">
                    <label>
                        Target
                        <select value={profileFilter} onChange={(event) => setProfileFilter(event.target.value)}>
                            <option value="all">All targets</option>
                            <option value="global">Launcher only</option>
                            {profiles.map((profile) => (
                                <option key={profile.id} value={profile.id}>
                                    {profile.name}
                                </option>
                            ))}
                            {localServers.map((server) => (
                                <option key={server.id} value={`server:${server.id}`}>
                                    Server: {server.name}
                                </option>
                            ))}
                        </select>
                    </label>
                    <label>
                        Level
                        <select value={levelFilter} onChange={(event) => setLevelFilter(event.target.value as LogLevelFilter)}>
                            <option value="all">All levels</option>
                            <option value="info">Info</option>
                            <option value="success">Success</option>
                            <option value="error">Error</option>
                        </select>
                    </label>
                    <label className="logs-search">
                        Search
                        <input
                            value={query}
                            placeholder="Search logs"
                            onChange={(event) => setQuery(event.target.value)}
                        />
                    </label>
                </div>

                {copyStatus && <p className="logs-status">{copyStatus}</p>}

                <div className="logs-console" role="log" aria-live="polite">
                    {consoleLogs.length === 0 ? (
                        <p className="logs-empty">No logs match the current filters.</p>
                    ) : (
                        consoleLogs.map((log) => (
                            <div key={log.id} className={`classic-log-line ${log.level}`}>
                                <span className="log-line-time">{log.time}</span>
                                <span className="log-line-level">{log.level}</span>
                                <span className="log-line-source">{log.source}</span>
                                <span className="log-line-profile">{logProfileLabel(log, profilesById, localServersById)}</span>
                                <span className="log-line-message">{log.message}</span>
                            </div>
                        ))
                    )}
                </div>
                    </>
                ) : (
                    <>
                        <div className="logs-console-header">
                            <div>
                                <p className="eyebrow">Minecraft logs</p>
                                <h2>{gameFileName === 'live' ? 'Live output' : gameFileName}</h2>
                                <p>{gameLogStatusText(gameProfile, gameLogList, selectedGameContent, liveGameLogs, gameFileName)}</p>
                            </div>
                            <div className="logs-actions">
                                {onOpenWindow && <button type="button" onClick={onOpenWindow}>Open native window</button>}
                                <button type="button" disabled={!gameProfile} onClick={copyGameLog}>Copy visible</button>
                                <button
                                    type="button"
                                    disabled={!gameProfile || gameLogActionKey === `${gameProfile?.id}:logs:refresh`}
                                    onClick={() => gameProfile && onRefreshGameLogs(gameProfile.id)}
                                >
                                    {gameProfile && gameLogActionKey === `${gameProfile.id}:logs:refresh` ? 'Refreshing' : 'Refresh'}
                                </button>
                                <button
                                    type="button"
                                    disabled={!gameProfile || gameLogActionKey === `${gameProfile?.id}:logs:export`}
                                    onClick={() => gameProfile && onExportLogs(gameProfile.id)}
                                >
                                    {gameProfile && gameLogActionKey === `${gameProfile.id}:logs:export` ? 'Exporting' : 'Export ZIP'}
                                </button>
                                <button
                                    type="button"
                                    disabled={!gameProfile}
                                    onClick={() => gameProfile && onOpenLogsFolder(gameProfile.id)}
                                >
                                    Open folder
                                </button>
                            </div>
                        </div>

                        <div className="logs-toolbar game-logs-toolbar">
                            <label>
                                Installation
                                <select
                                    value={gameProfile?.id ?? ''}
                                    disabled={profiles.length === 0}
                                    onChange={(event) => {
                                        setGameProfileId(event.target.value);
                                        setGameFileName('live');
                                        setCopyStatus('');
                                    }}
                                >
                                    {profiles.length === 0 && <option value="">No installations</option>}
                                    {profiles.map((profile) => (
                                        <option key={profile.id} value={profile.id}>
                                            {profile.name}
                                        </option>
                                    ))}
                                </select>
                            </label>
                            <div className="game-log-meta">
                                <span>{gameLogList?.logsDir ?? 'No logs folder selected'}</span>
                            </div>
                        </div>

                        {copyStatus && <p className="logs-status">{copyStatus}</p>}

                        <div className="logs-console game-log-console" role="log" aria-live="polite">
                            {gameFileName === 'live' ? (
                                liveGameLogs.length === 0 ? (
                                    <p className="logs-empty">No live game output for this installation yet.</p>
                                ) : (
                                    liveGameLogs.map((log) => (
                                        <div key={log.id} className={`classic-log-line ${log.level}`}>
                                            <span className="log-line-time">{log.time}</span>
                                            <span className="log-line-level">{log.source.replace('Game ', '')}</span>
                                            <span className="log-line-message">{log.message}</span>
                                        </div>
                                    ))
                                )
                            ) : selectedGameContent ? (
                                <>
                                    {selectedGameContent.truncated && (
                                        <p className="logs-empty">Showing a truncated view of this log.</p>
                                    )}
                                    <pre className="game-log-content">{selectedGameContent.content}</pre>
                                </>
                            ) : (
                                <p className="logs-empty">
                                    {gameProfile && gameLogActionKey === `${gameProfile.id}:logs:read:${gameFileName}`
                                        ? 'Loading log file.'
                                        : 'Select a log file.'}
                                </p>
                            )}
                        </div>
                    </>
                )}
            </section>
        </section>
    );
}

function DetachedLogsWindow({
    logs,
    profiles,
    localServers,
    onClose
}: {
    logs: LauncherLog[];
    profiles: domain.Profile[];
    localServers: domain.LocalServer[];
    onClose: () => void;
}) {
    const [copyStatus, setCopyStatus] = useState('');
    const profilesById = useMemo(() => new Map(profiles.map((profile) => [profile.id, profile])), [profiles]);
    const localServersById = useMemo(() => new Map(localServers.map((server) => [server.id, server])), [localServers]);
    const visibleLogs = useMemo(() => logs.slice(0, 1000), [logs]);
    const errorCount = visibleLogs.filter((log) => log.level === 'error').length;

    async function copyLogs() {
        if (visibleLogs.length === 0) {
            setCopyStatus('Nothing to copy');
            return;
        }
        const text = [...visibleLogs]
            .reverse()
            .map((log) => formatLauncherLogLine(log, profilesById, localServersById))
            .join('\n');
        try {
            if (!navigator.clipboard?.writeText) {
                throw new Error('Clipboard is unavailable');
            }
            await navigator.clipboard.writeText(text);
            setCopyStatus('Copied');
        } catch (err) {
            setCopyStatus(errorText(err));
        }
    }

    return (
        <div className="app-window-backdrop" role="presentation">
            <section className="app-window logs-window" role="dialog" aria-modal="false" aria-labelledby="detached-logs-title">
                <div className="app-window-titlebar">
                    <div>
                        <p className="eyebrow">App-owned window</p>
                        <h2 id="detached-logs-title">Detached logs</h2>
                        <p>{visibleLogs.length} visible events{errorCount > 0 ? ` / ${errorCount} errors` : ''}</p>
                    </div>
                    <div className="app-window-actions">
                        <button type="button" onClick={copyLogs} disabled={visibleLogs.length === 0}>Copy</button>
                        <button type="button" onClick={onClose}>Close</button>
                    </div>
                </div>
                {copyStatus && <p className="app-window-status">{copyStatus}</p>}
                <div className="app-window-log-feed" role="log" aria-live="polite">
                    {visibleLogs.length === 0 ? (
                        <p className="logs-empty">No launcher events yet.</p>
                    ) : (
                        visibleLogs.map((log) => (
                            <div key={log.id} className={`app-window-log-line ${log.level}`}>
                                <span className="log-line-time">{log.time}</span>
                                <span className="log-line-level">{log.level}</span>
                                <span className="log-line-source">{log.source}</span>
                                <span className="log-line-profile">{logProfileLabel(log, profilesById, localServersById)}</span>
                                <span className="log-line-message">{log.message}</span>
                            </div>
                        ))
                    )}
                </div>
            </section>
        </div>
    );
}

function ServerTerminalWindow({
    server,
    events,
    run,
    actionKey,
    onClose,
    onSendCommand
}: {
    server: domain.LocalServer;
    events: LocalServerEvent[];
    run?: LocalServerRunState;
    actionKey: string;
    onClose: () => void;
    onSendCommand: (id: string, command: string) => Promise<void>;
}) {
    const [command, setCommand] = useState('');
    const [status, setStatus] = useState('');
    const terminalRef = useRef<HTMLDivElement | null>(null);
    const sending = actionKey === `${server.id}:terminal-command`;

    useEffect(() => {
        setCommand('');
        setStatus('');
    }, [server.id]);

    useEffect(() => {
        if (terminalRef.current) {
            terminalRef.current.scrollTop = terminalRef.current.scrollHeight;
        }
    }, [events.length, server.id]);

    async function submitCommand(event: FormEvent<HTMLFormElement>) {
        event.preventDefault();
        const nextCommand = command.trim();
        if (!nextCommand || sending) {
            return;
        }
        setCommand('');
        setStatus('');
        try {
            await onSendCommand(server.id, nextCommand);
            setStatus('Sent');
        } catch (err) {
            setStatus(errorText(err));
        }
    }

    return (
        <div className="app-window-backdrop terminal-backdrop" role="presentation">
            <section className="app-window terminal-window" role="dialog" aria-modal="false" aria-labelledby="server-terminal-title">
                <div className="app-window-titlebar">
                    <div>
                        <p className="eyebrow">Server terminal</p>
                        <h2 id="server-terminal-title">{server.name}</h2>
                        <p>{server.minecraftVersion} / port {server.port}{run ? ` / ${localServerRunStatusText(run)}` : ''}</p>
                    </div>
                    <button type="button" onClick={onClose}>Close</button>
                </div>
                <div className="terminal-feed" ref={terminalRef} role="log" aria-live="polite">
                    {events.length === 0 ? (
                        <p className="logs-empty">No server output yet. Start the server, then keep this terminal open.</p>
                    ) : (
                        events.map((event, index) => (
                            <div key={`${event.time}-${event.stream}-${index}`} className={`terminal-event-line ${event.status} ${event.stream || 'event'}`}>
                                <span className="terminal-time">{(event.time || '').slice(11, 19) || '--:--:--'}</span>
                                <span className="terminal-status">{event.status}</span>
                                <span className="terminal-stream">{event.stream || 'event'}</span>
                                <span className="terminal-message">{event.message}{event.exitCode !== undefined ? ` (exit ${event.exitCode})` : ''}</span>
                            </div>
                        ))
                    )}
                </div>
                <form className="terminal-composer" onSubmit={submitCommand}>
                    <span aria-hidden="true">&gt;</span>
                    <input
                        value={command}
                        disabled={sending}
                        autoFocus
                        autoComplete="off"
                        spellCheck={false}
                        placeholder="server command, e.g. say Hello or stop"
                        onChange={(event) => setCommand(event.target.value)}
                    />
                    <button type="submit" disabled={sending || command.trim() === ''}>{sending ? 'Sending' : 'Send'}</button>
                </form>
                {status && <p className="app-window-status terminal-status-line">{status}</p>}
            </section>
        </div>
    );
}

function StatusTile({label, value}: { label: string; value: string }) {
    return (
        <div className="status-tile">
            <span>{label}</span>
            <strong>{value}</strong>
        </div>
    );
}

function EmptyState({title, action}: { title: string; action: string }) {
    return (
        <section className="empty-state">
            <h2>{title}</h2>
            <p>{action}</p>
        </section>
    );
}

function titleFor(screen: Screen) {
    switch (screen) {
        case 'home':
            return 'Launcher';
        case 'library':
            return 'Library';
        case 'create':
            return 'Create Profile';
        case 'account':
            return 'Account';
        case 'logs':
            return 'Logs';
        case 'settings':
            return 'Settings';
        case 'browse':
            return 'Browse';
    }
}

function profileSubtitle(profile: domain.Profile) {
    const loader = profile.loader.type === 'vanilla'
        ? 'Vanilla'
        : `${loaderDisplayName(profile.loader.type)} ${profile.loader.version || 'latest'}`;
    return `${profile.minecraftVersion} / ${loader}`;
}

function localServerSubtitle(server: domain.LocalServer) {
    return `${server.minecraftVersion} / port ${server.port}`;
}

function loaderDisplayName(loader: string) {
    switch (loader) {
        case 'fabric':
            return 'Fabric';
        case 'quilt':
            return 'Quilt';
        case 'forge':
            return 'Forge';
        case 'neoforge':
            return 'NeoForge';
        default:
            return loader || 'Vanilla';
    }
}

function accountLabel(account?: AccountDraft | domain.AccountConfig | null) {
    if (!account?.mode || account.mode === 'offline') {
        return `Offline: ${account?.offlineName || 'Player'}`;
    }
    return 'Microsoft account';
}

function logProfileLabel(
    log: LauncherLog,
    profilesById: Map<string, domain.Profile>,
    localServersById: Map<string, domain.LocalServer> = new Map()
) {
    if (log.serverId) {
        return `Server: ${localServersById.get(log.serverId)?.name ?? log.serverId}`;
    }
    if (!log.profileId) {
        return 'Launcher';
    }
    return profilesById.get(log.profileId)?.name ?? log.profileId;
}

function logTargetName(
    log: LauncherLog,
    profilesById: Map<string, domain.Profile>,
    localServersById: Map<string, domain.LocalServer> = new Map()
) {
    return logProfileLabel(log, profilesById, localServersById).toLowerCase();
}

function formatLauncherLogLine(
    log: LauncherLog,
    profilesById: Map<string, domain.Profile>,
    localServersById: Map<string, domain.LocalServer> = new Map()
) {
    return `[${log.time}] [${log.level.toUpperCase()}] [${log.source}] [${logProfileLabel(log, profilesById, localServersById)}] ${log.message}`;
}

function launcherLogExportText(logs: LauncherLog[], profiles: domain.Profile[], localServers: domain.LocalServer[] = []) {
    const profilesById = new Map(profiles.map((profile) => [profile.id, profile]));
    const localServersById = new Map(localServers.map((server) => [server.id, server]));
    return [...logs].reverse().map((log) => formatLauncherLogLine(log, profilesById, localServersById)).join('\n');
}

function detachedLogsSnapshot(logs: LauncherLog[], profiles: domain.Profile[], localServers: domain.LocalServer[]) {
    const profilesById = new Map(profiles.map((profile) => [profile.id, profile]));
    const localServersById = new Map(localServers.map((server) => [server.id, server]));
    return JSON.stringify({
        logs: logs.map((log) => ({
            ...log,
            target: logProfileLabel(log, profilesById, localServersById),
        })),
        profiles: profiles.map((profile) => ({id: profile.id, name: profile.name})),
        localServers: localServers.map((server) => ({id: server.id, name: server.name})),
    });
}

function logExportMessage(result: domain.LogExportResult) {
    const launcher = result.launcherEventsExported ? ' plus launcher events' : '';
    return `Exported ${result.filesExported} log files${launcher} to ${result.path}.`;
}

function gameLogContentKey(profileId: string, fileName: string) {
    return `${profileId}:${fileName}`;
}

function gameLogFileLabel(file: domain.GameLogFile) {
    if (file.compressed) {
        return `${file.displayName} / compressed`;
    }
    return file.displayName || file.fileName;
}

function gameLogKindLabel(kind: string) {
    switch (kind) {
        case 'crash':
            return 'crash';
        case 'jvm':
            return 'jvm';
        default:
            return 'log';
    }
}

function gameLogStatusText(
    profile: domain.Profile | undefined,
    list: domain.GameLogList | undefined,
    content: domain.GameLogContent | undefined,
    liveLogs: LauncherLog[],
    fileName: string
) {
    if (!profile) {
        return 'Select an installation.';
    }
    if (fileName === 'live') {
        return `${liveLogs.length} live output lines for ${profile.name}.`;
    }
    if (!content) {
        return list ? `${list.files?.length ?? 0} files found.` : 'Log file not loaded yet.';
    }
    const limit = content.truncated ? ` / showing ${formatBytes(content.maxBytes)}` : '';
    return `${formatBytes(content.size)}${limit}`;
}

function playDisabledReason(
    profile: domain.Profile | undefined,
    launch: LaunchState | undefined,
    javaRuntime: domain.ProfileJavaRuntime | undefined
) {
    if (!profile) {
        return 'Select profile';
    }
    if (launch?.status === 'running' || launch?.status === 'starting') {
        return 'Already running';
    }
    if (profile.loader.type !== 'vanilla' && profile.loader.type !== 'fabric' && profile.loader.type !== 'quilt' && profile.loader.type !== 'forge' && profile.loader.type !== 'neoforge') {
        return `${profile.loader.type} launch pending`;
    }
    if (profile.install?.status !== 'installed') {
        if (profile.install?.status === 'installing') {
            return 'Install in progress';
        }
        if (profile.install?.status === 'repairing') {
            return 'Repair in progress';
        }
        return 'Install required';
    }
    if (!javaRuntime) {
        return 'Checking Java runtime';
    }
    if (!javaRuntime.installed) {
        return `Install Java ${javaRuntime.requiredMajor}`;
    }
    return null;
}

function launchStatusText(launch: LaunchState) {
    switch (launch.status) {
        case 'running':
            return 'Running: ' + launch.message;
        case 'starting':
            return 'Starting: ' + launch.message;
        case 'stopped':
            return `Stopped${launch.exitCode !== undefined ? ` with exit ${launch.exitCode}` : ''}.`;
        case 'failed':
            return 'Failed: ' + launch.message;
    }
}

function localServerRunStatusText(run: LocalServerRunState) {
    switch (run.status) {
        case 'running':
            return 'Running: ' + run.message;
        case 'starting':
            return 'Starting: ' + run.message;
        case 'stopped':
            return `Stopped${run.exitCode !== undefined ? ` with exit ${run.exitCode}` : ''}.`;
        case 'failed':
            return 'Failed: ' + run.message;
    }
}

function localServerStatusText(server: domain.LocalServer, run?: LocalServerRunState) {
    const parts = [localServerInstallStatusText(server)];
    if (run) {
        parts.push(compactLaunchStatusText(run.status));
    }
    return parts.join(' / ');
}

function compactLaunchStatusText(status: LocalServerRunState['status']) {
    switch (status) {
        case 'running':
            return 'Running';
        case 'starting':
            return 'Starting';
        case 'stopped':
            return 'Stopped';
        case 'failed':
            return 'Failed';
    }
}

function localServerInstallStatusText(server: domain.LocalServer) {
    switch (server.install?.status) {
        case 'installed':
            return 'Installed';
        case 'installing':
            return 'Installing';
        case 'repairing':
            return 'Repairing';
        case 'failed':
            return 'Failed';
        default:
            return 'Not installed';
    }
}

function javaStatusText(status: domain.JavaStatus | null, fallbackPath: string) {
    if (!status) {
        return fallbackPath;
    }
    if (!status.ok) {
        return 'Java unavailable';
    }
    return status.version ? `Java ${status.version}` : 'Java OK';
}

function javaRuntimeText(runtime?: domain.ProfileJavaRuntime) {
    if (!runtime) {
        return 'Checking Java runtime.';
    }
    if (!runtime.installed) {
        return runtime.message || `Install Java ${runtime.requiredMajor}.`;
    }
    return runtime.version ? `Java ${runtime.version} ready.` : runtime.message;
}

function shouldShowJavaRuntimeNotice(
    runtime: domain.ProfileJavaRuntime | undefined,
    progress: JavaInstallProgress | null
) {
    if (!runtime) {
        return false;
    }
    const progressMatches = progress?.version === runtime.requiredMajor.toString();
    return !runtime.installed || (progressMatches && isJavaInstalling(progress));
}

function progressMessage(progress: InstallProgress) {
    const counter = progress.total > 0 ? ` (${progress.current}/${progress.total})` : '';
    return `${progress.message}${counter}`;
}

function localServerProgressMessage(progress: LocalServerProgress) {
    const counter = progress.total > 0 ? ` (${progress.current}/${progress.total})` : '';
    return `${progress.message}${counter}`;
}

function javaProgressMessage(progress: JavaInstallProgress) {
    const counter = progress.total > 0 ? ` (${formatBytes(progress.current)} / ${formatBytes(progress.total)})` : '';
    return `${progress.message}${counter}`;
}

function isInstalling(profile: domain.Profile, progressByProfile: Record<string, InstallProgress | undefined>) {
    const progress = progressByProfile[profile.id];
    return profile.install?.status === 'installing' || profile.install?.status === 'repairing' || (!!progress && !progress.done && progress.stage !== 'failed');
}

function isLocalServerInstalling(server: domain.LocalServer, progress?: LocalServerProgress) {
    if (server.install?.status === 'installing' || server.install?.status === 'repairing') {
        return true;
    }
    if (server.install?.status === 'installed') {
        return false;
    }
    return isActiveLocalServerProgress(progress);
}

function isActiveLocalServerProgress(progress?: LocalServerProgress) {
    return !!progress && !progress.done && progress.stage !== 'failed' && progress.stage !== 'complete';
}

function shouldShowLocalServerProgress(
    server: domain.LocalServer,
    progress?: LocalServerProgress
): progress is LocalServerProgress {
    return (server.install?.status === 'installing' || server.install?.status === 'repairing') && isActiveLocalServerProgress(progress);
}

function localServerStartDisabledReason(
    server: domain.LocalServer,
    progress?: LocalServerProgress,
    run?: LocalServerRunState
) {
    if (run?.status === 'running' || run?.status === 'starting') {
        return 'Already running';
    }
    if (isLocalServerInstalling(server, progress)) {
        if (server.install?.status === 'repairing') {
            return 'Repair in progress';
        }
        return 'Install in progress';
    }
    if (server.install?.status !== 'installed') {
        return 'Install required';
    }
    if (!server.eulaAccepted) {
        return 'Accept Minecraft EULA';
    }
    return null;
}

function replaceLocalServer(servers: domain.LocalServer[], next: domain.LocalServer) {
    if (!servers.some((server) => server.id === next.id)) {
        return [...servers, next];
    }
    return servers.map((server) => server.id === next.id ? next : server);
}

function localServerRunStateFromDomain(state: domain.LocalServerRunState): LocalServerRunState {
    return {
        serverId: state.serverId,
        status: state.status as LocalServerRunState['status'],
        message: state.message,
        exitCode: state.exitCode,
        startedAt: state.startedAt,
        endedAt: state.endedAt,
    };
}

function shouldShowInstallButton(profile: domain.Profile, progress?: InstallProgress) {
    if (profile.install?.status === 'installed') {
        return false;
    }
    return profile.install?.status !== 'installed' || (!!progress && !progress.done && progress.stage !== 'failed');
}

function shouldShowLocalServerInstallButton(server: domain.LocalServer, progress?: LocalServerProgress) {
    if (server.install?.status === 'installed' || server.install?.status === 'repairing') {
        return false;
    }
    return server.install?.status !== 'installed' || (!!progress && !progress.done && progress.stage !== 'failed');
}

function shouldShowLocalServerRepairButton(server: domain.LocalServer, progress?: LocalServerProgress) {
    if (server.install?.status === 'installed') {
        return !isActiveLocalServerProgress(progress);
    }
    return server.install?.status === 'repairing';
}

function shouldShowRepairButton(profile: domain.Profile, progress?: InstallProgress) {
    if (profile.install?.status === 'installed') {
        return !progress || progress.done || progress.stage === 'failed';
    }
    return profile.install?.status === 'repairing' || (!!progress && !progress.done && progress.stage !== 'failed');
}

function activeCreateImportProgress(
    progressByProfile: Record<string, InstallProgress>,
    importing: boolean
) {
    const progressValues = Object.values(progressByProfile);
    const activeModpackProgress = progressValues.find((progress) => (
        !progress.done &&
        progress.stage !== 'failed' &&
        (importing || progress.stage.startsWith('modpack-'))
    ));
    if (activeModpackProgress) {
        return activeModpackProgress;
    }
    if (!importing) {
        return undefined;
    }
    return progressValues.find((progress) => !progress.done && progress.stage !== 'failed');
}

function isJavaInstalling(progress: JavaInstallProgress | null) {
    return !!progress && !progress.done && progress.stage !== 'failed';
}

function formatBytes(value: number) {
    if (value <= 0) {
        return '0 B';
    }
    const units = ['B', 'KB', 'MB', 'GB'];
    let current = value;
    let unitIndex = 0;
    while (current >= 1024 && unitIndex < units.length - 1) {
        current /= 1024;
        unitIndex += 1;
    }
    return `${current >= 10 || unitIndex === 0 ? current.toFixed(0) : current.toFixed(1)} ${units[unitIndex]}`;
}

function formatNumber(value: number) {
    return new Intl.NumberFormat().format(value);
}

function markInstalledModrinthProjects(profileId: string, projectId: string, installedFiles: domain.ModrinthInstalledFile[]) {
    const next: Record<string, boolean> = {
        [`${profileId}:${projectId}`]: true,
    };
    for (const file of installedFiles) {
        if (file.projectId) {
            next[`${profileId}:${file.projectId}`] = true;
        }
    }
    return next;
}

function modrinthProjectProfileKey(profileId: string, projectId: string) {
    return `${profileId}:${projectId}`;
}

function modrinthVersionActionKey(projectId: string, versionId: string) {
    return versionId ? `${projectId}:${versionId}` : projectId;
}

function modrinthProjectBusy(actionKey: string, projectId: string) {
    return actionKey === projectId || actionKey.startsWith(`${projectId}:`);
}

function modrinthVersionIsCurrent(plan: domain.ModrinthUpdatePlan | undefined, version: domain.ModrinthVersion) {
    if (!plan) {
        return false;
    }
    if (plan.currentVersionId && version.id) {
        return plan.currentVersionId === version.id;
    }
    if (plan.currentVersionNumber && version.versionNumber) {
        return plan.currentVersionNumber === version.versionNumber;
    }
    if (plan.currentFileName && version.file?.fileName) {
        return modrinthModFileKey(plan.currentFileName) === modrinthModFileKey(version.file.fileName);
    }
    return false;
}

function unmarkDeletedModrinthProjects(profileId: string, result: domain.ModrinthDeleteResult) {
    const next: Record<string, boolean> = {
        [`${profileId}:${result.projectId}`]: false,
    };
    for (const file of result.deletedFiles ?? []) {
        if (file.projectId) {
            next[`${profileId}:${file.projectId}`] = false;
        }
    }
    return next;
}

function modrinthUpdatePlansByProject(plans: domain.ModrinthUpdatePlan[]) {
    const byProject: Record<string, domain.ModrinthUpdatePlan> = {};
    for (const plan of plans ?? []) {
        byProject[plan.projectId] = plan;
    }
    return byProject;
}

function modrinthUpdatePlansByFile(plans: domain.ModrinthUpdatePlan[]) {
    const byFile: Record<string, domain.ModrinthUpdatePlan> = {};
    for (const plan of plans ?? []) {
        if (plan.currentFileName) {
            byFile[modrinthModFileKey(plan.currentFileName)] = plan;
        }
    }
    return byFile;
}

function modrinthModFileKey(fileName: string) {
    return fileName.trim().toLowerCase().replace(/\.disabled$/, '');
}

function modBrowseQuery(value: string) {
    return value
        .trim()
        .replace(/\.jar(\.disabled)?$/i, '')
        .replace(/[-_]+/g, ' ')
        .trim();
}

function modrinthUpdateStatus(plan?: domain.ModrinthUpdatePlan) {
    if (!plan) {
        return 'Local only';
    }
    if (plan.checkError) {
        return 'Check failed';
    }
    if (plan.updateAvailable) {
        return 'Update available';
    }
    return 'Up to date';
}

function selectedDeleteFiles(files: domain.ModrinthDeleteFile[]) {
    const selected: Record<string, boolean> = {};
    for (const file of files) {
        selected[file.fileName] = true;
    }
    return selected;
}

function selectedInstallDependencies(dependencies: domain.ModrinthRequiredDependency[]) {
    const selected: Record<string, boolean> = {};
    for (const dependency of dependencies) {
        selected[installDependencyKey(dependency)] = true;
    }
    return selected;
}

function installDependencyKey(dependency: domain.ModrinthRequiredDependency) {
    return dependency.versionId || dependency.projectId || dependency.fileName;
}

function requiredDependencyIDs(dependencies: domain.ModrinthRequiredDependency[]) {
    return dependencies.map((dependency) => installDependencyKey(dependency));
}

function modrinthInstallMessage(result: domain.ModrinthInstallResult) {
    const title = result.projectTitle || result.fileName;
    const dependencies = modrinthInstalledDependencies(result);
    const installedDependencies = dependencies.filter((file) => !file.alreadyPresent);
    const existingDependencies = dependencies.filter((file) => file.alreadyPresent);
    if (installedDependencies.length > 0) {
        return `Installed ${title} with ${installedDependencies.length} dependencies.`;
    }
    if (existingDependencies.length > 0) {
        return `Installed ${title}; ${existingDependencies.length} dependencies were already present.`;
    }
    return `Installed ${title}.`;
}

function modrinthInstallLogMessage(result: domain.ModrinthInstallResult) {
    const installedFiles = result.installedFiles ?? [];
    const downloaded = installedFiles.filter((file) => !file.alreadyPresent).length;
    const existing = installedFiles.filter((file) => file.alreadyPresent).length;
    const skipped = result.skippedDependencies?.length ?? 0;
    const parts = [`Installed ${result.fileName}`];
    if (downloaded > 1) {
        parts.push(`${downloaded - 1} dependency downloads`);
    }
    if (existing > 0) {
        parts.push(`${existing} already present`);
    }
    if (skipped > 0) {
        parts.push(`${skipped} skipped`);
    }
    return parts.join(', ') + '.';
}

function modrinthDependencyInstallDetails(result: domain.ModrinthInstallResult) {
    const dependencies = modrinthInstalledDependencies(result);
    if (dependencies.length === 0) {
        return '';
    }
    const names = dependencies
        .map((file) => `${file.displayName || file.fileName}${file.alreadyPresent ? ' (already present)' : ''}`)
        .join(', ');
    return `Dependencies: ${names}.`;
}

function modrinthUpdateMessage(result: domain.ModrinthUpdateResult) {
    if (!result.updated) {
        return `${result.projectTitle || result.projectId} is already up to date.`;
    }
    return `Updated ${result.projectTitle || result.projectId}.`;
}

function modrinthUpdateLogMessage(result: domain.ModrinthUpdateResult) {
    if (!result.updated) {
        return `${result.projectTitle || result.projectId} is already up to date.`;
    }
    const downloaded = result.installedFiles?.filter((file) => !file.alreadyPresent).length ?? 0;
    const deleted = result.deletedFiles?.length ?? 0;
    const skipped = result.skippedFiles?.length ?? 0;
    const parts = [`Updated ${result.projectTitle || result.projectId}`];
    if (result.oldFileName && result.newFileName && result.oldFileName !== result.newFileName) {
        parts.push(`${result.oldFileName} -> ${result.newFileName}`);
    }
    if (downloaded > 0) {
        parts.push(`${downloaded} downloads`);
    }
    if (deleted > 0) {
        parts.push(`${deleted} old files removed`);
    }
    if (skipped > 0) {
        parts.push(`${skipped} old files kept`);
    }
    return parts.join(', ') + '.';
}

function modrinthInstalledDependencies(result: domain.ModrinthInstallResult) {
    return (result.installedFiles ?? []).filter((file) => !!file.dependencyType);
}

function modrinthDeleteMessage(result: domain.ModrinthDeleteResult) {
    const deleted = result.deletedFiles?.length ?? 0;
    const skipped = result.skippedFiles?.length ?? 0;
    if (deleted > 0 && skipped > 0) {
        return `Deleted ${result.projectTitle || result.projectId}; ${skipped} files were kept.`;
    }
    if (deleted > 0) {
        return `Deleted ${result.projectTitle || result.projectId}.`;
    }
    return `No files deleted for ${result.projectTitle || result.projectId}.`;
}

function modrinthDeleteLogMessage(result: domain.ModrinthDeleteResult) {
    const deleted = result.deletedFiles?.length ?? 0;
    const skipped = result.skippedFiles?.length ?? 0;
    const parts = [`Deleted ${deleted} files for ${result.projectTitle || result.projectId}`];
    if (skipped > 0) {
        parts.push(`${skipped} kept`);
    }
    return parts.join(', ') + '.';
}

function formatDateTime(value: string) {
    if (!value) {
        return 'Unknown';
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return value;
    }
    return date.toLocaleString();
}

function installStatusText(profile: domain.Profile) {
    switch (profile.install?.status) {
        case 'installed':
            return 'Installed';
        case 'base-installed':
            return 'Base installed';
        case 'installing':
            return 'Installing';
        case 'repairing':
            return 'Repairing';
        case 'failed':
            return 'Failed';
        default:
            return 'Not installed';
    }
}

function modpackImportMessage(result: domain.ModpackImportResult) {
    const name = result.name || result.profile?.name || 'Modpack';
    const version = result.versionId ? ` ${result.versionId}` : '';
    return `Imported ${name}${version}: ${result.filesInstalled} files, ${result.filesSkipped} cached, ${result.overridesInstalled} overrides.`;
}

function modpackExportMessage(result: domain.ModpackExportResult) {
    const name = result.name || 'Modpack';
    const version = result.versionId ? ` ${result.versionId}` : '';
    return `Exported ${name}${version}: ${result.filesExported} Modrinth files, ${result.overridesExported} overrides.`;
}

function errorText(err: unknown) {
    if (err instanceof Error) {
        return err.message;
    }
    return String(err);
}

function pickCurrentValue(current: string, options: domain.VersionOption[]) {
    if (options.some((option) => option.id === current)) {
        return current;
    }
    const latestStable = options.find((option) => option.latest && option.stable);
    if (latestStable) {
        return latestStable.id;
    }
    const latest = options.find((option) => option.latest);
    return latest?.id ?? options[0]?.id ?? current;
}

function fallbackFabricLoaderVersions(): domain.VersionOption[] {
    return [{
        id: 'latest',
        label: 'Latest compatible (resolved during install)',
        type: 'fabric-loader',
        stable: true,
        latest: true,
    }];
}

function fallbackQuiltLoaderVersions(): domain.VersionOption[] {
    return [{
        id: 'latest',
        label: 'Latest compatible (resolved during install)',
        type: 'quilt-loader',
        stable: true,
        latest: true,
    }];
}

function fallbackForgeLoaderVersions(): domain.VersionOption[] {
    return [{
        id: 'latest',
        label: 'Latest compatible (resolved during install)',
        type: 'forge-loader',
        stable: true,
        latest: true,
    }];
}

function fallbackNeoForgeLoaderVersions(): domain.VersionOption[] {
    return [{
        id: 'latest',
        label: 'Latest compatible (resolved during install)',
        type: 'neoforge-loader',
        stable: true,
        latest: true,
    }];
}

function nextCreateVersionSelection(
    current: typeof defaultCreateForm,
    minecraftVersions: domain.VersionOption[],
    fabricVersions: domain.VersionOption[],
    quiltVersions: domain.VersionOption[],
    forgeVersions: domain.VersionOption[],
    neoForgeVersions: domain.VersionOption[]
) {
    const minecraftVersion = minecraftVersions.length > 0
        ? pickCurrentValue(current.minecraftVersion, minecraftVersions)
        : current.minecraftVersion;
    return {
        minecraftVersion,
        loaderVersion: pickCurrentValue(
            current.loaderVersion || 'latest',
            loaderVersionOptions(current.loaderType, fabricVersions, quiltVersions, forgeVersions, neoForgeVersions, minecraftVersion)
        ),
    };
}

function loaderVersionOptions(
    loaderType: string,
    fabricVersions: domain.VersionOption[],
    quiltVersions: domain.VersionOption[],
    forgeVersions: domain.VersionOption[],
    neoForgeVersions: domain.VersionOption[],
    minecraftVersion: string
) {
    if (loaderType === 'neoforge') {
        const matchingNeoForgeVersions = neoForgeVersions.filter((version) => neoForgeMinecraftVersion(version.id) === minecraftVersion);
        return matchingNeoForgeVersions.length > 0 ? matchingNeoForgeVersions : fallbackNeoForgeLoaderVersions();
    }
    if (loaderType === 'forge') {
        const matchingForgeVersions = forgeVersions.filter((version) => version.id.startsWith(`${minecraftVersion}-`));
        return matchingForgeVersions.length > 0 ? matchingForgeVersions : fallbackForgeLoaderVersions();
    }
    if (loaderType === 'quilt') {
        return quiltVersions.length > 0 ? quiltVersions : fallbackQuiltLoaderVersions();
    }
    if (loaderType === 'fabric') {
        return fabricVersions.length > 0 ? fabricVersions : fallbackFabricLoaderVersions();
    }
    return [];
}

function neoForgeMinecraftVersion(version: string) {
    const parts = version.trim().split('.');
    if (parts.length < 2) {
        return '';
    }
    const major = leadingDigits(parts[0]);
    const minor = leadingDigits(parts[1]);
    if (!major || !minor) {
        return '';
    }
    return `1.${major}.${minor}`;
}

function leadingDigits(value: string) {
    const match = value.match(/^\d+/);
    return match ? match[0] : '';
}

function versionCatalogStatusText(catalog: domain.VersionCatalog) {
    const sourceStatus = versionCatalogSourceStatus(catalog);
    const suffix = sourceStatus ? ` (${sourceStatus})` : '';
    if ((catalog.warnings?.length ?? 0) > 0) {
        return `Catalog ready with warnings${suffix}`;
    }
    return `Catalog ready${suffix}`;
}

function versionCatalogSummary(catalog: domain.VersionCatalog) {
    const minecraftCount = catalog.minecraftVersions?.length ?? 0;
    const fabricCount = catalog.fabricLoaderVersions?.length ?? 0;
    const quiltCount = catalog.quiltLoaderVersions?.length ?? 0;
    const forgeCount = catalog.forgeLoaderVersions?.length ?? 0;
    const neoForgeCount = catalog.neoForgeLoaderVersions?.length ?? 0;
    return `${minecraftCount} Minecraft versions from ${sourceLabel(catalog.minecraftSource)}, ${fabricCount} Fabric loaders from ${sourceLabel(catalog.fabricLoaderSource)}, ${quiltCount} Quilt loaders from ${sourceLabel(catalog.quiltLoaderSource)}, ${forgeCount} Forge loaders from ${sourceLabel(catalog.forgeLoaderSource)}, ${neoForgeCount} NeoForge loaders from ${sourceLabel(catalog.neoForgeLoaderSource)}`;
}

function versionCatalogSourceStatus(catalog: domain.VersionCatalog) {
    const entries = [
        sourceLabel(catalog.minecraftSource),
        sourceLabel(catalog.fabricLoaderSource),
        sourceLabel(catalog.quiltLoaderSource),
        sourceLabel(catalog.forgeLoaderSource),
        sourceLabel(catalog.neoForgeLoaderSource),
    ].filter((source) => source !== 'empty');
    if (entries.length === 0) {
        return '';
    }
    if (entries.every((source) => source === entries[0])) {
        return entries[0];
    }
    return 'mixed sources';
}

function sourceLabel(source: string) {
    switch (source) {
        case 'network':
            return 'network';
        case 'cache':
            return 'cached';
        case 'fallback':
            return 'fallback';
        default:
            return 'empty';
    }
}

function settingsDraft(settings: domain.Settings): SettingsDraft {
    return {
        dataDir: settings.dataDir,
        javaPath: settings.javaPath,
        defaultMemory: {
            minMB: settings.defaultMemory.minMB,
            maxMB: settings.defaultMemory.maxMB,
        },
        network: {
            retryCount: settings.network.retryCount,
            metadataTtlHours: settings.network.metadataTtlHours,
        },
    };
}

function profileSettingsDraftFrom(profile: domain.Profile): ProfileSettingsDraft {
    return {
        gameDir: profile.gameDir,
        minMB: profile.memory.minMB,
        maxMB: profile.memory.maxMB,
    };
}

function accountDraft(account?: domain.AccountConfig | null): AccountDraft {
    return {
        mode: account?.mode || defaultAccount.mode,
        offlineName: account?.offlineName || defaultAccount.offlineName,
        offlineUuid: account?.offlineUuid || defaultAccount.offlineUuid,
    };
}

export default App
