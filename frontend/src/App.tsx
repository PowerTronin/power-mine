import {useEffect, useMemo, useRef, useState, type CSSProperties, type FormEvent, type KeyboardEvent, type ReactNode} from 'react';
import rehypeRaw from 'rehype-raw';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import './App.css';
import {
    AppInfo,
    ClearLauncherLogs,
    CreateProfile,
    DeleteProfile,
    DeleteModrinthModFiles,
    DeleteProfileMod,
    ExportModrinthModpack,
    GetAccount,
    GetCachedVersionCatalog,
    GetModrinthProject,
    GetProfileJavaRuntime,
    GetSettings,
    ImportProfileMod,
    ImportModrinthModpack,
    InstallJava,
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
    ListLauncherLogs,
    IsLogsWindow,
    OpenLauncherLogsWindow,
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
    RepairProfile,
    RecordLauncherLog,
    SaveAccount,
    SaveSettings,
    SearchModrinthMods,
    SelectProfile,
    SetProfileModEnabled,
    StopProfile,
    UpdateModrinthModFile,
    UpdateModrinthModFiles,
    UpdateModrinthModVersionFiles,
    UpdateProfile,
    ValidateJava
} from "../wailsjs/go/main/App";
import {domain} from "../wailsjs/go/models";
import {EventsOn as RuntimeEventsOn} from "../wailsjs/runtime/runtime";
import logoWhite from './assets/images/pt-logo-fin-white.svg';

type Screen = 'home' | 'library' | 'create' | 'account' | 'logs' | 'settings' | 'browse';

const markdownAllowedElements = [
    'a',
    'blockquote',
    'br',
    'code',
    'del',
    'details',
    'em',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'hr',
    'img',
    'li',
    'ol',
    'p',
    'pre',
    'strong',
    'summary',
    'table',
    'tbody',
    'td',
    'th',
    'thead',
    'tr',
    'ul',
];

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
};

type LaunchEvent = {
    profileId: string;
    status: 'starting' | 'running' | 'stopping' | 'stopped' | 'failed';
    stream?: string;
    message: string;
    exitCode?: number;
    time: string;
};

type LaunchState = {
    profileId: string;
    status: 'starting' | 'running' | 'stopping' | 'stopped' | 'failed';
    message: string;
    exitCode?: number;
    startedAt?: string;
    endedAt?: string;
};

type LogLevelFilter = 'all' | LauncherLog['level'];
type LogsMode = 'launcher' | 'game';
type WindowMode = 'checking' | 'main' | 'logs';
type HealthTone = 'ok' | 'warn' | 'error' | 'busy' | 'idle';

type LogAppearance = {
    fontSize: number;
    timeWidth: number;
    levelWidth: number;
    sourceWidth: number;
    profileWidth: number;
    consoleBg: string;
    timeColor: string;
    levelColor: string;
    sourceColor: string;
    profileColor: string;
    messageColor: string;
    severityColors: boolean;
};

type ProfileHealthItem = {
    key: string;
    label: string;
    status: string;
    detail: string;
    tone: HealthTone;
    actionLabel?: string;
    actionClass?: 'primary' | 'danger';
    actionDisabled?: boolean;
    onAction?: () => void;
};

type ProfileHealthProps = {
    profile: domain.Profile;
    progress?: InstallProgress;
    launch?: LaunchState;
    javaRuntime?: domain.ProfileJavaRuntime;
    javaInstallProgress: JavaInstallProgress | null;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    onInstall: (id: string) => void;
    onRepair: (id: string) => void;
    onInstallJava: (version: number) => void;
    onStop: (id: string) => void;
    onBrowseMods: (profileId: string) => void;
    onOpenLogs: () => void;
};

type SelectOption = {
    value: string;
    label: string;
    disabled?: boolean;
};

function onRuntimeEvent<T>(eventName: string, callback: (event: T) => void) {
    if (typeof globalThis === 'undefined' || !(globalThis as { runtime?: unknown }).runtime) {
        return () => undefined;
    }
    return RuntimeEventsOn(eventName, callback as (...data: unknown[]) => void);
}

function hasWailsBackend() {
    return typeof globalThis !== 'undefined'
        && !!(globalThis as { go?: { main?: { App?: unknown } } }).go?.main?.App;
}

function launcherLogsFromBackend(logs?: domain.LauncherLog[] | null): LauncherLog[] {
    return (logs ?? []).map((log) => ({
        id: log.id,
        time: log.time,
        level: launcherLogLevel(log.level),
        source: log.source,
        message: log.message,
        profileId: log.profileId,
    }));
}

function launcherLogLevel(level: string): LauncherLog['level'] {
    if (level === 'error' || level === 'success') {
        return level;
    }
    return 'info';
}

function loadLogAppearance(): LogAppearance {
    if (typeof localStorage === 'undefined') {
        return defaultLogAppearance;
    }
    try {
        const raw = localStorage.getItem(logAppearanceStorageKey);
        if (!raw) {
            return defaultLogAppearance;
        }
        return sanitizeLogAppearance(JSON.parse(raw));
    } catch {
        return defaultLogAppearance;
    }
}

function saveLogAppearance(appearance: LogAppearance) {
    if (typeof localStorage === 'undefined') {
        return;
    }
    localStorage.setItem(logAppearanceStorageKey, JSON.stringify(appearance));
}

function sanitizeLogAppearance(input: Partial<LogAppearance>): LogAppearance {
    return {
        fontSize: clampNumber(input.fontSize, 10, 22, defaultLogAppearance.fontSize),
        timeWidth: clampNumber(input.timeWidth, 46, 180, defaultLogAppearance.timeWidth),
        levelWidth: clampNumber(input.levelWidth, 46, 160, defaultLogAppearance.levelWidth),
        sourceWidth: clampNumber(input.sourceWidth, 60, 260, defaultLogAppearance.sourceWidth),
        profileWidth: clampNumber(input.profileWidth, 60, 280, defaultLogAppearance.profileWidth),
        consoleBg: cleanColor(input.consoleBg, defaultLogAppearance.consoleBg),
        timeColor: cleanColor(input.timeColor, defaultLogAppearance.timeColor),
        levelColor: cleanColor(input.levelColor, defaultLogAppearance.levelColor),
        sourceColor: cleanColor(input.sourceColor, defaultLogAppearance.sourceColor),
        profileColor: cleanColor(input.profileColor, defaultLogAppearance.profileColor),
        messageColor: cleanColor(input.messageColor, defaultLogAppearance.messageColor),
        severityColors: input.severityColors ?? defaultLogAppearance.severityColors,
    };
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
    const number = Number(value);
    if (!Number.isFinite(number)) {
        return fallback;
    }
    return Math.min(max, Math.max(min, number));
}

function cleanColor(value: unknown, fallback: string) {
    if (typeof value !== 'string' || !/^#[0-9a-f]{6}$/i.test(value)) {
        return fallback;
    }
    return value;
}

function logAppearanceStyle(appearance: LogAppearance): CSSProperties {
    const style = {
        '--log-font-size': `${appearance.fontSize}px`,
        '--log-time-width': `${appearance.timeWidth}px`,
        '--log-level-width': `${appearance.levelWidth}px`,
        '--log-source-width': `${appearance.sourceWidth}px`,
        '--log-profile-width': `${appearance.profileWidth}px`,
        '--log-console-bg': appearance.consoleBg,
        '--log-time-color': appearance.timeColor,
        '--log-source-color': appearance.sourceColor,
        '--log-profile-color': appearance.profileColor,
        '--log-message-color': appearance.messageColor,
    } as CSSProperties & Record<string, string>;
    if (!appearance.severityColors) {
        style['--log-level-color'] = appearance.levelColor;
    }
    return style;
}

