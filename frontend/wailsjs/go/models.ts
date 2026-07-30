export namespace domain {
	
	export class AccountConfig {
	    mode: string;
	    offlineName?: string;
	    offlineUuid?: string;
	
	    static createFrom(source: any = {}) {
	        return new AccountConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.offlineName = source["offlineName"];
	        this.offlineUuid = source["offlineUuid"];
	    }
	}
	export class AppInfo {
	    name: string;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.version = source["version"];
	    }
	}
	export class GameLogContent {
	    profileId: string;
	    fileName: string;
	    displayName: string;
	    kind: string;
	    content: string;
	    size: number;
	    truncated: boolean;
	    maxBytes: number;
	
	    static createFrom(source: any = {}) {
	        return new GameLogContent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.kind = source["kind"];
	        this.content = source["content"];
	        this.size = source["size"];
	        this.truncated = source["truncated"];
	        this.maxBytes = source["maxBytes"];
	    }
	}
	export class GameLogFile {
	    fileName: string;
	    displayName: string;
	    kind: string;
	    size: number;
	    updatedAt: string;
	    compressed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GameLogFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.updatedAt = source["updatedAt"];
	        this.compressed = source["compressed"];
	    }
	}
	export class GameLogList {
	    profileId: string;
	    logsDir: string;
	    files: GameLogFile[];
	
	    static createFrom(source: any = {}) {
	        return new GameLogList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.logsDir = source["logsDir"];
	        this.files = this.convertValues(source["files"], GameLogFile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InstallState {
	    status: string;
	    installed: boolean;
	    message?: string;
	    updatedAt?: string;
	    lastError?: string;
	    baseVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new InstallState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.installed = source["installed"];
	        this.message = source["message"];
	        this.updatedAt = source["updatedAt"];
	        this.lastError = source["lastError"];
	        this.baseVersion = source["baseVersion"];
	    }
	}
	export class JavaStatus {
	    path: string;
	    ok: boolean;
	    version?: string;
	    message: string;
	    checkedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new JavaStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.ok = source["ok"];
	        this.version = source["version"];
	        this.message = source["message"];
	        this.checkedAt = source["checkedAt"];
	    }
	}
	export class LaunchState {
	    profileId: string;
	    status: string;
	    message: string;
	    exitCode?: number;
	    startedAt?: string;
	    endedAt?: string;
	
	    static createFrom(source: any = {}) {
	        return new LaunchState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.status = source["status"];
	        this.message = source["message"];
	        this.exitCode = source["exitCode"];
	        this.startedAt = source["startedAt"];
	        this.endedAt = source["endedAt"];
	    }
	}
	export class LauncherLog {
	    id: string;
	    time: string;
	    level: string;
	    source: string;
	    message: string;
	    profileId?: string;

	    static createFrom(source: any = {}) {
	        return new LauncherLog(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = source["time"];
	        this.level = source["level"];
	        this.source = source["source"];
	        this.message = source["message"];
	        this.profileId = source["profileId"];
	    }
	}
	export class LoaderConfig {
	    type: string;
	    version?: string;
	
	    static createFrom(source: any = {}) {
	        return new LoaderConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.version = source["version"];
	    }
	}
	export class MemorySettings {
	    minMB: number;
	    maxMB: number;
	
	    static createFrom(source: any = {}) {
	        return new MemorySettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minMB = source["minMB"];
	        this.maxMB = source["maxMB"];
	    }
	}
	export class ModFile {
	    fileName: string;
	    displayName: string;
	    path: string;
	    enabled: boolean;
	    size: number;
	    updatedAt: string;
	    sha1?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.path = source["path"];
	        this.enabled = source["enabled"];
	        this.size = source["size"];
	        this.updatedAt = source["updatedAt"];
	        this.sha1 = source["sha1"];
	    }
	}
	export class ModList {
	    profileId: string;
	    modsDir: string;
	    mods: ModFile[];
	
	    static createFrom(source: any = {}) {
	        return new ModList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.modsDir = source["modsDir"];
	        this.mods = this.convertValues(source["mods"], ModFile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModpackExportResult {
	    profileId: string;
	    name: string;
	    versionId: string;
	    path: string;
	    filesExported: number;
	    overridesExported: number;
	
	    static createFrom(source: any = {}) {
	        return new ModpackExportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.name = source["name"];
	        this.versionId = source["versionId"];
	        this.path = source["path"];
	        this.filesExported = source["filesExported"];
	        this.overridesExported = source["overridesExported"];
	    }
	}
	export class Profile {
	    id: string;
	    name: string;
	    minecraftVersion: string;
	    loader: LoaderConfig;
	    account?: AccountConfig;
	    gameDir: string;
	    memory: MemorySettings;
	    install: InstallState;
	    createdAt: string;
	    updatedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new Profile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.minecraftVersion = source["minecraftVersion"];
	        this.loader = this.convertValues(source["loader"], LoaderConfig);
	        this.account = this.convertValues(source["account"], AccountConfig);
	        this.gameDir = source["gameDir"];
	        this.memory = this.convertValues(source["memory"], MemorySettings);
	        this.install = this.convertValues(source["install"], InstallState);
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModpackImportResult {
	    profile: Profile;
	    name: string;
	    versionId: string;
	    filesInstalled: number;
	    filesSkipped: number;
	    overridesInstalled: number;
	
	    static createFrom(source: any = {}) {
	        return new ModpackImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profile = this.convertValues(source["profile"], Profile);
	        this.name = source["name"];
	        this.versionId = source["versionId"];
	        this.filesInstalled = source["filesInstalled"];
	        this.filesSkipped = source["filesSkipped"];
	        this.overridesInstalled = source["overridesInstalled"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthDeleteFile {
	    projectId: string;
	    projectTitle: string;
	    versionId: string;
	    versionName: string;
	    versionNumber: string;
	    fileName: string;
	    displayName: string;
	    dependencyType?: string;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthDeleteFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.versionId = source["versionId"];
	        this.versionName = source["versionName"];
	        this.versionNumber = source["versionNumber"];
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.dependencyType = source["dependencyType"];
	        this.reason = source["reason"];
	    }
	}
	export class ModrinthDeletePlan {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    files?: ModrinthDeleteFile[];
	    skippedFiles?: ModrinthDeleteFile[];
	    tracked: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthDeletePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.files = this.convertValues(source["files"], ModrinthDeleteFile);
	        this.skippedFiles = this.convertValues(source["skippedFiles"], ModrinthDeleteFile);
	        this.tracked = source["tracked"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthDeleteResult {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    deletedFiles?: ModrinthDeleteFile[];
	    skippedFiles?: ModrinthDeleteFile[];
	    modList: ModList;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthDeleteResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.deletedFiles = this.convertValues(source["deletedFiles"], ModrinthDeleteFile);
	        this.skippedFiles = this.convertValues(source["skippedFiles"], ModrinthDeleteFile);
	        this.modList = this.convertValues(source["modList"], ModList);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthDependency {
	    versionId?: string;
	    projectId?: string;
	    fileName?: string;
	    dependencyType: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.versionId = source["versionId"];
	        this.projectId = source["projectId"];
	        this.fileName = source["fileName"];
	        this.dependencyType = source["dependencyType"];
	    }
	}
	export class ModrinthSkippedDependency {
	    dependency: ModrinthDependency;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthSkippedDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dependency = this.convertValues(source["dependency"], ModrinthDependency);
	        this.reason = source["reason"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthRequiredDependency {
	    projectId: string;
	    projectTitle: string;
	    versionId: string;
	    versionName: string;
	    versionNumber: string;
	    fileName: string;
	    displayName: string;
	    alreadyPresent?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthRequiredDependency(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.versionId = source["versionId"];
	        this.versionName = source["versionName"];
	        this.versionNumber = source["versionNumber"];
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.alreadyPresent = source["alreadyPresent"];
	    }
	}
	export class ModrinthInstallPlan {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    versionId: string;
	    versionName: string;
	    versionNumber: string;
	    fileName: string;
	    requiredDependencies?: ModrinthRequiredDependency[];
	    skippedDependencies?: ModrinthSkippedDependency[];
	
	    static createFrom(source: any = {}) {
	        return new ModrinthInstallPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.versionId = source["versionId"];
	        this.versionName = source["versionName"];
	        this.versionNumber = source["versionNumber"];
	        this.fileName = source["fileName"];
	        this.requiredDependencies = this.convertValues(source["requiredDependencies"], ModrinthRequiredDependency);
	        this.skippedDependencies = this.convertValues(source["skippedDependencies"], ModrinthSkippedDependency);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthInstalledFile {
	    projectId: string;
	    versionId: string;
	    versionName: string;
	    versionNumber: string;
	    fileName: string;
	    displayName: string;
	    dependencyType?: string;
	    alreadyPresent?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthInstalledFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.versionId = source["versionId"];
	        this.versionName = source["versionName"];
	        this.versionNumber = source["versionNumber"];
	        this.fileName = source["fileName"];
	        this.displayName = source["displayName"];
	        this.dependencyType = source["dependencyType"];
	        this.alreadyPresent = source["alreadyPresent"];
	    }
	}
	export class ModrinthInstallResult {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    versionId: string;
	    versionName: string;
	    versionNumber: string;
	    fileName: string;
	    modList: ModList;
	    dependencies?: ModrinthDependency[];
	    installedFiles?: ModrinthInstalledFile[];
	    skippedDependencies?: ModrinthSkippedDependency[];
	
	    static createFrom(source: any = {}) {
	        return new ModrinthInstallResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.versionId = source["versionId"];
	        this.versionName = source["versionName"];
	        this.versionNumber = source["versionNumber"];
	        this.fileName = source["fileName"];
	        this.modList = this.convertValues(source["modList"], ModList);
	        this.dependencies = this.convertValues(source["dependencies"], ModrinthDependency);
	        this.installedFiles = this.convertValues(source["installedFiles"], ModrinthInstalledFile);
	        this.skippedDependencies = this.convertValues(source["skippedDependencies"], ModrinthSkippedDependency);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ModrinthProject {
	    projectId: string;
	    slug: string;
	    title: string;
	    description: string;
	    body?: string;
	    author: string;
	    iconUrl?: string;
	    downloads: number;
	    followers?: number;
	    latestVersion?: string;
	    clientSide?: string;
	    serverSide?: string;
	    licenseName?: string;
	    sourceUrl?: string;
	    issuesUrl?: string;
	    wikiUrl?: string;
	    discordUrl?: string;
	    categories?: string[];
	    gameVersions?: string[];
	    loaders?: string[];
	    displayVersion?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthProject(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.projectId = source["projectId"];
	        this.slug = source["slug"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.body = source["body"];
	        this.author = source["author"];
	        this.iconUrl = source["iconUrl"];
	        this.downloads = source["downloads"];
	        this.followers = source["followers"];
	        this.latestVersion = source["latestVersion"];
	        this.clientSide = source["clientSide"];
	        this.serverSide = source["serverSide"];
	        this.licenseName = source["licenseName"];
	        this.sourceUrl = source["sourceUrl"];
	        this.issuesUrl = source["issuesUrl"];
	        this.wikiUrl = source["wikiUrl"];
	        this.discordUrl = source["discordUrl"];
	        this.categories = source["categories"];
	        this.gameVersions = source["gameVersions"];
	        this.loaders = source["loaders"];
	        this.displayVersion = source["displayVersion"];
	    }
	}
	
	export class ModrinthSearchResult {
	    profileId: string;
	    query: string;
	    minecraftVersion: string;
	    loader: string;
	    totalHits: number;
	    hits: ModrinthProject[];
	
	    static createFrom(source: any = {}) {
	        return new ModrinthSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.query = source["query"];
	        this.minecraftVersion = source["minecraftVersion"];
	        this.loader = source["loader"];
	        this.totalHits = source["totalHits"];
	        this.hits = this.convertValues(source["hits"], ModrinthProject);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ModrinthUpdatePlan {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    tracked: boolean;
	    currentVersionId: string;
	    currentVersionName: string;
	    currentVersionNumber: string;
	    currentFileName: string;
	    latestVersionId: string;
	    latestVersionName: string;
	    latestVersionNumber: string;
	    latestFileName: string;
	    updateAvailable: boolean;
	    requiredDependencies?: ModrinthRequiredDependency[];
	    skippedDependencies?: ModrinthSkippedDependency[];
	    checkError?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthUpdatePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.tracked = source["tracked"];
	        this.currentVersionId = source["currentVersionId"];
	        this.currentVersionName = source["currentVersionName"];
	        this.currentVersionNumber = source["currentVersionNumber"];
	        this.currentFileName = source["currentFileName"];
	        this.latestVersionId = source["latestVersionId"];
	        this.latestVersionName = source["latestVersionName"];
	        this.latestVersionNumber = source["latestVersionNumber"];
	        this.latestFileName = source["latestFileName"];
	        this.updateAvailable = source["updateAvailable"];
	        this.requiredDependencies = this.convertValues(source["requiredDependencies"], ModrinthRequiredDependency);
	        this.skippedDependencies = this.convertValues(source["skippedDependencies"], ModrinthSkippedDependency);
	        this.checkError = source["checkError"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthUpdateResult {
	    profileId: string;
	    projectId: string;
	    projectTitle: string;
	    updated: boolean;
	    oldFileName: string;
	    newFileName: string;
	    modList: ModList;
	    installedFiles?: ModrinthInstalledFile[];
	    deletedFiles?: ModrinthDeleteFile[];
	    skippedFiles?: ModrinthDeleteFile[];
	    skippedDependencies?: ModrinthSkippedDependency[];
	
	    static createFrom(source: any = {}) {
	        return new ModrinthUpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.projectId = source["projectId"];
	        this.projectTitle = source["projectTitle"];
	        this.updated = source["updated"];
	        this.oldFileName = source["oldFileName"];
	        this.newFileName = source["newFileName"];
	        this.modList = this.convertValues(source["modList"], ModList);
	        this.installedFiles = this.convertValues(source["installedFiles"], ModrinthInstalledFile);
	        this.deletedFiles = this.convertValues(source["deletedFiles"], ModrinthDeleteFile);
	        this.skippedFiles = this.convertValues(source["skippedFiles"], ModrinthDeleteFile);
	        this.skippedDependencies = this.convertValues(source["skippedDependencies"], ModrinthSkippedDependency);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModrinthVersionFile {
	    url: string;
	    fileName: string;
	    size: number;
	    primary: boolean;
	    sha1?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModrinthVersionFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.fileName = source["fileName"];
	        this.size = source["size"];
	        this.primary = source["primary"];
	        this.sha1 = source["sha1"];
	    }
	}
	export class ModrinthVersion {
	    id: string;
	    projectId: string;
	    name: string;
	    versionNumber: string;
	    versionType: string;
	    datePublished?: string;
	    changelog?: string;
	    gameVersions: string[];
	    loaders: string[];
	    file: ModrinthVersionFile;
	    dependencies?: ModrinthDependency[];
	
	    static createFrom(source: any = {}) {
	        return new ModrinthVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.projectId = source["projectId"];
	        this.name = source["name"];
	        this.versionNumber = source["versionNumber"];
	        this.versionType = source["versionType"];
	        this.datePublished = source["datePublished"];
	        this.changelog = source["changelog"];
	        this.gameVersions = source["gameVersions"];
	        this.loaders = source["loaders"];
	        this.file = this.convertValues(source["file"], ModrinthVersionFile);
	        this.dependencies = this.convertValues(source["dependencies"], ModrinthDependency);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class NetworkSettings {
	    retryCount: number;
	    metadataTtlHours: number;
	
	    static createFrom(source: any = {}) {
	        return new NetworkSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.retryCount = source["retryCount"];
	        this.metadataTtlHours = source["metadataTtlHours"];
	    }
	}
	
	export class ProfileInput {
	    name: string;
	    minecraftVersion: string;
	    loader: LoaderConfig;
	    account?: AccountConfig;
	    gameDir?: string;
	    memory: MemorySettings;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.minecraftVersion = source["minecraftVersion"];
	        this.loader = this.convertValues(source["loader"], LoaderConfig);
	        this.account = this.convertValues(source["account"], AccountConfig);
	        this.gameDir = source["gameDir"];
	        this.memory = this.convertValues(source["memory"], MemorySettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProfileJavaRuntime {
	    profileId: string;
	    requiredMajor: number;
	    installed: boolean;
	    javaPath?: string;
	    version?: string;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new ProfileJavaRuntime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.profileId = source["profileId"];
	        this.requiredMajor = source["requiredMajor"];
	        this.installed = source["installed"];
	        this.javaPath = source["javaPath"];
	        this.version = source["version"];
	        this.message = source["message"];
	    }
	}
	export class ProfileList {
	    selectedProfileId: string;
	    profiles: Profile[];
	
	    static createFrom(source: any = {}) {
	        return new ProfileList(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.selectedProfileId = source["selectedProfileId"];
	        this.profiles = this.convertValues(source["profiles"], Profile);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    dataDir: string;
	    javaPath: string;
	    account: AccountConfig;
	    defaultMemory: MemorySettings;
	    network: NetworkSettings;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.dataDir = source["dataDir"];
	        this.javaPath = source["javaPath"];
	        this.account = this.convertValues(source["account"], AccountConfig);
	        this.defaultMemory = this.convertValues(source["defaultMemory"], MemorySettings);
	        this.network = this.convertValues(source["network"], NetworkSettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class VersionOption {
	    id: string;
	    label: string;
	    type?: string;
	    stable: boolean;
	    latest: boolean;
	
	    static createFrom(source: any = {}) {
	        return new VersionOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.type = source["type"];
	        this.stable = source["stable"];
	        this.latest = source["latest"];
	    }
	}
	export class VersionCatalog {
	    minecraftVersions: VersionOption[];
	    fabricLoaderVersions: VersionOption[];
	    quiltLoaderVersions: VersionOption[];
	    forgeLoaderVersions: VersionOption[];
	    neoForgeLoaderVersions: VersionOption[];
	    minecraftSource: string;
	    fabricLoaderSource: string;
	    quiltLoaderSource: string;
	    forgeLoaderSource: string;
	    neoForgeLoaderSource: string;
	    minecraftUpdatedAt?: string;
	    fabricLoaderUpdatedAt?: string;
	    quiltLoaderUpdatedAt?: string;
	    forgeLoaderUpdatedAt?: string;
	    neoForgeLoaderUpdatedAt?: string;
	    warnings?: string[];
	
	    static createFrom(source: any = {}) {
	        return new VersionCatalog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.minecraftVersions = this.convertValues(source["minecraftVersions"], VersionOption);
	        this.fabricLoaderVersions = this.convertValues(source["fabricLoaderVersions"], VersionOption);
	        this.quiltLoaderVersions = this.convertValues(source["quiltLoaderVersions"], VersionOption);
	        this.forgeLoaderVersions = this.convertValues(source["forgeLoaderVersions"], VersionOption);
	        this.neoForgeLoaderVersions = this.convertValues(source["neoForgeLoaderVersions"], VersionOption);
	        this.minecraftSource = source["minecraftSource"];
	        this.fabricLoaderSource = source["fabricLoaderSource"];
	        this.quiltLoaderSource = source["quiltLoaderSource"];
	        this.forgeLoaderSource = source["forgeLoaderSource"];
	        this.neoForgeLoaderSource = source["neoForgeLoaderSource"];
	        this.minecraftUpdatedAt = source["minecraftUpdatedAt"];
	        this.fabricLoaderUpdatedAt = source["fabricLoaderUpdatedAt"];
	        this.quiltLoaderUpdatedAt = source["quiltLoaderUpdatedAt"];
	        this.forgeLoaderUpdatedAt = source["forgeLoaderUpdatedAt"];
	        this.neoForgeLoaderUpdatedAt = source["neoForgeLoaderUpdatedAt"];
	        this.warnings = source["warnings"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
