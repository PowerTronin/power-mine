package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"power-mine/internal/domain"
	"power-mine/internal/storage"
)

const (
	versionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	fabricLoadersURL   = "https://meta.fabricmc.net/v2/versions/loader"
	quiltLoadersURL    = "https://meta.quiltmc.org/v3/versions/loader"
	forgeLoadersURL    = "https://maven.minecraftforge.net/net/minecraftforge/forge/maven-metadata.xml"
	neoForgeLoadersURL = "https://maven.neoforged.net/releases/net/neoforged/neoforge/maven-metadata.xml"
	modrinthAPIBaseURL = "https://api.modrinth.com/v2"
)

type Service struct {
	client             *http.Client
	versionManifestURL string
	fabricLoadersURL   string
	quiltLoadersURL    string
	forgeLoadersURL    string
	neoForgeLoadersURL string
	modrinthAPIBaseURL string
	cachePath          string
}

func NewService(dataDirs ...string) *Service {
	service := &Service{
		client:             &http.Client{Timeout: 15 * time.Second},
		versionManifestURL: versionManifestURL,
		fabricLoadersURL:   fabricLoadersURL,
		quiltLoadersURL:    quiltLoadersURL,
		forgeLoadersURL:    forgeLoadersURL,
		neoForgeLoadersURL: neoForgeLoadersURL,
		modrinthAPIBaseURL: modrinthAPIBaseURL,
	}
	if len(dataDirs) > 0 && strings.TrimSpace(dataDirs[0]) != "" {
		service.cachePath = filepath.Join(dataDirs[0], "catalog", "version-cache.json")
	}
	return service
}

type versionCatalogCache struct {
	MinecraftVersions       []domain.VersionOption `json:"minecraftVersions"`
	FabricLoaderVersions    []domain.VersionOption `json:"fabricLoaderVersions"`
	QuiltLoaderVersions     []domain.VersionOption `json:"quiltLoaderVersions"`
	ForgeLoaderVersions     []domain.VersionOption `json:"forgeLoaderVersions"`
	NeoForgeLoaderVersions  []domain.VersionOption `json:"neoForgeLoaderVersions"`
	MinecraftUpdatedAt      string                 `json:"minecraftUpdatedAt,omitempty"`
	FabricLoaderUpdatedAt   string                 `json:"fabricLoaderUpdatedAt,omitempty"`
	QuiltLoaderUpdatedAt    string                 `json:"quiltLoaderUpdatedAt,omitempty"`
	ForgeLoaderUpdatedAt    string                 `json:"forgeLoaderUpdatedAt,omitempty"`
	NeoForgeLoaderUpdatedAt string                 `json:"neoForgeLoaderUpdatedAt,omitempty"`
}

type versionManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []struct {
		ID   string `json:"id"`
		Type string `json:"type"`
	} `json:"versions"`
}

type fabricLoaderVersion struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
}

type quiltLoaderVersion struct {
	Version string `json:"version"`
}