function CommandSelect({
    value,
    options,
    onChange,
    disabled = false,
    ariaLabel
}: {
    value: string;
    options: SelectOption[];
    onChange: (value: string) => void;
    disabled?: boolean;
    ariaLabel?: string;
}) {
    const [open, setOpen] = useState(false);
    const [activeIndex, setActiveIndex] = useState(0);
    const rootRef = useRef<HTMLDivElement | null>(null);
    const selectedIndex = Math.max(0, options.findIndex((option) => option.value === value));
    const selected = options[selectedIndex];

    useEffect(() => {
        if (!open) {
            return;
        }
        function handlePointerDown(event: MouseEvent) {
            if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
                setOpen(false);
            }
        }
        document.addEventListener('mousedown', handlePointerDown);
        return () => document.removeEventListener('mousedown', handlePointerDown);
    }, [open]);

    useEffect(() => {
        setActiveIndex(selectedIndex);
    }, [selectedIndex, options.length]);

    function nextEnabledIndex(direction: 1 | -1) {
        if (options.length === 0) {
            return 0;
        }
        for (let step = 1; step <= options.length; step += 1) {
            const index = (activeIndex + step * direction + options.length) % options.length;
            if (!options[index]?.disabled) {
                return index;
            }
        }
        return activeIndex;
    }

    function choose(index: number) {
        const option = options[index];
        if (!option || option.disabled) {
            return;
        }
        onChange(option.value);
        setOpen(false);
    }

    function handleKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            setOpen(true);
            setActiveIndex(nextEnabledIndex(1));
            return;
        }
        if (event.key === 'ArrowUp') {
            event.preventDefault();
            setOpen(true);
            setActiveIndex(nextEnabledIndex(-1));
            return;
        }
        if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            if (open) {
                choose(activeIndex);
            } else {
                setOpen(true);
            }
            return;
        }
        if (event.key === 'Escape') {
            setOpen(false);
        }
    }

    return (
        <div className="command-select" ref={rootRef}>
            <button
                type="button"
                className="command-select-trigger"
                disabled={disabled || options.length === 0}
                aria-label={ariaLabel}
                aria-haspopup="listbox"
                aria-expanded={open}
                onClick={() => setOpen((current) => !current)}
                onKeyDown={handleKeyDown}
            >
                <span>{selected?.label ?? value ?? 'Select'}</span>
                <span className="command-select-arrow" aria-hidden="true"/>
            </button>
            {open && (
                <div className="command-select-menu" role="listbox">
                    {options.map((option, index) => (
                        <button
                            key={`${option.value}-${index}`}
                            type="button"
                            className={[
                                'command-select-option',
                                option.value === value ? 'selected' : '',
                                index === activeIndex ? 'active' : '',
                            ].filter(Boolean).join(' ')}
                            disabled={option.disabled}
                            role="option"
                            aria-selected={option.value === value}
                            onMouseEnter={() => setActiveIndex(index)}
                            onClick={() => choose(index)}
                        >
                            {option.label}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

const navItems: Array<{ id: Screen; label: string; mark: string }> = [
    {id: 'home', label: 'Home', mark: 'H'},
    {id: 'library', label: 'Library', mark: 'L'},
    {id: 'create', label: 'Create', mark: '+'},
    {id: 'account', label: 'Account', mark: 'A'},
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

const defaultAccount: AccountDraft = {
    mode: 'offline',
    offlineName: 'Player',
    offlineUuid: '',
};

const logAppearanceStorageKey = 'power-mine:log-appearance:v1';

const defaultLogAppearance: LogAppearance = {
    fontSize: 13,
    timeWidth: 82,
    levelWidth: 72,
    sourceWidth: 112,
    profileWidth: 150,
    consoleBg: '#050505',
    timeColor: '#8f8f8f',
    levelColor: '#ffffff',
    sourceColor: '#b8b8b8',
    profileColor: '#9f9f9f',
    messageColor: '#d6d6d6',
    severityColors: true,
};

function App() {
    const [windowMode, setWindowMode] = useState<WindowMode>(hasWailsBackend() ? 'checking' : 'main');
    const [logsFocusMode, setLogsFocusMode] = useState(false);
    const [screen, setScreen] = useState<Screen>('home');
    const [info, setInfo] = useState<domain.AppInfo | null>(null);
    const [settings, setSettings] = useState<SettingsDraft | null>(null);
    const [account, setAccount] = useState<AccountDraft>(defaultAccount);
    const [profiles, setProfiles] = useState<domain.Profile[]>([]);
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
    const [launcherLogs, setLauncherLogs] = useState<LauncherLog[]>([]);
    const [selectedProfileId, setSelectedProfileId] = useState('');
    const [profileSettingsId, setProfileSettingsId] = useState('');
    const [profileSettingsDraft, setProfileSettingsDraft] = useState<ProfileSettingsDraft | null>(null);
    const [modActionKey, setModActionKey] = useState('');
    const [gameLogActionKey, setGameLogActionKey] = useState('');
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
    const [createForm, setCreateForm] = useState(defaultCreateForm);

    const selectedProfile = useMemo(
        () => profiles.find((profile) => profile.id === selectedProfileId) ?? profiles[0],
        [profiles, selectedProfileId]
    );
    const browseProfile = useMemo(
        () => profiles.find((profile) => profile.id === browseProfileId) ?? selectedProfile,
        [profiles, browseProfileId, selectedProfile]
    );
    const selectedProgress = selectedProfile ? installProgress[selectedProfile.id] : undefined;
    const selectedLaunch = selectedProfile ? launchStates[selectedProfile.id] : undefined;
    const selectedJavaRuntime = selectedProfile ? profileJavaRuntimes[selectedProfile.id] : undefined;
    const selectedModList = selectedProfile ? profileModLists[selectedProfile.id] : undefined;
    const selectedModrinthUpdatePlans = selectedProfile ? modrinthUpdatePlans[selectedProfile.id] ?? [] : [];
    const selectedSettingsOpen = !!selectedProfile && profileSettingsId === selectedProfile.id;
    const selectedLogs = useMemo(
        () => selectedProfile
            ? launcherLogs.filter((log) => !log.profileId || log.profileId === selectedProfile.id)
            : launcherLogs,
        [launcherLogs, selectedProfile]
    );
    const createImportProgress = useMemo(
        () => activeCreateImportProgress(installProgress, modpackImporting),
        [installProgress, modpackImporting]
    );
    const browseProfileForModrinth = browseProfile?.id;

    useEffect(() => {
        if (!hasWailsBackend()) {
            setWindowMode('main');
            return;
        }
        let cancelled = false;
        IsLogsWindow()
            .then((isLogsWindow) => {
                if (!cancelled) {
                    setWindowMode(isLogsWindow ? 'logs' : 'main');
                }
            })
            .catch(() => {
                if (!cancelled) {
                    setWindowMode('main');
                }
            });
        return () => {
            cancelled = true;
        };
    }, []);

    useEffect(() => {
        if (!hasWailsBackend() || windowMode !== 'main') {
            return;
        }
        void (async () => {
            await loadLauncherLogs();
            refreshApp();
            refreshVersionOptions();
            validateJava();
        })();
    }, [windowMode]);

    useEffect(() => {
        if (!hasWailsBackend() || windowMode !== 'logs') {
            return;
        }
        let cancelled = false;
        async function refreshLogsWindow() {
            try {
                const [profileList, logs] = await Promise.all([
                    ListProfiles(),
                    ListLauncherLogs(),
                ]);
                if (!cancelled) {
                    setProfiles(profileList.profiles ?? []);
                    setSelectedProfileId(profileList.selectedProfileId);
                    setLauncherLogs(launcherLogsFromBackend(logs));
                    setError('');
                }
            } catch (err) {
                if (!cancelled) {
                    setError(errorText(err));
                }
            }
        }
        void refreshLogsWindow();
        const interval = window.setInterval(refreshLogsWindow, 1000);
        return () => {
            cancelled = true;
            window.clearInterval(interval);
        };
    }, [windowMode]);

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
        if (windowMode === 'main' && browseProfileForModrinth) {
            refreshInstalledModrinthProjects(browseProfileForModrinth);
            refreshModrinthUpdates(browseProfileForModrinth);
        }
    }, [browseProfileForModrinth, windowMode]);

    useEffect(() => {
        if (windowMode === 'main' && selectedProfile?.id) {
            refreshProfileJavaRuntime(selectedProfile.id);
            refreshProfileMods(selectedProfile.id);
            refreshModrinthUpdates(selectedProfile.id);
        }
    }, [selectedProfile?.id, selectedProfile?.minecraftVersion, selectedProfile?.install?.status, windowMode]);

    useEffect(() => {
        if (windowMode !== 'main') {
            return;
        }
        return onRuntimeEvent<InstallProgress>('install:progress', (event) => {
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
    }, [windowMode]);

    useEffect(() => {
        if (windowMode !== 'main') {
            return;
        }
        return onRuntimeEvent<JavaInstallProgress>('java:progress', (event) => {
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
    }, [windowMode]);

    useEffect(() => {
        if (windowMode !== 'main') {
            return;
        }
        return onRuntimeEvent<LaunchEvent>('launch:event', (event) => {
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
    }, [windowMode]);

    useEffect(() => {
        if (screen !== 'logs' && logsFocusMode) {
            setLogsFocusMode(false);
        }
    }, [screen, logsFocusMode]);

    function appendLog(entry: Omit<LauncherLog, 'id' | 'time'>) {
        const next: LauncherLog = {
            ...entry,
            id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
            time: new Date().toLocaleTimeString(),
        };
        setLauncherLogs((current) => [next, ...current]);
        if (hasWailsBackend()) {
            void RecordLauncherLog(next).catch(() => undefined);
        }
    }

    async function loadLauncherLogs() {
        try {
            const logs = await ListLauncherLogs();
            setLauncherLogs(launcherLogsFromBackend(logs));
        } catch {
            // Launcher log history is non-critical; live events will still appear.
        }
    }

    async function openLauncherLogsWindow() {
        try {
            setError('');
            await OpenLauncherLogsWindow();
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Launcher logs',
                message: `Open logs window failed: ${text}`,
            });
        }
    }

    async function clearLauncherLogs() {
        setLauncherLogs([]);
        if (!hasWailsBackend()) {
            return;
        }
        try {
            await ClearLauncherLogs();
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Launcher logs',
                message: `Clear logs failed: ${text}`,
            });
        }
    }

    async function refreshApp() {
        try {
            setError('');
            const [nextInfo, nextSettings, nextAccount, profileList] = await Promise.all([
                AppInfo(),
                GetSettings(),
                GetAccount(),
                ListProfiles()
            ]);
            setInfo(nextInfo);
            setSettings(settingsDraft(nextSettings));
            setAccount(accountDraft(nextAccount));
            setProfiles(profileList.profiles ?? []);
            setSelectedProfileId(profileList.selectedProfileId);
            setBrowseProfileId((current) => current || profileList.selectedProfileId || profileList.profiles?.[0]?.id || '');
            setCreateForm((current) => ({
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
            setScreen('library');
        } catch (err) {
            setError(errorText(err));
            appendLog({
                level: 'error',
                source: 'Profile',
                message: `Create profile failed: ${errorText(err)}`,
            });
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
            setScreen('library');
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
        setScreen('browse');
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

    async function stopProfile(id: string) {
        try {
            setError('');
            setMessage('');
            setLaunchStates((current) => ({
                ...current,
                [id]: {
                    ...current[id],
                    profileId: id,
                    status: 'stopping',
                    message: 'Stop requested',
                },
            }));
            appendLog({
                level: 'info',
                source: 'Launch',
                message: 'Stop requested.',
                profileId: id,
            });
            const state = await StopProfile(id);
            setLaunchStates((current) => ({
                ...current,
                [id]: {
                    ...current[id],
                    profileId: state.profileId,
                    status: state.status as LaunchState['status'],
                    message: state.message,
                    exitCode: state.exitCode,
                    startedAt: current[id]?.startedAt ?? state.startedAt,
                    endedAt: state.endedAt,
                },
            }));
        } catch (err) {
            const text = errorText(err);
            setError(text);
            appendLog({
                level: 'error',
                source: 'Launch',
                message: `Stop failed: ${text}`,
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

    if (windowMode === 'checking') {
        return (
            <div className="logs-window-shell loading">
                <p className="eyebrow">Power Mine</p>
                <h1>Opening interface</h1>
            </div>
        );
    }

    if (windowMode === 'logs') {
        return (
            <LauncherLogsWindow
                logs={launcherLogs}
                profiles={profiles}
                error={error}
                gameLogLists={profileGameLogLists}
                gameLogContents={profileGameLogContents}
                gameLogActionKey={gameLogActionKey}
                onClearLogs={clearLauncherLogs}
                onRefreshGameLogs={refreshProfileGameLogs}
                onReadGameLog={readProfileGameLog}
                onOpenLogsFolder={openProfileLogsFolder}
            />
        );
    }

    return (
        <div className={screen === 'logs' && logsFocusMode ? 'app-shell logs-focus-active' : 'app-shell'}>
            <aside className="rail">
                <div className="brand">
                    <div className="brand-mark">
                        <img src={logoWhite} alt="" aria-hidden="true"/>
                    </div>
                    <div>
                        <strong>{info?.name ?? 'Power Mine'}</strong>
                        <span>version: {info?.version ?? '0.1.0'}</span>
                    </div>
                </div>
                <nav className="nav-list" aria-label="Primary navigation">
                    {navItems.map((item) => (
                        <button
                            key={item.id}
                            type="button"
                            className={screen === item.id ? 'nav-item active' : 'nav-item'}
                            onClick={() => setScreen(item.id)}
                        >
                            <span>{item.mark}</span>
                            {item.label}
                        </button>
                    ))}
                </nav>
            </aside>

            <main className="workspace">
                <header className="topbar">
                    <div>
                        <p className="eyebrow">macOS + Linux MVP</p>
                        <h1>{titleFor(screen)}</h1>
                    </div>
                    <div className="account-pill">
                        <span className="status-dot"/>
                        {accountLabel(account)}
                    </div>
                </header>

                {error && <div className="banner error">{error}</div>}
                {message && <div className="banner success">{message}</div>}

                {screen === 'home' && (
                    <HomePanel
                        profiles={profiles}
                        selectedProfile={selectedProfile}
                        selectedProfileId={selectedProfileId}
                        account={account}
                        javaStatus={javaStatus}
                        javaPath={settings?.javaPath ?? 'java'}
                        installProgress={installProgress}
                        javaInstallProgress={javaInstallProgress}
                        profileJavaRuntimes={profileJavaRuntimes}
                        modList={selectedModList}
                        modrinthUpdatePlans={selectedModrinthUpdatePlans}
                        launchStates={launchStates}
                        selectedLogs={selectedLogs}
                        onSelectProfile={selectProfile}
                        onInstallProfile={installProfile}
                        onRepairProfile={repairProfile}
                        onInstallJava={installJava}
                        onLaunchProfile={launchProfile}
                        onStopProfile={stopProfile}
                        onOpenBrowse={(profileId) => {
                            setBrowseProfileId(profileId);
                            setScreen('browse');
                        }}
                        onOpenCreate={() => setScreen('create')}
                        onOpenLibrary={(profile) => {
                            if (profile) {
                                openProfileSettings(profile);
                            }
                            setScreen('library');
                        }}
                        onOpenLogs={() => setScreen('logs')}
                        onOpenLogsWindow={openLauncherLogsWindow}
                    />
                )}

                {screen === 'library' && (
                    <section className="library-layout">
                        <div className="profile-list">
                            {profiles.length === 0 && <EmptyState title="No profiles" action="Library is empty."/>}
                            {profiles.map((profile) => (
                                <button
                                    key={profile.id}
                                    className={selectedProfile?.id === profile.id ? 'profile-row active' : 'profile-row'}
                                    type="button"
                                    onClick={() => selectProfile(profile.id)}
                                >
                                    <strong>{profile.name}</strong>
                                    <span>{profileSubtitle(profile)}</span>
                                    <small>{installStatusText(profile)}</small>
                                </button>
                            ))}
                        </div>
                        <ProfileDetail
                            profile={selectedProfile}
                            progress={selectedProgress}
                            launch={selectedLaunch}
                            javaRuntime={selectedJavaRuntime}
                            javaInstallProgress={javaInstallProgress}
                            modList={selectedModList}
                            modrinthUpdatePlans={selectedModrinthUpdatePlans}
                            modActionKey={modActionKey}
                            settingsOpen={selectedSettingsOpen}
                            settingsDraft={selectedSettingsOpen ? profileSettingsDraft : null}
                            onDelete={deleteProfile}
                            onInstall={installProfile}
                            onRepair={repairProfile}
                            onInstallJava={installJava}
                            onLaunch={launchProfile}
                            onStop={stopProfile}
                            onOpenLogs={() => setScreen('logs')}
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
                    </section>
                )}

                {screen === 'create' && (
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
                        <label>
                            Minecraft version
                            <CommandSelect
                                value={createForm.minecraftVersion}
                                disabled={minecraftVersions.length === 0}
                                ariaLabel="Minecraft version"
                                options={minecraftVersions.map((version) => ({
                                    value: version.id,
                                    label: version.label,
                                }))}
                                onChange={(minecraftVersion) => {
                                    setCreateForm({
                                        ...createForm,
                                        minecraftVersion,
                                        loaderVersion: pickCurrentValue(
                                            createForm.loaderVersion || 'latest',
                                            loaderVersionOptions(createForm.loaderType, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, minecraftVersion)
                                        ),
                                    });
                                }}
                            />
                        </label>
                        <label>
                            Loader
                            <CommandSelect
                                value={createForm.loaderType}
                                ariaLabel="Loader"
                                options={[
                                    {value: 'fabric', label: 'Fabric'},
                                    {value: 'quilt', label: 'Quilt'},
                                    {value: 'forge', label: 'Forge'},
                                    {value: 'neoforge', label: 'NeoForge'},
                                    {value: 'vanilla', label: 'Vanilla'},
                                ]}
                                onChange={(loaderType) => setCreateForm({
                                    ...createForm,
                                    loaderType,
                                    loaderVersion: pickCurrentValue(
                                        createForm.loaderVersion || 'latest',
                                        loaderVersionOptions(loaderType, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, createForm.minecraftVersion)
                                    ),
                                })}
                            />
                        </label>
                        {createForm.loaderType !== 'vanilla' && (
                            <label>
                                Loader version
                                <CommandSelect
                                    value={createForm.loaderVersion}
                                    ariaLabel="Loader version"
                                    options={loaderVersionOptions(createForm.loaderType, fabricLoaderVersions, quiltLoaderVersions, forgeLoaderVersions, neoForgeLoaderVersions, createForm.minecraftVersion).map((version) => ({
                                        value: version.id,
                                        label: version.label,
                                    }))}
                                    onChange={(loaderVersion) => setCreateForm({...createForm, loaderVersion})}
                                />
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
                            <CommandSelect
                                value={account.mode}
                                ariaLabel="Account mode"
                                options={[{value: 'offline', label: 'Offline profile'}]}
                                onChange={(mode) => setAccount({...account, mode})}
                            />
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
                        gameLogLists={profileGameLogLists}
                        gameLogContents={profileGameLogContents}
                        gameLogActionKey={gameLogActionKey}
                        onClearLogs={clearLauncherLogs}
                        onRefreshGameLogs={refreshProfileGameLogs}
                        onReadGameLog={readProfileGameLog}
                        onOpenLogsFolder={openProfileLogsFolder}
                        onOpenLogsWindow={openLauncherLogsWindow}
                        onFocusModeChange={setLogsFocusMode}
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
            </main>
        </div>
    );
}

function LauncherLogsWindow({
    logs,
    profiles,
    error,
    gameLogLists,
    gameLogContents,
    gameLogActionKey,
    onClearLogs,
    onRefreshGameLogs,
    onReadGameLog,
    onOpenLogsFolder
}: {
    logs: LauncherLog[];
    profiles: domain.Profile[];
    error: string;
    gameLogLists: Record<string, domain.GameLogList>;
    gameLogContents: Record<string, domain.GameLogContent>;
    gameLogActionKey: string;
    onClearLogs: () => void;
    onRefreshGameLogs: (profileId: string) => void;
    onReadGameLog: (profileId: string, fileName: string) => void;
    onOpenLogsFolder: (profileId: string) => void;
}) {
    const [focusMode, setFocusMode] = useState(false);

    return (
        <div className={focusMode ? 'logs-window-shell logs-window-focus' : 'logs-window-shell'}>
            {!focusMode && (
                <header className="logs-window-header">
                    <div>
                        <p className="eyebrow">Power Mine Logs</p>
                        <h1>Live log console</h1>
                        <p>{logs.length} launcher events / {profiles.length} installations</p>
                    </div>
                </header>
            )}
            {error && !focusMode && <div className="banner error">{error}</div>}
            <ClassicLogsPanel
                logs={logs}
                profiles={profiles}
                gameLogLists={gameLogLists}
                gameLogContents={gameLogContents}
                gameLogActionKey={gameLogActionKey}
                onClearLogs={onClearLogs}
                onRefreshGameLogs={onRefreshGameLogs}
                onReadGameLog={onReadGameLog}
                onOpenLogsFolder={onOpenLogsFolder}
                onFocusModeChange={setFocusMode}
            />
        </div>
    );
}

function HomePanel({
    profiles,
    selectedProfile,
    selectedProfileId,
    account,
    javaStatus,
    javaPath,
    installProgress,
    javaInstallProgress,
    profileJavaRuntimes,
    modList,
    modrinthUpdatePlans,
    launchStates,
    selectedLogs,
    onSelectProfile,
    onInstallProfile,
    onRepairProfile,
    onInstallJava,
    onLaunchProfile,
    onStopProfile,
    onOpenBrowse,
    onOpenCreate,
    onOpenLibrary,
    onOpenLogs,
    onOpenLogsWindow
}: {
    profiles: domain.Profile[];
    selectedProfile?: domain.Profile;
    selectedProfileId: string;
    account: AccountDraft;
    javaStatus: domain.JavaStatus | null;
    javaPath: string;
    installProgress: Record<string, InstallProgress>;
    javaInstallProgress: JavaInstallProgress | null;
    profileJavaRuntimes: Record<string, domain.ProfileJavaRuntime>;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    launchStates: Record<string, LaunchState>;
    selectedLogs: LauncherLog[];
    onSelectProfile: (id: string) => void;
    onInstallProfile: (id: string) => void;
    onRepairProfile: (id: string) => void;
    onInstallJava: (version: number) => void;
    onLaunchProfile: (id: string) => void;
    onStopProfile: (id: string) => void;
    onOpenBrowse: (profileId: string) => void;
    onOpenCreate: () => void;
    onOpenLibrary: (profile?: domain.Profile) => void;
    onOpenLogs: () => void;
    onOpenLogsWindow: () => void;
}) {
    const selectedProgress = selectedProfile ? installProgress[selectedProfile.id] : undefined;
    const selectedLaunch = selectedProfile ? launchStates[selectedProfile.id] : undefined;
    const selectedJavaRuntime = selectedProfile ? profileJavaRuntimes[selectedProfile.id] : undefined;
    const selectedPlayReason = playDisabledReason(selectedProfile, selectedLaunch, selectedJavaRuntime);
    const selectedInstalling = selectedProfile ? isInstalling(selectedProfile, installProgress) : false;
    const selectedInstallVisible = selectedProfile ? shouldShowInstallButton(selectedProfile, installProgress[selectedProfile.id]) : false;
    const selectedRepairVisible = selectedProfile ? shouldShowRepairButton(selectedProfile, installProgress[selectedProfile.id]) : false;
    const selectedStopVisible = shouldShowStopButton(selectedLaunch);
    const javaRequired = selectedJavaRuntime && !selectedJavaRuntime.installed;
    const javaBusy = isJavaInstalling(javaInstallProgress);
    const installedCount = profiles.filter((profile) => profile.install?.status === 'installed').length;
    const runningCount = Object.values(launchStates).filter((launch) => isLaunchActive(launch)).length;
    const readyCount = profiles.filter((profile) => !playDisabledReason(
        profile,
        launchStates[profile.id],
        profileJavaRuntimes[profile.id]
    )).length;
    const [healthReviewOpen, setHealthReviewOpen] = useState(false);
    const [logReviewOpen, setLogReviewOpen] = useState(false);
    const healthProps: ProfileHealthProps | null = selectedProfile ? {
        profile: selectedProfile,
        progress: selectedProgress,
        launch: selectedLaunch,
        javaRuntime: selectedJavaRuntime,
        javaInstallProgress,
        modList,
        modrinthUpdatePlans,
        onInstall: onInstallProfile,
        onRepair: onRepairProfile,
        onInstallJava,
        onStop: onStopProfile,
        onBrowseMods: onOpenBrowse,
        onOpenLogs,
    } : null;

    useEffect(() => {
        setHealthReviewOpen(false);
        setLogReviewOpen(false);
    }, [selectedProfile?.id]);

    return (
        <section className="home-grid">
            <div className="launch-panel">
                <div className="launch-main">
                    <p className="eyebrow">Home</p>
                    <h2>{selectedProfile?.name ?? 'Create your first installation'}</h2>
                    <p>{selectedProfile ? profileSubtitle(selectedProfile) : 'No launch target yet.'}</p>
                    {profiles.length > 0 && (
                        <label className="launch-profile-select">
                            Installation
                            <CommandSelect
                                value={selectedProfile?.id ?? selectedProfileId}
                                ariaLabel="Installation"
                                options={profiles.map((profile) => ({
                                    value: profile.id,
                                    label: `${profile.name} - ${profileSubtitle(profile)}`,
                                }))}
                                onChange={onSelectProfile}
                            />
                        </label>
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
                                {launchActionLabel(selectedLaunch)}
                            </button>
                            {selectedStopVisible && (
                                <button
                                    className="danger"
                                    type="button"
                                    disabled={selectedLaunch?.status === 'stopping'}
                                    onClick={() => onStopProfile(selectedProfile.id)}
                                >
                                    {selectedLaunch?.status === 'stopping' ? 'Stopping' : 'Stop'}
                                </button>
                            )}
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
                            <button type="button" onClick={() => onOpenLibrary(selectedProfile)}>Settings</button>
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
                <StatusTile label="Running" value={runningCount.toString()}/>
                <StatusTile label="Account" value={accountLabel(account)}/>
                <StatusTile label="Java" value={javaStatusText(javaStatus, javaPath)}/>
            </div>

            {selectedProfile ? (
                <div className="home-secondary">
                    {healthProps && (
                        <ProfileHealthReviewCard
                            {...healthProps}
                            onReview={() => setHealthReviewOpen(true)}
                        />
                    )}
                    <LauncherLogReviewCard
                        logs={selectedLogs}
                        onReview={() => setLogReviewOpen(true)}
                    />
                    {healthReviewOpen && healthProps && (
                        <ProfileHealthReviewModal
                            {...healthProps}
                            onClose={() => setHealthReviewOpen(false)}
                        />
                    )}
                    {logReviewOpen && (
                        <LauncherLogReviewModal
                            logs={selectedLogs}
                            onOpenLogs={onOpenLogsWindow}
                            onClose={() => setLogReviewOpen(false)}
                        />
                    )}
                </div>
            ) : (
                <EmptyState title="No profiles" action="Create an installation to start playing."/>
            )}

            {profiles.length > 0 && (
                <HomeInstallationsPanel
                    profiles={profiles}
                    selectedProfileId={selectedProfile?.id ?? selectedProfileId}
                    installProgress={installProgress}
                    profileJavaRuntimes={profileJavaRuntimes}
                    launchStates={launchStates}
                    onSelectProfile={onSelectProfile}
                    onInstallProfile={onInstallProfile}
                    onRepairProfile={onRepairProfile}
                    onLaunchProfile={onLaunchProfile}
                    onStopProfile={onStopProfile}
                    onOpenLibrary={onOpenLibrary}
                />
            )}
        </section>
    );
}

function HomeInstallationsPanel({
    profiles,
    selectedProfileId,
    installProgress,
    profileJavaRuntimes,
    launchStates,
    onSelectProfile,
    onInstallProfile,
    onRepairProfile,
    onLaunchProfile,
    onStopProfile,
    onOpenLibrary
}: {
    profiles: domain.Profile[];
    selectedProfileId: string;
    installProgress: Record<string, InstallProgress>;
    profileJavaRuntimes: Record<string, domain.ProfileJavaRuntime>;
    launchStates: Record<string, LaunchState>;
    onSelectProfile: (id: string) => void;
    onInstallProfile: (id: string) => void;
    onRepairProfile: (id: string) => void;
    onLaunchProfile: (id: string) => void;
    onStopProfile: (id: string) => void;
    onOpenLibrary: (profile?: domain.Profile) => void;
}) {
    return (
        <section className="dashboard-panel installation-overview">
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Installations</p>
                    <h2>Quick access</h2>
                </div>
            </div>
            <div className="home-profile-list">
                {profiles.map((profile) => {
                    const progress = installProgress[profile.id];
                    const launch = launchStates[profile.id];
                    const javaRuntime = profileJavaRuntimes[profile.id];
                    const playReason = playDisabledReason(profile, launch, javaRuntime);
                    const installing = isInstalling(profile, installProgress);
                    const installVisible = shouldShowInstallButton(profile, progress);
                    const repairVisible = shouldShowRepairButton(profile, progress);
                    const stopVisible = shouldShowStopButton(launch);
                    const active = profile.id === selectedProfileId;

                    return (
                        <article key={profile.id} className={active ? 'home-profile-row active' : 'home-profile-row'}>
                            <div className="home-profile-info">
                                <strong>{profile.name}</strong>
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
                                    {launchActionLabel(launch)}
                                </button>
                                {stopVisible && (
                                    <button className="small danger" type="button" disabled={launch?.status === 'stopping'} onClick={() => onStopProfile(profile.id)}>
                                        {launch?.status === 'stopping' ? 'Stopping' : 'Stop'}
                                    </button>
                                )}
                                <button className="small" type="button" onClick={() => onOpenLibrary(profile)}>Settings</button>
                            </div>
                        </article>
                    );
                })}
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
    onStop,
    onOpenLogs,
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
    onStop: (id: string) => void;
    onOpenLogs: () => void;
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
    const stopVisible = shouldShowStopButton(launch);

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
            <ProfileHealthPanel
                profile={profile}
                progress={progress}
                launch={launch}
                javaRuntime={javaRuntime}
                javaInstallProgress={javaInstallProgress}
                modList={modList}
                modrinthUpdatePlans={modrinthUpdatePlans}
                onInstall={onInstall}
                onRepair={onRepair}
                onInstallJava={onInstallJava}
                onStop={onStop}
                onBrowseMods={(profileId) => onBrowseMod(profileId, '', '')}
                onOpenLogs={onOpenLogs}
            />
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
                    {launchActionLabel(launch)}
                </button>
                {stopVisible && (
                    <button
                        className="danger"
                        type="button"
                        disabled={launch?.status === 'stopping'}
                        onClick={() => onStop(profile.id)}
                    >
                        {launch?.status === 'stopping' ? 'Stopping' : 'Stop'}
                    </button>
                )}
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
            {settingsOpen && settingsDraft && (
                <ProfileSettingsReviewModal onClose={onCloseSettings}>
                    <ProfileSettingsPanel
                        profile={profile}
                        draft={settingsDraft}
                        javaRuntime={javaRuntime}
                        javaInstallProgress={javaInstallProgress}
                        modList={modList}
                        modrinthUpdatePlans={modrinthUpdatePlans}
                        modActionKey={modActionKey}
                        onDraftChange={onSettingsDraftChange}
                        onInstallJava={onInstallJava}
                        onSave={onSaveSettings}
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
                </ProfileSettingsReviewModal>
            )}
        </section>
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
                    <CommandSelect
                        value={profile?.id ?? ''}
                        disabled={loading || !!installingProjectId || !!deletingProjectId}
                        ariaLabel="Browse installation"
                        options={profiles.length === 0
                            ? [{value: '', label: 'No profiles'}]
                            : profiles.map((option) => ({
                                value: option.id,
                                label: `${option.name} - ${profileSubtitle(option)}`,
                            }))}
                        onChange={onProfileChange}
                    />
                </label>
                <div className="browse-controls">
                    <input
                        value={query}
                        placeholder="Search mods"
                        disabled={!canBrowse || loading}
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
                    <MarkdownContent>{project.body}</MarkdownContent>
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

function MarkdownContent({children}: {children: string}) {
    return (
        <ReactMarkdown
            allowedElements={markdownAllowedElements}
            rehypePlugins={[rehypeRaw]}
            remarkPlugins={[remarkGfm]}
            components={{
                a({href, children}) {
                    const safeHref = safeMarkdownLink(href);
                    if (!safeHref) {
                        return <>{children}</>;
                    }
                    return <a href={safeHref} target="_blank" rel="noreferrer">{children}</a>;
                },
                img({src, alt}) {
                    const safeSrc = safeMarkdownImage(src);
                    if (!safeSrc) {
                        return null;
                    }
                    return <img src={safeSrc} alt={alt ?? ''} loading="lazy"/>;
                },
                details({children}) {
                    return <details className="markdown-details">{children}</details>;
                },
                summary({children}) {
                    return <summary>{children}</summary>;
                }
            }}
        >
            {children}
        </ReactMarkdown>
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

function profileHealthView({
    profile,
    progress,
    launch,
    javaRuntime,
    javaInstallProgress,
    modList,
    modrinthUpdatePlans,
    onInstall,
    onRepair,
    onInstallJava,
    onStop,
    onBrowseMods,
    onOpenLogs
}: ProfileHealthProps) {
    const items = profileHealthItems({
        profile,
        progress,
        launch,
        javaRuntime,
        javaInstallProgress,
        modList,
        modrinthUpdatePlans,
        onInstall,
        onRepair,
        onInstallJava,
        onStop,
        onBrowseMods,
        onOpenLogs,
    });
    const summary = profileHealthSummary(items, playDisabledReason(profile, launch, javaRuntime), launch);
    return {items, summary};
}

function ProfileHealthReviewCard(props: ProfileHealthProps & { onReview: () => void }) {
    const {summary, items} = profileHealthView(props);
    const actionCount = items.filter((item) => item.actionLabel).length;

    return (
        <section className="profile-health-review dashboard-panel">
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Profile health</p>
                    <h2>{summary.title}</h2>
                    <p className="muted">{summary.detail}</p>
                </div>
                <span className={`health-badge ${summary.tone}`}>{summary.label}</span>
            </div>
            <div className="health-review-footer">
                <span>{items.length} checks{actionCount > 0 ? ` / ${actionCount} actions` : ''}</span>
                <button className="primary" type="button" onClick={props.onReview}>Review</button>
            </div>
        </section>
    );
}

function ProfileHealthReviewModal(props: ProfileHealthProps & { onClose: () => void }) {
    useEffect(() => {
        function handleKeyDown(event: Event) {
            if ((event as globalThis.KeyboardEvent).key === 'Escape') {
                props.onClose();
            }
        }
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [props.onClose]);

    return (
        <div className="modal-backdrop profile-health-backdrop" role="presentation" onMouseDown={props.onClose}>
            <div role="presentation" onMouseDown={(event) => event.stopPropagation()}>
                <ProfileHealthPanel {...props} modal onClose={props.onClose}/>
            </div>
        </div>
    );
}

function ProfileHealthPanel(props: ProfileHealthProps & { modal?: boolean; onClose?: () => void }) {
    const {progress, modal = false, onClose} = props;
    const {items, summary} = profileHealthView(props);

    return (
        <section
            className={modal ? 'profile-health dashboard-panel profile-health-window' : 'profile-health dashboard-panel'}
            role={modal ? 'dialog' : undefined}
            aria-modal={modal ? true : undefined}
            aria-labelledby={modal ? 'profile-health-review-title' : undefined}
        >
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Profile health</p>
                    <h2 id={modal ? 'profile-health-review-title' : undefined}>{summary.title}</h2>
                    <p className="muted">{summary.detail}</p>
                </div>
                <div className="health-heading-actions">
                    <span className={`health-badge ${summary.tone}`}>{summary.label}</span>
                    {onClose && <button className="small" type="button" onClick={onClose}>Close</button>}
                </div>
            </div>
            {progress && <ProgressBar progress={progress}/>}
            <div className="health-list">
                {items.map((item) => (
                    <article key={item.key} className={`health-row ${item.tone}`}>
                        <span className="health-dot" aria-hidden="true"/>
                        <div className="health-main">
                            <strong>{item.label}</strong>
                            <span>{item.status}</span>
                            <p>{item.detail}</p>
                        </div>
                        {item.actionLabel && item.onAction && (
                            <button
                                className={item.actionClass ? `small ${item.actionClass}` : 'small'}
                                type="button"
                                disabled={item.actionDisabled}
                                onClick={item.onAction}
                            >
                                {item.actionLabel}
                            </button>
                        )}
                    </article>
                ))}
            </div>
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
    onOpenLogs,
    openLabel = 'Open logs'
}: {
    logs: LauncherLog[];
    compact?: boolean;
    detailed?: boolean;
    onOpenLogs?: () => void;
    openLabel?: string;
}) {
    return (
        <section className={compact ? 'log-panel compact' : 'log-panel'}>
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Launcher logs</p>
                    <h2>{detailed ? 'Event stream' : 'Recent events'}</h2>
                </div>
                {onOpenLogs && (
                    <button className="small" type="button" onClick={onOpenLogs}>{openLabel}</button>
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

function LauncherLogReviewCard({
    logs,
    onReview
}: {
    logs: LauncherLog[];
    onReview: () => void;
}) {
    const latest = logs[0];
    const errorCount = logs.filter((log) => log.level === 'error').length;

    return (
        <section className="log-review dashboard-panel">
            <div className="panel-heading">
                <div>
                    <p className="eyebrow">Launcher logs</p>
                    <h2>Recent events</h2>
                    <p className="muted">{latest ? latest.message : 'No launcher events yet.'}</p>
                </div>
                <span className={errorCount > 0 ? 'health-badge error' : 'health-badge idle'}>
                    {logs.length} events
                </span>
            </div>
            <div className="health-review-footer">
                <span>{errorCount > 0 ? `${errorCount} errors` : 'stream idle'}</span>
                <button className="primary" type="button" onClick={onReview}>Review</button>
            </div>
        </section>
    );
}

function LauncherLogReviewModal({
    logs,
    onOpenLogs,
    onClose
}: {
    logs: LauncherLog[];
    onOpenLogs: () => void;
    onClose: () => void;
}) {
    useEffect(() => {
        function handleKeyDown(event: Event) {
            if ((event as globalThis.KeyboardEvent).key === 'Escape') {
                onClose();
            }
        }
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [onClose]);

    return (
        <div className="modal-backdrop profile-health-backdrop" role="presentation" onMouseDown={onClose}>
            <div className="launcher-log-window" role="presentation" onMouseDown={(event) => event.stopPropagation()}>
                <LauncherLogPanel logs={logs} detailed onOpenLogs={onOpenLogs} openLabel="Open window"/>
                <div className="modal-window-actions">
                    <button className="small" type="button" onClick={onClose}>Close</button>
                </div>
            </div>
        </div>
    );
}

function ProfileSettingsReviewModal({
    children,
    onClose
}: {
    children: ReactNode;
    onClose: () => void;
}) {
    useEffect(() => {
        function handleKeyDown(event: Event) {
            if ((event as globalThis.KeyboardEvent).key === 'Escape') {
                onClose();
            }
        }
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [onClose]);

    return (
        <div className="modal-backdrop profile-health-backdrop" role="presentation" onMouseDown={onClose}>
            <section
                className="profile-settings-window"
                role="dialog"
                aria-modal="true"
                aria-labelledby="profile-settings-review-title"
                onMouseDown={(event) => event.stopPropagation()}
            >
                <div className="settings-window-header">
                    <div>
                        <p className="eyebrow">Profile settings</p>
                        <h2 id="profile-settings-review-title">Settings review</h2>
                    </div>
                    <button className="small" type="button" onClick={onClose}>Close</button>
                </div>
                {children}
            </section>
        </div>
    );
}

function ClassicLogsPanel({
    logs,
    profiles,
    gameLogLists,
    gameLogContents,
    gameLogActionKey,
    onClearLogs,
    onRefreshGameLogs,
    onReadGameLog,
    onOpenLogsFolder,
    onOpenLogsWindow,
    onFocusModeChange
}: {
    logs: LauncherLog[];
    profiles: domain.Profile[];
    gameLogLists: Record<string, domain.GameLogList>;
    gameLogContents: Record<string, domain.GameLogContent>;
    gameLogActionKey: string;
    onClearLogs: () => void;
    onRefreshGameLogs: (profileId: string) => void;
    onReadGameLog: (profileId: string, fileName: string) => void;
    onOpenLogsFolder: (profileId: string) => void;
    onOpenLogsWindow?: () => void;
    onFocusModeChange?: (focused: boolean) => void;
}) {
    const [mode, setMode] = useState<LogsMode>('launcher');
    const [focusMode, setFocusMode] = useState(false);
    const [appearanceOpen, setAppearanceOpen] = useState(false);
    const [appearance, setAppearance] = useState<LogAppearance>(() => loadLogAppearance());
    const [profileFilter, setProfileFilter] = useState('all');
    const [levelFilter, setLevelFilter] = useState<LogLevelFilter>('all');
    const [sourceFilter, setSourceFilter] = useState('all');
    const [query, setQuery] = useState('');
    const [copyStatus, setCopyStatus] = useState('');
    const [gameProfileId, setGameProfileId] = useState('');
    const [gameFileName, setGameFileName] = useState('live');

    const profilesById = useMemo(() => new Map(profiles.map((profile) => [profile.id, profile])), [profiles]);
    const gameProfile = useMemo(
        () => profiles.find((profile) => profile.id === gameProfileId) ?? profiles[0],
        [profiles, gameProfileId]
    );
    const gameLogList = gameProfile ? gameLogLists[gameProfile.id] : undefined;
    const gameFiles = gameLogList?.files ?? [];
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
        if (profileFilter !== 'all' && profileFilter !== 'global' && !profilesById.has(profileFilter)) {
            setProfileFilter('all');
        }
    }, [profileFilter, profilesById]);

    const baseLogs = useMemo(() => logs.filter((log) => {
        if (profileFilter === 'global' && log.profileId) {
            return false;
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
        const profileName = log.profileId ? profilesById.get(log.profileId)?.name ?? log.profileId : 'launcher';
        return [
            log.time,
            log.level,
            log.source,
            log.message,
            profileName,
        ].some((part) => part.toLowerCase().includes(text));
    }), [logs, profileFilter, levelFilter, query, profilesById]);

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
    const appearanceStyle = useMemo(() => logAppearanceStyle(appearance), [appearance]);

    useEffect(() => {
        saveLogAppearance(appearance);
    }, [appearance]);

    useEffect(() => {
        onFocusModeChange?.(focusMode);
        return () => onFocusModeChange?.(false);
    }, [focusMode, onFocusModeChange]);

    useEffect(() => {
        if (!focusMode) {
            return;
        }
        function handleKeyDown(event: Event) {
            if ((event as globalThis.KeyboardEvent).key === 'Escape') {
                setFocusMode(false);
            }
        }
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [focusMode]);

    async function copyVisibleLogs() {
        if (consoleLogs.length === 0) {
            setCopyStatus('Nothing to copy');
            return;
        }
        const text = consoleLogs.map((log) => formatLauncherLogLine(log, profilesById)).join('\n');
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
            ? liveGameLogs.map((log) => formatLauncherLogLine(log, profilesById)).join('\n')
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

    function updateAppearance(next: Partial<LogAppearance>) {
        setAppearance((current) => sanitizeLogAppearance({...current, ...next}));
    }

    function resetAppearance() {
        setAppearance(defaultLogAppearance);
    }

    return (
        <section className={focusMode ? 'classic-logs logs-focus-mode' : 'classic-logs'}>
            {focusMode && (
                <div className="logs-focus-bar">
                    <button type="button" onClick={() => setFocusMode(false)}>Controls</button>
                    <button type="button" onClick={() => setAppearanceOpen((current) => !current)}>Appearance</button>
                </div>
            )}
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
                        <button type="button" onClick={() => setFocusMode(true)}>Focus</button>
                        <button type="button" onClick={() => setAppearanceOpen((current) => !current)}>
                            {appearanceOpen ? 'Hide appearance' : 'Appearance'}
                        </button>
                        {onOpenLogsWindow && (
                            <button type="button" onClick={onOpenLogsWindow}>Open window</button>
                        )}
                        <button type="button" onClick={copyVisibleLogs} disabled={consoleLogs.length === 0}>Copy visible</button>
                        <button className="danger" type="button" onClick={clearLogs} disabled={logs.length === 0}>Clear</button>
                    </div>
                </div>

                {appearanceOpen && (
                    <LogAppearancePanel
                        appearance={appearance}
                        onChange={updateAppearance}
                        onReset={resetAppearance}
                    />
                )}

                <div className="logs-toolbar">
                    <label>
                        Installation
                        <CommandSelect
                            value={profileFilter}
                            ariaLabel="Log installation filter"
                            options={[
                                {value: 'all', label: 'All installations'},
                                {value: 'global', label: 'Launcher only'},
                                ...profiles.map((profile) => ({
                                    value: profile.id,
                                    label: profile.name,
                                })),
                            ]}
                            onChange={setProfileFilter}
                        />
                    </label>
                    <label>
                        Level
                        <CommandSelect
                            value={levelFilter}
                            ariaLabel="Log level filter"
                            options={[
                                {value: 'all', label: 'All levels'},
                                {value: 'info', label: 'Info'},
                                {value: 'success', label: 'Success'},
                                {value: 'error', label: 'Error'},
                            ]}
                            onChange={(nextLevel) => setLevelFilter(nextLevel as LogLevelFilter)}
                        />
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

                <div className="logs-console" style={appearanceStyle} role="log" aria-live="polite">
                    {consoleLogs.length === 0 ? (
                        <p className="logs-empty">No logs match the current filters.</p>
                    ) : (
                        consoleLogs.map((log) => (
                            <div key={log.id} className={`classic-log-line ${log.level}`}>
                                <span className="log-line-time">{log.time}</span>
                                <span className="log-line-level">{log.level}</span>
                                <span className="log-line-source">{log.source}</span>
                                <span className="log-line-profile">{logProfileLabel(log, profilesById)}</span>
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
                                <button type="button" onClick={() => setFocusMode(true)}>Focus</button>
                                <button type="button" onClick={() => setAppearanceOpen((current) => !current)}>
                                    {appearanceOpen ? 'Hide appearance' : 'Appearance'}
                                </button>
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
                                    disabled={!gameProfile}
                                    onClick={() => gameProfile && onOpenLogsFolder(gameProfile.id)}
                                >
                                    Open folder
                                </button>
                            </div>
                        </div>

                        {appearanceOpen && (
                            <LogAppearancePanel
                                appearance={appearance}
                                onChange={updateAppearance}
                                onReset={resetAppearance}
                            />
                        )}

                        <div className="logs-toolbar game-logs-toolbar">
                            <label>
                                Installation
                                <CommandSelect
                                    value={gameProfile?.id ?? ''}
                                    disabled={profiles.length === 0}
                                    ariaLabel="Game log installation"
                                    options={profiles.length === 0
                                        ? [{value: '', label: 'No installations'}]
                                        : profiles.map((profile) => ({
                                            value: profile.id,
                                            label: profile.name,
                                        }))}
                                    onChange={(profileId) => {
                                        setGameProfileId(profileId);
                                        setGameFileName('live');
                                        setCopyStatus('');
                                    }}
                                />
                            </label>
                            <div className="game-log-meta">
                                <span>{gameLogList?.logsDir ?? 'No logs folder selected'}</span>
                            </div>
                        </div>

                        {copyStatus && <p className="logs-status">{copyStatus}</p>}

                        <div className="logs-console game-log-console" style={appearanceStyle} role="log" aria-live="polite">
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

function LogAppearancePanel({
    appearance,
    onChange,
    onReset
}: {
    appearance: LogAppearance;
    onChange: (next: Partial<LogAppearance>) => void;
    onReset: () => void;
}) {
    return (
        <section className="logs-appearance-panel">
            <div className="logs-appearance-heading">
                <div>
                    <p className="eyebrow">Log appearance</p>
                    <strong>Columns and colors</strong>
                </div>
                <button className="small" type="button" onClick={onReset}>Reset</button>
            </div>
            <div className="logs-appearance-grid">
                <label>
                    Font size
                    <input
                        type="range"
                        min="10"
                        max="22"
                        value={appearance.fontSize}
                        onChange={(event) => onChange({fontSize: Number(event.target.value)})}
                    />
                    <span>{appearance.fontSize}px</span>
                </label>
                <label>
                    Time width
                    <input
                        type="range"
                        min="46"
                        max="180"
                        value={appearance.timeWidth}
                        onChange={(event) => onChange({timeWidth: Number(event.target.value)})}
                    />
                    <span>{appearance.timeWidth}px</span>
                </label>
                <label>
                    Level width
                    <input
                        type="range"
                        min="46"
                        max="160"
                        value={appearance.levelWidth}
                        onChange={(event) => onChange({levelWidth: Number(event.target.value)})}
                    />
                    <span>{appearance.levelWidth}px</span>
                </label>
                <label>
                    Source width
                    <input
                        type="range"
                        min="60"
                        max="260"
                        value={appearance.sourceWidth}
                        onChange={(event) => onChange({sourceWidth: Number(event.target.value)})}
                    />
                    <span>{appearance.sourceWidth}px</span>
                </label>
                <label>
                    Profile width
                    <input
                        type="range"
                        min="60"
                        max="280"
                        value={appearance.profileWidth}
                        onChange={(event) => onChange({profileWidth: Number(event.target.value)})}
                    />
                    <span>{appearance.profileWidth}px</span>
                </label>
                <label>
                    Background
                    <input
                        type="color"
                        value={appearance.consoleBg}
                        onChange={(event) => onChange({consoleBg: event.target.value})}
                    />
                </label>
                <label>
                    Time color
                    <input
                        type="color"
                        value={appearance.timeColor}
                        onChange={(event) => onChange({timeColor: event.target.value})}
                    />
                </label>
                <label>
                    Level color
                    <input
                        type="color"
                        value={appearance.levelColor}
                        disabled={appearance.severityColors}
                        onChange={(event) => onChange({levelColor: event.target.value})}
                    />
                </label>
                <label>
                    Source color
                    <input
                        type="color"
                        value={appearance.sourceColor}
                        onChange={(event) => onChange({sourceColor: event.target.value})}
                    />
                </label>
                <label>
                    Profile color
                    <input
                        type="color"
                        value={appearance.profileColor}
                        onChange={(event) => onChange({profileColor: event.target.value})}
                    />
                </label>
                <label>
                    Message color
                    <input
                        type="color"
                        value={appearance.messageColor}
                        onChange={(event) => onChange({messageColor: event.target.value})}
                    />
                </label>
                <label className="logs-appearance-toggle">
                    <input
                        type="checkbox"
                        checked={appearance.severityColors}
                        onChange={(event) => onChange({severityColors: event.target.checked})}
                    />
                    Severity colors
                </label>
            </div>
        </section>
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

function logProfileLabel(log: LauncherLog, profilesById: Map<string, domain.Profile>) {
    if (!log.profileId) {
        return 'Launcher';
    }
    return profilesById.get(log.profileId)?.name ?? log.profileId;
}

function formatLauncherLogLine(log: LauncherLog, profilesById: Map<string, domain.Profile>) {
    return `[${log.time}] [${log.level.toUpperCase()}] [${log.source}] [${logProfileLabel(log, profilesById)}] ${log.message}`;
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
    if (launch?.status === 'stopping') {
        return 'Stop in progress';
    }
    if (isLaunchActive(launch)) {
        return 'Already running';
    }
    if (!isLaunchableLoader(profile.loader.type)) {
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
        case 'stopping':
            return 'Stopping: ' + launch.message;
        case 'stopped':
            return `Stopped${launch.exitCode !== undefined ? ` with exit ${launch.exitCode}` : ''}.`;
        case 'failed':
            return 'Failed: ' + launch.message;
    }
}

function isLaunchActive(launch?: LaunchState) {
    return launch?.status === 'starting' || launch?.status === 'running' || launch?.status === 'stopping';
}

function shouldShowStopButton(launch?: LaunchState) {
    return launch?.status === 'running' || launch?.status === 'stopping';
}

function launchActionLabel(launch?: LaunchState) {
    switch (launch?.status) {
        case 'starting':
            return 'Starting';
        case 'running':
            return 'Running';
        case 'stopping':
            return 'Stopping';
        default:
            return 'Play';
    }
}

function profileHealthItems({
    profile,
    progress,
    launch,
    javaRuntime,
    javaInstallProgress,
    modList,
    modrinthUpdatePlans,
    onInstall,
    onRepair,
    onInstallJava,
    onStop,
    onBrowseMods,
    onOpenLogs,
}: {
    profile: domain.Profile;
    progress?: InstallProgress;
    launch?: LaunchState;
    javaRuntime?: domain.ProfileJavaRuntime;
    javaInstallProgress: JavaInstallProgress | null;
    modList?: domain.ModList;
    modrinthUpdatePlans: domain.ModrinthUpdatePlan[];
    onInstall: (id: string) => void;
    onRepair: (id: string) => void;
    onInstallJava: (version: number) => void;
    onStop: (id: string) => void;
    onBrowseMods: (profileId: string) => void;
    onOpenLogs: () => void;
}): ProfileHealthItem[] {
    const progressActive = !!progress && !progress.done && progress.stage !== 'failed';
    const installing = isInstalling(profile, {[profile.id]: progress});
    const installVisible = shouldShowInstallButton(profile, progress);
    const repairVisible = shouldShowRepairButton(profile, progress);
    const installFailed = profile.install?.status === 'failed' || !!profile.install?.lastError;
    const installAction = installFailed && repairVisible
        ? {label: 'Repair', handler: () => onRepair(profile.id)}
        : installVisible
            ? {label: installing ? 'Installing' : 'Install', handler: () => onInstall(profile.id)}
            : repairVisible
                ? {label: installing ? 'Repairing' : 'Repair', handler: () => onRepair(profile.id)}
                : undefined;

    const items: ProfileHealthItem[] = [{
        key: 'install',
        label: 'Minecraft files',
        status: progressActive ? progressMessage(progress) : installStatusText(profile),
        detail: profile.install?.lastError || profile.install?.message || 'Base game and loader files are tracked for this profile.',
        tone: progressActive ? 'busy' : installFailed ? 'error' : profile.install?.status === 'installed' ? 'ok' : 'warn',
        actionLabel: installAction?.label,
        actionClass: installFailed ? 'primary' : undefined,
        actionDisabled: installing,
        onAction: installAction?.handler,
    }];

    const javaProgressMatches = !!javaRuntime?.requiredMajor && javaInstallProgress?.version === javaRuntime.requiredMajor.toString();
    const javaBusy = isJavaInstalling(javaInstallProgress) && javaProgressMatches;
    items.push({
        key: 'java',
        label: 'Java runtime',
        status: javaBusy
            ? javaProgressMessage(javaInstallProgress)
            : javaRuntime
                ? javaRuntime.installed ? `Java ${javaRuntime.requiredMajor} ready` : `Java ${javaRuntime.requiredMajor} missing`
                : 'Checking runtime',
        detail: javaRuntimeText(javaRuntime),
        tone: javaBusy ? 'busy' : !javaRuntime ? 'warn' : javaRuntime.installed ? 'ok' : 'error',
        actionLabel: javaRuntime && !javaRuntime.installed ? `Install Java ${javaRuntime.requiredMajor}` : undefined,
        actionClass: 'primary',
        actionDisabled: javaBusy,
        onAction: javaRuntime && !javaRuntime.installed ? () => onInstallJava(javaRuntime.requiredMajor) : undefined,
    });

    const launchable = isLaunchableLoader(profile.loader.type);
    items.push({
        key: 'loader',
        label: 'Loader',
        status: launchable ? 'Supported' : 'Unsupported',
        detail: launchable
            ? `${profile.loader.type}${profile.loader.version ? ` ${profile.loader.version}` : ''} can be launched by Power Mine.`
            : `${profile.loader.type} profiles are not launchable yet.`,
        tone: launchable ? 'ok' : 'error',
    });

    const modLoader = isModCapableLoader(profile.loader.type);
    const mods = modList?.mods ?? [];
    const disabledCount = mods.filter((mod) => !mod.enabled).length;
    const updateCount = modrinthUpdatePlans.filter((plan) => plan.updateAvailable).length;
    const updateErrorCount = modrinthUpdatePlans.filter((plan) => !!plan.checkError).length;
    items.push({
        key: 'mods',
        label: 'Mods',
        status: modLoader
            ? modList ? `${mods.length} file${mods.length === 1 ? '' : 's'}` : 'Not loaded yet'
            : 'Not used',
        detail: modLoader
            ? modList
                ? modHealthDetail(mods.length, disabledCount, updateCount, updateErrorCount)
                : 'Open Library to load local mods and Modrinth update state.'
            : 'Vanilla profiles do not use a mods folder.',
        tone: !modLoader ? 'idle' : updateErrorCount > 0 ? 'error' : updateCount > 0 || disabledCount > 0 || !modList ? 'warn' : 'ok',
        actionLabel: modLoader ? 'Browse mods' : undefined,
        onAction: modLoader ? () => onBrowseMods(profile.id) : undefined,
    });

    items.push({
        key: 'launch',
        label: 'Launch',
        status: launch ? launchStatusText(launch) : 'Not running',
        detail: launch ? launch.message : 'No active Minecraft process for this profile.',
        tone: launch?.status === 'failed' ? 'error' : isLaunchActive(launch) ? 'busy' : 'idle',
        actionLabel: shouldShowStopButton(launch) ? (launch?.status === 'stopping' ? 'Stopping' : 'Stop') : launch?.status === 'failed' ? 'Open logs' : undefined,
        actionClass: shouldShowStopButton(launch) ? 'danger' : undefined,
        actionDisabled: launch?.status === 'stopping',
        onAction: shouldShowStopButton(launch) ? () => onStop(profile.id) : launch?.status === 'failed' ? onOpenLogs : undefined,
    });

    items.push({
        key: 'logs',
        label: 'Logs',
        status: profile.install?.lastError || launch?.status === 'failed' ? 'Needs review' : 'Available',
        detail: profile.install?.lastError || launch?.message || 'Open launcher and game logs when a profile fails to install or launch.',
        tone: profile.install?.lastError || launch?.status === 'failed' ? 'error' : 'ok',
        actionLabel: 'Open logs',
        onAction: onOpenLogs,
    });

    return items;
}

function profileHealthSummary(items: ProfileHealthItem[], playReason: string | null, launch?: LaunchState) {
    if (launch?.status === 'running') {
        return {title: 'Minecraft is running', detail: launch.message, label: 'Running', tone: 'busy' as HealthTone};
    }
    if (launch?.status === 'starting' || launch?.status === 'stopping') {
        return {title: 'Launch is changing state', detail: launchStatusText(launch), label: 'Busy', tone: 'busy' as HealthTone};
    }
    if (items.some((item) => item.tone === 'error')) {
        return {title: 'Action required', detail: firstHealthProblem(items), label: 'Fix', tone: 'error' as HealthTone};
    }
    if (playReason) {
        return {title: 'Not ready yet', detail: playReason, label: 'Check', tone: 'warn' as HealthTone};
    }
    if (items.some((item) => item.tone === 'warn')) {
        return {title: 'Ready with notes', detail: firstHealthProblem(items), label: 'Review', tone: 'warn' as HealthTone};
    }
    return {title: 'Ready to play', detail: 'Profile files, Java runtime, loader, and launch state look good.', label: 'Ready', tone: 'ok' as HealthTone};
}

function firstHealthProblem(items: ProfileHealthItem[]) {
    const item = items.find((candidate) => candidate.tone === 'error' || candidate.tone === 'warn');
    return item ? `${item.label}: ${item.status}` : 'No blockers detected.';
}

function modHealthDetail(modCount: number, disabledCount: number, updateCount: number, updateErrorCount: number) {
    const parts = [`${modCount} installed`];
    if (disabledCount > 0) {
        parts.push(`${disabledCount} disabled`);
    }
    if (updateCount > 0) {
        parts.push(`${updateCount} update${updateCount === 1 ? '' : 's'} available`);
    }
    if (updateErrorCount > 0) {
        parts.push(`${updateErrorCount} update check error${updateErrorCount === 1 ? '' : 's'}`);
    }
    return parts.join(', ') + '.';
}

function isLaunchableLoader(loader: string) {
    return loader === 'vanilla' || loader === 'fabric' || loader === 'quilt' || loader === 'forge' || loader === 'neoforge';
}

function isModCapableLoader(loader: string) {
    return loader === 'fabric' || loader === 'quilt' || loader === 'forge' || loader === 'neoforge';
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

function javaProgressMessage(progress: JavaInstallProgress) {
    const counter = progress.total > 0 ? ` (${formatBytes(progress.current)} / ${formatBytes(progress.total)})` : '';
    return `${progress.message}${counter}`;
}

function isInstalling(profile: domain.Profile, progressByProfile: Record<string, InstallProgress | undefined>) {
    const progress = progressByProfile[profile.id];
    return profile.install?.status === 'installing' || profile.install?.status === 'repairing' || (!!progress && !progress.done && progress.stage !== 'failed');
}

function shouldShowInstallButton(profile: domain.Profile, progress?: InstallProgress) {
    if (profile.install?.status === 'installed') {
        return false;
    }
    return profile.install?.status !== 'installed' || (!!progress && !progress.done && progress.stage !== 'failed');
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

function safeMarkdownLink(value?: string) {
    const url = safeMarkdownURL(value, ['http:', 'https:', 'mailto:']);
    if (url) {
        return url;
    }
    const trimmed = value?.trim() ?? '';
    return trimmed.startsWith('#') ? trimmed : '';
}

function safeMarkdownImage(value?: string) {
    return safeMarkdownURL(value, ['http:', 'https:']);
}

function safeMarkdownURL(value: string | undefined, allowedProtocols: string[]) {
    const trimmed = value?.trim() ?? '';
    if (!trimmed) {
        return '';
    }
    try {
        const url = new URL(trimmed);
        return allowedProtocols.includes(url.protocol) ? url.toString() : '';
    } catch {
        return '';
    }
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