type mavenMetadata struct {
	Versioning struct {
		Latest   string   `xml:"latest"`
		Release  string   `xml:"release"`
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

type modrinthSearchResponse struct {
	Hits []struct {
		ProjectID         string   `json:"project_id"`
		Slug              string   `json:"slug"`
		Author            string   `json:"author"`
		Title             string   `json:"title"`
		Description       string   `json:"description"`
		Categories        []string `json:"categories"`
		DisplayCategories []string `json:"display_categories"`
		Versions          []string `json:"versions"`
		Downloads         int      `json:"downloads"`
		IconURL           string   `json:"icon_url"`
		LatestVersion     string   `json:"latest_version"`
		ClientSide        string   `json:"client_side"`
		ServerSide        string   `json:"server_side"`
	} `json:"hits"`
	TotalHits int `json:"total_hits"`
}

type modrinthVersionResponse struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"project_id"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	VersionType   string   `json:"version_type"`
	DatePublished string   `json:"date_published"`
	Changelog     string   `json:"changelog"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Dependencies  []struct {
		VersionID      string `json:"version_id"`
		ProjectID      string `json:"project_id"`
		FileName       string `json:"file_name"`
		DependencyType string `json:"dependency_type"`
	} `json:"dependencies"`
	Files []struct {
		Hashes   map[string]string `json:"hashes"`
		URL      string            `json:"url"`
		FileName string            `json:"filename"`
		Primary  bool              `json:"primary"`
		Size     int64             `json:"size"`
		FileType string            `json:"file_type"`
	} `json:"files"`
}

type modrinthProjectResponse struct {
	ID                   string   `json:"id"`
	Slug                 string   `json:"slug"`
	ProjectType          string   `json:"project_type"`
	Team                 string   `json:"team"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	Body                 string   `json:"body"`
	Categories           []string `json:"categories"`
	AdditionalCategories []string `json:"additional_categories"`
	GameVersions         []string `json:"game_versions"`
	Loaders              []string `json:"loaders"`
	ClientSide           string   `json:"client_side"`
	ServerSide           string   `json:"server_side"`
	Downloads            int      `json:"downloads"`
	Followers            int      `json:"followers"`
	IconURL              string   `json:"icon_url"`
	SourceURL            string   `json:"source_url"`
	IssuesURL            string   `json:"issues_url"`
	WikiURL              string   `json:"wiki_url"`
	DiscordURL           string   `json:"discord_url"`
	License              struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"license"`
}

func (s *Service) MinecraftVersions(ctx context.Context) ([]domain.VersionOption, error) {
	return s.fetchMinecraftVersions(ctx)
}

func (s *Service) FabricLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	return s.fetchFabricLoaderVersions(ctx)
}

func (s *Service) QuiltLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	return s.fetchQuiltLoaderVersions(ctx)
}

func (s *Service) ForgeLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	return s.fetchForgeLoaderVersions(ctx)
}

func (s *Service) NeoForgeLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	return s.fetchNeoForgeLoaderVersions(ctx)
}

func (s *Service) CachedVersionCatalog() (domain.VersionCatalog, error) {
	cache, err := s.readVersionCatalogCache()
	if err != nil {
		return domain.VersionCatalog{}, err
	}
	return s.catalogFromCache(cache), nil
}

func (s *Service) VersionCatalog(ctx context.Context, ttlHours int, retryCount int) (domain.VersionCatalog, error) {
	cache, err := s.readVersionCatalogCache()
	var warnings []string
	if err != nil {
		warnings = append(warnings, "Version catalog cache could not be read: "+err.Error())
		cache = versionCatalogCache{}
	}

	result := s.catalogFromCache(cache)
	result.Warnings = append(result.Warnings, warnings...)

	refreshMinecraft := !cacheFresh(cache.MinecraftUpdatedAt, ttlHours) || len(cache.MinecraftVersions) == 0
	refreshFabric := !cacheFresh(cache.FabricLoaderUpdatedAt, ttlHours) || len(cache.FabricLoaderVersions) == 0
	refreshQuilt := !cacheFresh(cache.QuiltLoaderUpdatedAt, ttlHours) || len(cache.QuiltLoaderVersions) == 0
	refreshForge := !cacheFresh(cache.ForgeLoaderUpdatedAt, ttlHours) || len(cache.ForgeLoaderVersions) == 0
	refreshNeoForge := !cacheFresh(cache.NeoForgeLoaderUpdatedAt, ttlHours) || len(cache.NeoForgeLoaderVersions) == 0
	if !refreshMinecraft && !refreshFabric && !refreshQuilt && !refreshForge && !refreshNeoForge {
		return result, nil
	}

	type fetchResult struct {
		kind     string
		versions []domain.VersionOption
		err      error
	}
	results := make(chan fetchResult, 5)
	var wait sync.WaitGroup
	if refreshMinecraft {
		wait.Add(1)
		go func() {
			defer wait.Done()
			versions, err := s.fetchVersionOptionsWithRetry(ctx, retryCount, s.fetchMinecraftVersions)
			results <- fetchResult{kind: "minecraft", versions: versions, err: err}
		}()
	}
	if refreshFabric {
		wait.Add(1)
		go func() {
			defer wait.Done()
			versions, err := s.fetchVersionOptionsWithRetry(ctx, retryCount, s.fetchFabricLoaderVersions)
			results <- fetchResult{kind: "fabric", versions: versions, err: err}
		}()
	}
	if refreshQuilt {
		wait.Add(1)
		go func() {
			defer wait.Done()
			versions, err := s.fetchVersionOptionsWithRetry(ctx, retryCount, s.fetchQuiltLoaderVersions)
			results <- fetchResult{kind: "quilt", versions: versions, err: err}
		}()
	}
	if refreshForge {
		wait.Add(1)
		go func() {
			defer wait.Done()
			versions, err := s.fetchVersionOptionsWithRetry(ctx, retryCount, s.fetchForgeLoaderVersions)
			results <- fetchResult{kind: "forge", versions: versions, err: err}
		}()
	}
	if refreshNeoForge {
		wait.Add(1)
		go func() {
			defer wait.Done()
			versions, err := s.fetchVersionOptionsWithRetry(ctx, retryCount, s.fetchNeoForgeLoaderVersions)
			results <- fetchResult{kind: "neoforge", versions: versions, err: err}
		}()
	}

	go func() {
		wait.Wait()
		close(results)
	}()

	now := time.Now().UTC().Format(time.RFC3339)
	updated := false
	for fetch := range results {
		switch fetch.kind {
		case "minecraft":
			if fetch.err != nil {
				result.Warnings = append(result.Warnings, "Minecraft versions unavailable: "+fetch.err.Error())
				if len(result.MinecraftVersions) == 0 {
					result.MinecraftSource = "empty"
				}
				continue
			}
			cache.MinecraftVersions = fetch.versions
			cache.MinecraftUpdatedAt = now
			result.MinecraftVersions = fetch.versions
			result.MinecraftSource = "network"
			result.MinecraftUpdatedAt = now
			updated = true
		case "fabric":
			if fetch.err != nil {
				result.Warnings = append(result.Warnings, "Fabric loader versions unavailable: "+fetch.err.Error())
				if len(result.FabricLoaderVersions) == 0 {
					result.FabricLoaderVersions = FallbackFabricLoaderVersions()
					result.FabricLoaderSource = "fallback"
				}
				continue
			}
			cache.FabricLoaderVersions = fetch.versions
			cache.FabricLoaderUpdatedAt = now
			result.FabricLoaderVersions = fetch.versions
			result.FabricLoaderSource = "network"
			result.FabricLoaderUpdatedAt = now
			updated = true
		case "quilt":
			if fetch.err != nil {
				result.Warnings = append(result.Warnings, "Quilt loader versions unavailable: "+fetch.err.Error())
				if len(result.QuiltLoaderVersions) == 0 {
					result.QuiltLoaderVersions = FallbackQuiltLoaderVersions()
					result.QuiltLoaderSource = "fallback"
				}
				continue
			}
			cache.QuiltLoaderVersions = fetch.versions
			cache.QuiltLoaderUpdatedAt = now
			result.QuiltLoaderVersions = fetch.versions
			result.QuiltLoaderSource = "network"
			result.QuiltLoaderUpdatedAt = now
			updated = true
		case "forge":
			if fetch.err != nil {
				result.Warnings = append(result.Warnings, "Forge loader versions unavailable: "+fetch.err.Error())
				if len(result.ForgeLoaderVersions) == 0 {
					result.ForgeLoaderVersions = FallbackForgeLoaderVersions()
					result.ForgeLoaderSource = "fallback"
				}
				continue
			}
			cache.ForgeLoaderVersions = fetch.versions
			cache.ForgeLoaderUpdatedAt = now
			result.ForgeLoaderVersions = fetch.versions
			result.ForgeLoaderSource = "network"
			result.ForgeLoaderUpdatedAt = now
			updated = true
		case "neoforge":
			if fetch.err != nil {
				result.Warnings = append(result.Warnings, "NeoForge loader versions unavailable: "+fetch.err.Error())
				if len(result.NeoForgeLoaderVersions) == 0 {
					result.NeoForgeLoaderVersions = FallbackNeoForgeLoaderVersions()
					result.NeoForgeLoaderSource = "fallback"
				}
				continue
			}
			cache.NeoForgeLoaderVersions = fetch.versions
			cache.NeoForgeLoaderUpdatedAt = now
			result.NeoForgeLoaderVersions = fetch.versions
			result.NeoForgeLoaderSource = "network"
			result.NeoForgeLoaderUpdatedAt = now
			updated = true
		}
	}
	if updated && s.cachePath != "" {
		if err := storage.WriteJSON(s.cachePath, cache); err != nil {
			result.Warnings = append(result.Warnings, "Version catalog cache could not be saved: "+err.Error())
		}
	}
	return result, nil
}

func (s *Service) fetchMinecraftVersions(ctx context.Context) ([]domain.VersionOption, error) {
	var manifest versionManifest
	if err := s.getJSON(ctx, s.versionManifestURL, &manifest); err != nil {
		return nil, err
	}

	options := make([]domain.VersionOption, 0, len(manifest.Versions))
	for _, version := range manifest.Versions {
		latest := version.ID == manifest.Latest.Release || version.ID == manifest.Latest.Snapshot
		label := version.ID
		if version.ID == manifest.Latest.Release {
			label += " (latest release)"
		} else if version.ID == manifest.Latest.Snapshot {
			label += " (latest snapshot)"
		}

		options = append(options, domain.VersionOption{
			ID:     version.ID,
			Label:  label,
			Type:   version.Type,
			Stable: version.Type == "release",
			Latest: latest,
		})
	}
	return options, nil
}

func (s *Service) fetchFabricLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	var versions []fabricLoaderVersion
	if err := s.getJSON(ctx, s.fabricLoadersURL, &versions); err != nil {
		return nil, err
	}

	options := make([]domain.VersionOption, 0, len(versions))
	for index, version := range versions {
		label := version.Version
		if index == 0 {
			label += " (latest)"
		}
		if version.Stable {
			label += " stable"
		}
		options = append(options, domain.VersionOption{
			ID:     version.Version,
			Label:  label,
			Type:   "fabric-loader",
			Stable: version.Stable,
			Latest: index == 0,
		})
	}
	return options, nil
}

func (s *Service) fetchQuiltLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	var versions []quiltLoaderVersion
	if err := s.getJSON(ctx, s.quiltLoadersURL, &versions); err != nil {
		return nil, err
	}

	options := make([]domain.VersionOption, 0, len(versions))
	for index, version := range versions {
		label := version.Version
		if index == 0 {
			label += " (latest)"
		}
		stable := !strings.Contains(version.Version, "alpha") && !strings.Contains(version.Version, "beta")
		if stable {
			label += " stable"
		}
		options = append(options, domain.VersionOption{
			ID:     version.Version,
			Label:  label,
			Type:   "quilt-loader",
			Stable: stable,
			Latest: index == 0,
		})
	}
	return options, nil
}

func (s *Service) fetchForgeLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	var metadata mavenMetadata
	if err := s.getXML(ctx, s.forgeLoadersURL, &metadata); err != nil {
		return nil, err
	}

	versions := append([]string{}, metadata.Versioning.Versions...)
	sort.SliceStable(versions, func(left int, right int) bool {
		return compareDottedVersions(versions[left], versions[right]) > 0
	})

	options := make([]domain.VersionOption, 0, len(versions))
	latest := metadata.Versioning.Release
	if latest == "" {
		latest = metadata.Versioning.Latest
	}
	for _, version := range versions {
		minecraftVersion, forgeVersion, ok := splitForgeMavenVersion(version)
		if !ok {
			continue
		}
		label := fmt.Sprintf("%s (%s)", forgeVersion, minecraftVersion)
		isLatest := version == latest
		if isLatest {
			label += " latest"
		}
		stable := !strings.Contains(strings.ToLower(version), "alpha") &&
			!strings.Contains(strings.ToLower(version), "beta") &&
			!strings.Contains(strings.ToLower(version), "rc")
		if stable {
			label += " stable"
		}
		options = append(options, domain.VersionOption{
			ID:     version,
			Label:  label,
			Type:   "forge-loader",
			Stable: stable,
			Latest: isLatest,
		})
	}
	return options, nil
}

func (s *Service) fetchNeoForgeLoaderVersions(ctx context.Context) ([]domain.VersionOption, error) {
	var metadata mavenMetadata
	if err := s.getXML(ctx, s.neoForgeLoadersURL, &metadata); err != nil {
		return nil, err
	}

	versions := append([]string{}, metadata.Versioning.Versions...)
	sort.SliceStable(versions, func(left int, right int) bool {
		return compareDottedVersions(versions[left], versions[right]) > 0
	})

	options := make([]domain.VersionOption, 0, len(versions))
	latest := metadata.Versioning.Release
	if latest == "" {
		latest = metadata.Versioning.Latest
	}
	for _, version := range versions {
		minecraftVersion := neoForgeMinecraftVersion(version)
		if minecraftVersion == "" {
			continue
		}
		label := fmt.Sprintf("%s (%s)", version, minecraftVersion)
		isLatest := version == latest
		if isLatest {
			label += " latest"
		}
		stable := !strings.Contains(strings.ToLower(version), "alpha") &&
			!strings.Contains(strings.ToLower(version), "beta") &&
			!strings.Contains(strings.ToLower(version), "rc")
		if stable {
			label += " stable"
		}
		options = append(options, domain.VersionOption{
			ID:     version,
			Label:  label,
			Type:   "neoforge-loader",
			Stable: stable,
			Latest: isLatest,
		})
	}
	return options, nil
}

func (s *Service) readVersionCatalogCache() (versionCatalogCache, error) {
	if s.cachePath == "" {
		return versionCatalogCache{}, nil
	}
	return storage.ReadJSON(s.cachePath, versionCatalogCache{})
}

func (s *Service) catalogFromCache(cache versionCatalogCache) domain.VersionCatalog {
	catalog := domain.VersionCatalog{
		MinecraftVersions:       cache.MinecraftVersions,
		FabricLoaderVersions:    cache.FabricLoaderVersions,
		QuiltLoaderVersions:     cache.QuiltLoaderVersions,
		ForgeLoaderVersions:     cache.ForgeLoaderVersions,
		NeoForgeLoaderVersions:  cache.NeoForgeLoaderVersions,
		MinecraftUpdatedAt:      cache.MinecraftUpdatedAt,
		FabricLoaderUpdatedAt:   cache.FabricLoaderUpdatedAt,
		QuiltLoaderUpdatedAt:    cache.QuiltLoaderUpdatedAt,
		ForgeLoaderUpdatedAt:    cache.ForgeLoaderUpdatedAt,
		NeoForgeLoaderUpdatedAt: cache.NeoForgeLoaderUpdatedAt,
		MinecraftSource:         "empty",
		FabricLoaderSource:      "empty",
		QuiltLoaderSource:       "empty",
		ForgeLoaderSource:       "empty",
		NeoForgeLoaderSource:    "empty",
	}
	if len(cache.MinecraftVersions) > 0 {
		catalog.MinecraftSource = "cache"
	}
	if len(cache.FabricLoaderVersions) > 0 {
		catalog.FabricLoaderSource = "cache"
	} else {
		catalog.FabricLoaderVersions = FallbackFabricLoaderVersions()
		catalog.FabricLoaderSource = "fallback"
	}
	if len(cache.QuiltLoaderVersions) > 0 {
		catalog.QuiltLoaderSource = "cache"
	} else {
		catalog.QuiltLoaderVersions = FallbackQuiltLoaderVersions()
		catalog.QuiltLoaderSource = "fallback"
	}
	if len(cache.ForgeLoaderVersions) > 0 {
		catalog.ForgeLoaderSource = "cache"
	} else {
		catalog.ForgeLoaderVersions = FallbackForgeLoaderVersions()
		catalog.ForgeLoaderSource = "fallback"
	}
	if len(cache.NeoForgeLoaderVersions) > 0 {
		catalog.NeoForgeLoaderSource = "cache"
	} else {
		catalog.NeoForgeLoaderVersions = FallbackNeoForgeLoaderVersions()
		catalog.NeoForgeLoaderSource = "fallback"
	}
	return catalog
}

func FallbackFabricLoaderVersions() []domain.VersionOption {
	return []domain.VersionOption{{
		ID:     "latest",
		Label:  "Latest compatible (resolved during install)",
		Type:   "fabric-loader",
		Stable: true,
		Latest: true,
	}}
}

func FallbackQuiltLoaderVersions() []domain.VersionOption {
	return []domain.VersionOption{{
		ID:     "latest",
		Label:  "Latest compatible (resolved during install)",
		Type:   "quilt-loader",
		Stable: true,
		Latest: true,
	}}
}

func FallbackForgeLoaderVersions() []domain.VersionOption {
	return []domain.VersionOption{{
		ID:     "latest",
		Label:  "Latest compatible (resolved during install)",
		Type:   "forge-loader",
		Stable: true,
		Latest: true,
	}}
}

func FallbackNeoForgeLoaderVersions() []domain.VersionOption {
	return []domain.VersionOption{{
		ID:     "latest",
		Label:  "Latest compatible (resolved during install)",
		Type:   "neoforge-loader",
		Stable: true,
		Latest: true,
	}}
}

func (s *Service) fetchVersionOptionsWithRetry(
	ctx context.Context,
	retryCount int,
	fetch func(context.Context) ([]domain.VersionOption, error),
) ([]domain.VersionOption, error) {
	if retryCount < 0 {
		retryCount = 0
	}
	if retryCount > 3 {
		retryCount = 3
	}
	attempts := retryCount + 1
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		versions, err := fetch(attemptCtx)
		cancel()
		if err == nil {
			return versions, nil
		}
		lastErr = err
		if attempt < attempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

func cacheFresh(updatedAt string, ttlHours int) bool {
	if updatedAt == "" {
		return false
	}
	if ttlHours <= 0 {
		return false
	}
	updated, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return false
	}
	return time.Since(updated) < time.Duration(ttlHours)*time.Hour
}

func (s *Service) SearchModrinthMods(ctx context.Context, profile domain.Profile, query string, limit int) (domain.ModrinthSearchResult, error) {
	loader, err := modrinthLoader(profile.Loader.Type)
	if err != nil {
		return domain.ModrinthSearchResult{}, err
	}
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	endpoint, err := s.modrinthURL("/search")
	if err != nil {
		return domain.ModrinthSearchResult{}, err
	}
	params := endpoint.Query()
	params.Set("query", query)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("index", "relevance")
	params.Set("facets", modrinthFacets(profile.MinecraftVersion, loader))
	endpoint.RawQuery = params.Encode()

	var response modrinthSearchResponse
	if err := s.getJSON(ctx, endpoint.String(), &response); err != nil {
		return domain.ModrinthSearchResult{}, err
	}

	hits := make([]domain.ModrinthProject, 0, len(response.Hits))
	for _, hit := range response.Hits {
		categories := hit.DisplayCategories
		if len(categories) == 0 {
			categories = hit.Categories
		}
		hits = append(hits, domain.ModrinthProject{
			ProjectID:      hit.ProjectID,
			Slug:           hit.Slug,
			Title:          hit.Title,
			Description:    hit.Description,
			Author:         hit.Author,
			IconURL:        hit.IconURL,
			Downloads:      hit.Downloads,
			LatestVersion:  hit.LatestVersion,
			ClientSide:     hit.ClientSide,
			ServerSide:     hit.ServerSide,
			Categories:     categories,
			GameVersions:   hit.Versions,
			DisplayVersion: pickDisplayVersion(hit.Versions, profile.MinecraftVersion, hit.LatestVersion),
		})
	}

	return domain.ModrinthSearchResult{
		ProfileID:        profile.ID,
		Query:            query,
		MinecraftVersion: profile.MinecraftVersion,
		Loader:           loader,
		TotalHits:        response.TotalHits,
		Hits:             hits,
	}, nil
}

func (s *Service) LatestModrinthVersion(ctx context.Context, profile domain.Profile, projectID string) (domain.ModrinthVersion, error) {
	loader, err := modrinthLoader(profile.Loader.Type)
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return domain.ModrinthVersion{}, fmt.Errorf("modrinth project id is required")
	}

	versions, err := s.modrinthProjectVersions(ctx, profile, projectID, loader)
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	if len(versions) == 0 {
		return domain.ModrinthVersion{}, fmt.Errorf("no downloadable .jar file found for compatible Modrinth versions")
	}
	return versions[0], nil
}

func (s *Service) ModrinthProjectVersions(ctx context.Context, profile domain.Profile, projectID string) ([]domain.ModrinthVersion, error) {
	loader, err := modrinthLoader(profile.Loader.Type)
	if err != nil {
		return nil, err
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("modrinth project id is required")
	}
	return s.modrinthProjectVersions(ctx, profile, projectID, loader)
}

func (s *Service) LatestModrinthVersionFromHash(ctx context.Context, profile domain.Profile, hash string) (domain.ModrinthVersion, error) {
	loader, err := modrinthLoader(profile.Loader.Type)
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return domain.ModrinthVersion{}, fmt.Errorf("modrinth file hash is required")
	}

	endpoint, err := s.modrinthURL(path.Join("/version_file", url.PathEscape(hash), "update"))
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	params := endpoint.Query()
	params.Set("algorithm", "sha1")
	endpoint.RawQuery = params.Encode()

	requestBody := struct {
		Loaders      []string `json:"loaders"`
		GameVersions []string `json:"game_versions"`
	}{
		Loaders:      []string{loader},
		GameVersions: []string{profile.MinecraftVersion},
	}

	var response modrinthVersionResponse
	if err := s.postJSON(ctx, endpoint.String(), requestBody, &response); err != nil {
		return domain.ModrinthVersion{}, err
	}

	version, ok := convertModrinthVersion(response)
	if !ok {
		return domain.ModrinthVersion{}, fmt.Errorf("no downloadable .jar file found for compatible Modrinth version")
	}
	if !modrinthVersionSupportsProfile(version, profile.MinecraftVersion, loader) {
		return domain.ModrinthVersion{}, fmt.Errorf("Modrinth version %s is not compatible with Minecraft %s and %s", version.ID, profile.MinecraftVersion, loader)
	}
	return version, nil
}

func (s *Service) ModrinthVersionFromHash(ctx context.Context, hash string) (domain.ModrinthVersion, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return domain.ModrinthVersion{}, fmt.Errorf("modrinth file hash is required")
	}

	endpoint, err := s.modrinthURL(path.Join("/version_file", url.PathEscape(hash)))
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	params := endpoint.Query()
	params.Set("algorithm", "sha1")
	endpoint.RawQuery = params.Encode()

	var response modrinthVersionResponse
	if err := s.getJSON(ctx, endpoint.String(), &response); err != nil {
		return domain.ModrinthVersion{}, err
	}

	version, ok := convertModrinthVersion(response)
	if !ok {
		return domain.ModrinthVersion{}, fmt.Errorf("no downloadable .jar file found for Modrinth version from hash")
	}
	return version, nil
}

func (s *Service) ModrinthProjectVersion(ctx context.Context, profile domain.Profile, projectID string, versionID string) (domain.ModrinthVersion, error) {
	loader, err := modrinthLoader(profile.Loader.Type)
	if err != nil {
		return domain.ModrinthVersion{}, err
	}
	projectID = strings.TrimSpace(projectID)
	versionID = strings.TrimSpace(versionID)
	if projectID == "" {
		return domain.ModrinthVersion{}, fmt.Errorf("modrinth project id is required")
	}
	if versionID == "" {
		return domain.ModrinthVersion{}, fmt.Errorf("modrinth version id is required")
	}

	endpoint, err := s.modrinthURL(path.Join("/project", url.PathEscape(projectID), "version", url.PathEscape(versionID)))
	if err != nil {
		return domain.ModrinthVersion{}, err
	}

	var response modrinthVersionResponse
	if err := s.getJSON(ctx, endpoint.String(), &response); err != nil {
		return domain.ModrinthVersion{}, err
	}

	version, ok := convertModrinthVersion(response)
	if !ok {
		return domain.ModrinthVersion{}, fmt.Errorf("no downloadable .jar file found for Modrinth version %s", versionID)
	}
	if !modrinthVersionSupportsProfile(version, profile.MinecraftVersion, loader) {
		return domain.ModrinthVersion{}, fmt.Errorf("Modrinth version %s is not compatible with Minecraft %s and %s", versionID, profile.MinecraftVersion, loader)
	}
	return version, nil
}

func (s *Service) modrinthProjectVersions(ctx context.Context, profile domain.Profile, projectID string, loader string) ([]domain.ModrinthVersion, error) {
	endpoint, err := s.modrinthURL(path.Join("/project", url.PathEscape(projectID), "version"))
	if err != nil {
		return nil, err
	}
	params := endpoint.Query()
	params.Set("loaders", jsonArray(loader))
	params.Set("game_versions", jsonArray(profile.MinecraftVersion))
	endpoint.RawQuery = params.Encode()

	var versions []modrinthVersionResponse
	if err := s.getJSON(ctx, endpoint.String(), &versions); err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no compatible Modrinth version found for Minecraft %s and %s", profile.MinecraftVersion, loader)
	}

	convertedVersions := make([]domain.ModrinthVersion, 0, len(versions))
	for _, version := range versions {
		if converted, ok := convertModrinthVersion(version); ok {
			convertedVersions = append(convertedVersions, converted)
		}
	}
	return convertedVersions, nil
}

func (s *Service) ModrinthProject(ctx context.Context, projectID string) (domain.ModrinthProject, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return domain.ModrinthProject{}, fmt.Errorf("modrinth project id is required")
	}

	endpoint, err := s.modrinthURL(path.Join("/project", url.PathEscape(projectID)))
	if err != nil {
		return domain.ModrinthProject{}, err
	}

	var response modrinthProjectResponse
	if err := s.getJSON(ctx, endpoint.String(), &response); err != nil {
		return domain.ModrinthProject{}, err
	}

	categories := append([]string{}, response.Categories...)
	categories = append(categories, response.AdditionalCategories...)
	licenseName := response.License.Name
	if licenseName == "" {
		licenseName = response.License.ID
	}

	return domain.ModrinthProject{
		ProjectID:     response.ID,
		Slug:          response.Slug,
		Title:         response.Title,
		Description:   response.Description,
		Body:          response.Body,
		IconURL:       response.IconURL,
		Downloads:     response.Downloads,
		Followers:     response.Followers,
		ClientSide:    response.ClientSide,
		ServerSide:    response.ServerSide,
		LicenseName:   licenseName,
		SourceURL:     response.SourceURL,
		IssuesURL:     response.IssuesURL,
		WikiURL:       response.WikiURL,
		DiscordURL:    response.DiscordURL,
		Categories:    categories,
		GameVersions:  response.GameVersions,
		Loaders:       response.Loaders,
		LatestVersion: pickDisplayVersion(response.GameVersions, "", ""),
	}, nil
}

func (s *Service) OpenDownload(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	request, err := s.newRequest(ctx, http.MethodGet, rawURL)
	if err != nil {
		return nil, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		return nil, fmt.Errorf("download request failed: %s returned %s", rawURL, response.Status)
	}
	return response.Body, nil
}

func (s *Service) getJSON(ctx context.Context, url string, target any) error {
	request, err := s.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("metadata request failed: %s returned %s", url, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("metadata response decode failed: %w", err)
	}
	return nil
}

func (s *Service) getXML(ctx context.Context, url string, target any) error {
	request, err := s.newRequest(ctx, http.MethodGet, url)
	if err != nil {
		return err
	}

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("metadata request failed: %s returned %s", url, response.Status)
	}
	if err := xml.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("metadata response decode failed: %w", err)
	}
	return nil
}

func (s *Service) postJSON(ctx context.Context, url string, body any, target any) error {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(body); err != nil {
		return err
	}

	request, err := s.newRequest(ctx, http.MethodPost, url)
	if err != nil {
		return err
	}
	request.Body = io.NopCloser(&buffer)
	request.ContentLength = int64(buffer.Len())
	request.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("metadata request failed: %s returned %s", url, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("metadata response decode failed: %w", err)
	}
	return nil
}

func (s *Service) newRequest(ctx context.Context, method string, rawURL string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Power-Mine-Launcher/0.1")
	return request, nil
}

func (s *Service) modrinthURL(endpoint string) (*url.URL, error) {
	baseURL, err := url.Parse(s.modrinthAPIBaseURL)
	if err != nil {
		return nil, err
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + endpoint
	return baseURL, nil
}

func modrinthLoader(loader domain.LoaderType) (string, error) {
	switch loader {
	case domain.LoaderFabric:
		return "fabric", nil
	case domain.LoaderQuilt:
		return "quilt", nil
	case domain.LoaderForge:
		return "forge", nil
	case domain.LoaderNeoForge:
		return "neoforge", nil
	default:
		return "", fmt.Errorf("Modrinth mod install requires a mod loader profile")
	}
}

func splitForgeMavenVersion(version string) (string, string, bool) {
	minecraftVersion, forgeVersion, ok := strings.Cut(strings.TrimSpace(version), "-")
	if !ok || minecraftVersion == "" || forgeVersion == "" {
		return "", "", false
	}
	return minecraftVersion, forgeVersion, true
}

func neoForgeMinecraftVersion(version string) string {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 {
		return ""
	}
	major := numericPrefix(parts[0])
	minor := numericPrefix(parts[1])
	if major == "" || minor == "" {
		return ""
	}
	return "1." + major + "." + minor
}

func numericPrefix(value string) string {
	var builder strings.Builder
	for _, char := range value {
		if char < '0' || char > '9' {
			break
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

func compareDottedVersions(left string, right string) int {
	leftParts := splitVersionParts(left)
	rightParts := splitVersionParts(right)
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}
	for index := 0; index < max; index++ {
		leftPart := versionPart{}
		rightPart := versionPart{}
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

type versionPart struct {
	Number int
	Suffix string
}

func splitVersionParts(version string) []versionPart {
	fields := strings.FieldsFunc(version, func(char rune) bool {
		return char == '.' || char == '-'
	})
	parts := make([]versionPart, 0, len(fields))
	for _, field := range fields {
		part := versionPart{}
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

func modrinthFacets(minecraftVersion string, loader string) string {
	facets := [][]string{
		{"project_type:mod"},
		{"versions:" + minecraftVersion},
		{"categories:" + loader},
	}
	data, err := json.Marshal(facets)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func modrinthVersionSupportsProfile(version domain.ModrinthVersion, minecraftVersion string, loader string) bool {
	return stringSliceContains(version.GameVersions, minecraftVersion) && stringSliceContains(version.Loaders, loader)
}

func jsonArray(value string) string {
	data, err := json.Marshal([]string{value})
	if err != nil {
		return "[]"
	}
	return string(data)
}

func convertModrinthVersion(version modrinthVersionResponse) (domain.ModrinthVersion, bool) {
	file, ok := pickModrinthFile(version.Files)
	if !ok {
		return domain.ModrinthVersion{}, false
	}

	dependencies := make([]domain.ModrinthDependency, 0, len(version.Dependencies))
	for _, dependency := range version.Dependencies {
		dependencies = append(dependencies, domain.ModrinthDependency{
			VersionID:      dependency.VersionID,
			ProjectID:      dependency.ProjectID,
			FileName:       dependency.FileName,
			DependencyType: dependency.DependencyType,
		})
	}

	return domain.ModrinthVersion{
		ID:            version.ID,
		ProjectID:     version.ProjectID,
		Name:          version.Name,
		VersionNumber: version.VersionNumber,
		VersionType:   version.VersionType,
		DatePublished: version.DatePublished,
		Changelog:     version.Changelog,
		GameVersions:  version.GameVersions,
		Loaders:       version.Loaders,
		Dependencies:  dependencies,
		File:          file,
	}, true
}

func pickModrinthFile(files []struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	FileName string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	FileType string            `json:"file_type"`
}) (domain.ModrinthVersionFile, bool) {
	for _, file := range files {
		if file.Primary && isModrinthJar(file) {
			return modrinthFile(file), true
		}
	}
	for _, file := range files {
		if isModrinthJar(file) {
			return modrinthFile(file), true
		}
	}
	return domain.ModrinthVersionFile{}, false
}

func isModrinthJar(file struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	FileName string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	FileType string            `json:"file_type"`
}) bool {
	fileName := strings.ToLower(file.FileName)
	if strings.TrimSpace(file.URL) == "" || !strings.HasSuffix(fileName, ".jar") {
		return false
	}
	return !strings.Contains(fileName, "-sources") && !strings.Contains(fileName, "-javadoc") && !strings.Contains(fileName, "-dev")
}

func modrinthFile(file struct {
	Hashes   map[string]string `json:"hashes"`
	URL      string            `json:"url"`
	FileName string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	FileType string            `json:"file_type"`
}) domain.ModrinthVersionFile {
	return domain.ModrinthVersionFile{
		URL:      file.URL,
		FileName: file.FileName,
		Size:     file.Size,
		Primary:  file.Primary,
		SHA1:     strings.ToLower(strings.TrimSpace(file.Hashes["sha1"])),
	}
}

func pickDisplayVersion(versions []string, minecraftVersion string, latestVersion string) string {
	for _, version := range versions {
		if version == minecraftVersion {
			return version
		}
	}
	if latestVersion != "" {
		return latestVersion
	}
	if len(versions) > 0 {
		return versions[0]
	}
	return ""
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
