package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"power-mine/internal/domain"
)

func TestSearchModrinthModsUsesProfileFacets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/search" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("query") != "jei" {
			t.Fatalf("unexpected query %q", request.URL.Query().Get("query"))
		}

		var facets [][]string
		if err := json.Unmarshal([]byte(request.URL.Query().Get("facets")), &facets); err != nil {
			t.Fatal(err)
		}
		wantFacets := map[string]bool{
			"project_type:mod":  false,
			"versions:1.20.1":   false,
			"categories:fabric": false,
		}
		for _, group := range facets {
			for _, facet := range group {
				if _, ok := wantFacets[facet]; ok {
					wantFacets[facet] = true
				}
			}
		}
		for facet, found := range wantFacets {
			if !found {
				t.Fatalf("missing facet %s in %#v", facet, facets)
			}
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"hits": [{
				"project_id": "project-1",
				"slug": "jei",
				"author": "author",
				"title": "Just Enough Items",
				"description": "Item browser",
				"categories": ["fabric"],
				"display_categories": ["Fabric"],
				"versions": ["1.20.1"],
				"downloads": 42,
				"icon_url": "https://example.test/icon.png",
				"latest_version": "1.20.1",
				"client_side": "required",
				"server_side": "unsupported"
			}],
			"total_hits": 1
		}`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	result, err := service.SearchModrinthMods(context.Background(), domain.Profile{
		ID:               "profile-1",
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric},
	}, "jei", 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalHits != 1 || len(result.Hits) != 1 {
		t.Fatalf("unexpected result %#v", result)
	}
	if result.Hits[0].Title != "Just Enough Items" || result.Hits[0].DisplayVersion != "1.20.1" {
		t.Fatalf("unexpected hit %#v", result.Hits[0])
	}
}

func TestLatestModrinthVersionSelectsPrimaryJar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/project/project-1/version" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("loaders") != `["fabric"]` {
			t.Fatalf("unexpected loaders %q", request.URL.Query().Get("loaders"))
		}
		if request.URL.Query().Get("game_versions") != `["1.20.1"]` {
			t.Fatalf("unexpected game versions %q", request.URL.Query().Get("game_versions"))
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{
			"id": "version-1",
			"project_id": "project-1",
			"name": "Release",
			"version_number": "1.0.0",
			"version_type": "release",
			"game_versions": ["1.20.1"],
			"loaders": ["fabric"],
			"dependencies": [{"project_id":"dep-1","dependency_type":"required"}],
			"files": [
				{"url":"https://example.test/sources.jar","filename":"mod-sources.jar","primary":false,"size":10,"file_type":"source"},
				{"url":"https://example.test/mod.jar","filename":"mod.jar","primary":true,"size":20}
			]
		}]`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	version, err := service.LatestModrinthVersion(context.Background(), domain.Profile{
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric},
	}, "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if version.File.FileName != "mod.jar" || version.File.URL != "https://example.test/mod.jar" {
		t.Fatalf("unexpected file %#v", version.File)
	}
	if len(version.Dependencies) != 1 || version.Dependencies[0].ProjectID != "dep-1" {
		t.Fatalf("unexpected dependencies %#v", version.Dependencies)
	}
}

func TestLatestModrinthVersionFromHashUsesUpdateEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.URL.Path != "/v2/version_file/abc123/update" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("algorithm") != "sha1" {
			t.Fatalf("unexpected algorithm %q", request.URL.Query().Get("algorithm"))
		}
		var body struct {
			Loaders      []string `json:"loaders"`
			GameVersions []string `json:"game_versions"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Loaders) != 1 || body.Loaders[0] != "fabric" {
			t.Fatalf("unexpected loaders %#v", body.Loaders)
		}
		if len(body.GameVersions) != 1 || body.GameVersions[0] != "1.20.1" {
			t.Fatalf("unexpected game versions %#v", body.GameVersions)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": "version-2",
			"project_id": "project-1",
			"name": "Release",
			"version_number": "2.0.0",
			"version_type": "release",
			"game_versions": ["1.20.1"],
			"loaders": ["fabric"],
			"dependencies": [],
			"files": [
				{"hashes":{"sha1":"def456"},"url":"https://example.test/mod.jar","filename":"mod.jar","primary":true,"size":20}
			]
		}`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	version, err := service.LatestModrinthVersionFromHash(context.Background(), domain.Profile{
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric},
	}, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if version.ID != "version-2" || version.ProjectID != "project-1" || version.File.SHA1 != "def456" {
		t.Fatalf("unexpected version %#v", version)
	}
}

func TestModrinthVersionFromHashUsesVersionFileEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if request.URL.Path != "/v2/version_file/abc123" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		if request.URL.Query().Get("algorithm") != "sha1" {
			t.Fatalf("unexpected algorithm %q", request.URL.Query().Get("algorithm"))
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": "version-1",
			"project_id": "project-1",
			"name": "Release",
			"version_number": "1.0.0",
			"version_type": "release",
			"game_versions": ["1.20.1"],
			"loaders": ["fabric"],
			"dependencies": [],
			"files": [
				{"hashes":{"sha1":"abc123"},"url":"https://example.test/mod.jar","filename":"mod.jar","primary":true,"size":20}
			]
		}`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	version, err := service.ModrinthVersionFromHash(context.Background(), "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if version.ID != "version-1" || version.VersionNumber != "1.0.0" || version.File.SHA1 != "abc123" {
		t.Fatalf("unexpected version %#v", version)
	}
}

func TestModrinthProjectVersionSelectsCompatibleVersionByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/project/project-1/version/version-1" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": "version-1",
			"project_id": "project-1",
			"name": "Release",
			"version_number": "1.0.0",
			"version_type": "release",
			"game_versions": ["1.20.1"],
			"loaders": ["fabric"],
			"dependencies": [],
			"files": [
				{"url":"https://example.test/mod.jar","filename":"mod.jar","primary":true,"size":20}
			]
		}`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	profile := domain.Profile{
		MinecraftVersion: "1.20.1",
		Loader:           domain.LoaderConfig{Type: domain.LoaderFabric},
	}
	version, err := service.ModrinthProjectVersion(context.Background(), profile, "project-1", "version-1")
	if err != nil {
		t.Fatal(err)
	}
	if version.ID != "version-1" || version.File.FileName != "mod.jar" {
		t.Fatalf("unexpected version %#v", version)
	}
}

func TestModrinthProjectReturnsDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v2/project/project-1" {
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id": "project-1",
			"slug": "jei",
			"title": "Just Enough Items",
			"description": "Item browser",
			"body": "# Details",
			"categories": ["fabric"],
			"additional_categories": ["utility"],
			"game_versions": ["1.20.1"],
			"loaders": ["fabric"],
			"client_side": "required",
			"server_side": "unsupported",
			"downloads": 42,
			"followers": 7,
			"icon_url": "https://example.test/icon.png",
			"source_url": "https://example.test/source",
			"issues_url": "https://example.test/issues",
			"wiki_url": "https://example.test/wiki",
			"discord_url": "https://example.test/discord",
			"license": {"id":"MIT","name":"MIT License"}
		}`))
	}))
	defer server.Close()

	service := NewService()
	service.client = server.Client()
	service.modrinthAPIBaseURL = server.URL + "/v2"

	project, err := service.ModrinthProject(context.Background(), "project-1")
	if err != nil {
		t.Fatal(err)
	}
	if project.Title != "Just Enough Items" || project.Body != "# Details" {
		t.Fatalf("unexpected project %#v", project)
	}
	if project.LicenseName != "MIT License" || project.Followers != 7 {
		t.Fatalf("unexpected metadata %#v", project)
	}
	if len(project.Categories) != 2 || project.Categories[1] != "utility" {
		t.Fatalf("unexpected categories %#v", project.Categories)
	}
}

func TestVersionCatalogCachesNetworkResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/mc/version_manifest_v2.json":
			_, _ = writer.Write([]byte(`{
				"latest": {"release":"1.21.5","snapshot":"25w01a"},
				"versions": [
					{"id":"1.21.5","type":"release"},
					{"id":"25w01a","type":"snapshot"}
				]
			}`))
		case "/fabric/versions/loader":
			_, _ = writer.Write([]byte(`[
				{"version":"0.16.14","stable":true},
				{"version":"0.16.13","stable":true}
			]`))
		case "/quilt/versions/loader":
			_, _ = writer.Write([]byte(`[
				{"version":"0.29.1"},
				{"version":"0.29.0"}
			]`))
		case "/forge/maven-metadata.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<metadata>
					<versioning>
						<latest>1.20.1-47.4.20</latest>
						<release>1.20.1-47.4.20</release>
						<versions>
							<version>1.20.1-47.4.9</version>
							<version>1.20.1-47.4.20</version>
						</versions>
					</versioning>
				</metadata>`))
		case "/neoforge/maven-metadata.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<metadata>
					<versioning>
						<latest>21.1.207</latest>
						<release>21.1.207</release>
						<versions>
							<version>21.1.206</version>
							<version>21.1.207</version>
						</versions>
					</versioning>
				</metadata>`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer server.Close()

	service := NewService(t.TempDir())
	service.client = server.Client()
	service.versionManifestURL = server.URL + "/mc/version_manifest_v2.json"
	service.fabricLoadersURL = server.URL + "/fabric/versions/loader"
	service.quiltLoadersURL = server.URL + "/quilt/versions/loader"
	service.forgeLoadersURL = server.URL + "/forge/maven-metadata.xml"
	service.neoForgeLoadersURL = server.URL + "/neoforge/maven-metadata.xml"

	catalog, err := service.VersionCatalog(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MinecraftSource != "network" || catalog.FabricLoaderSource != "network" || catalog.QuiltLoaderSource != "network" || catalog.ForgeLoaderSource != "network" || catalog.NeoForgeLoaderSource != "network" {
		t.Fatalf("unexpected sources %#v", catalog)
	}
	if len(catalog.MinecraftVersions) != 2 || catalog.MinecraftVersions[0].ID != "1.21.5" {
		t.Fatalf("unexpected Minecraft versions %#v", catalog.MinecraftVersions)
	}
	if len(catalog.FabricLoaderVersions) != 2 || catalog.FabricLoaderVersions[0].ID != "0.16.14" {
		t.Fatalf("unexpected Fabric versions %#v", catalog.FabricLoaderVersions)
	}
	if len(catalog.QuiltLoaderVersions) != 2 || catalog.QuiltLoaderVersions[0].ID != "0.29.1" {
		t.Fatalf("unexpected Quilt versions %#v", catalog.QuiltLoaderVersions)
	}
	if len(catalog.ForgeLoaderVersions) != 2 || catalog.ForgeLoaderVersions[0].ID != "1.20.1-47.4.20" {
		t.Fatalf("unexpected Forge versions %#v", catalog.ForgeLoaderVersions)
	}
	if len(catalog.NeoForgeLoaderVersions) != 2 || catalog.NeoForgeLoaderVersions[0].ID != "21.1.207" {
		t.Fatalf("unexpected NeoForge versions %#v", catalog.NeoForgeLoaderVersions)
	}

	cached, err := service.CachedVersionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if cached.MinecraftSource != "cache" || cached.FabricLoaderSource != "cache" || cached.QuiltLoaderSource != "cache" || cached.ForgeLoaderSource != "cache" || cached.NeoForgeLoaderSource != "cache" {
		t.Fatalf("unexpected cached sources %#v", cached)
	}
}

func TestVersionCatalogFallsBackToCacheOnNetworkFailure(t *testing.T) {
	goodServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/mc/version_manifest_v2.json":
			_, _ = writer.Write([]byte(`{
				"latest": {"release":"1.20.1","snapshot":"25w01a"},
				"versions": [{"id":"1.20.1","type":"release"}]
			}`))
		case "/fabric/versions/loader":
			_, _ = writer.Write([]byte(`[{"version":"0.16.10","stable":true}]`))
		case "/quilt/versions/loader":
			_, _ = writer.Write([]byte(`[{"version":"0.29.1"}]`))
		case "/forge/maven-metadata.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<metadata>
					<versioning>
						<versions><version>1.20.1-47.4.20</version></versions>
					</versioning>
				</metadata>`))
		case "/neoforge/maven-metadata.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<metadata>
					<versioning>
						<versions><version>21.1.207</version></versions>
					</versioning>
				</metadata>`))
		default:
			t.Fatalf("unexpected path %s", request.URL.Path)
		}
	}))
	defer goodServer.Close()

	service := NewService(t.TempDir())
	service.client = goodServer.Client()
	service.versionManifestURL = goodServer.URL + "/mc/version_manifest_v2.json"
	service.fabricLoadersURL = goodServer.URL + "/fabric/versions/loader"
	service.quiltLoadersURL = goodServer.URL + "/quilt/versions/loader"
	service.forgeLoadersURL = goodServer.URL + "/forge/maven-metadata.xml"
	service.neoForgeLoadersURL = goodServer.URL + "/neoforge/maven-metadata.xml"
	if _, err := service.VersionCatalog(context.Background(), 0, 0); err != nil {
		t.Fatal(err)
	}

	badServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "down", http.StatusInternalServerError)
	}))
	defer badServer.Close()
	service.client = badServer.Client()
	service.versionManifestURL = badServer.URL + "/mc/version_manifest_v2.json"
	service.fabricLoadersURL = badServer.URL + "/fabric/versions/loader"
	service.quiltLoadersURL = badServer.URL + "/quilt/versions/loader"
	service.forgeLoadersURL = badServer.URL + "/forge/maven-metadata.xml"
	service.neoForgeLoadersURL = badServer.URL + "/neoforge/maven-metadata.xml"

	catalog, err := service.VersionCatalog(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.MinecraftSource != "cache" || catalog.FabricLoaderSource != "cache" || catalog.QuiltLoaderSource != "cache" || catalog.ForgeLoaderSource != "cache" || catalog.NeoForgeLoaderSource != "cache" {
		t.Fatalf("expected cache fallback, got %#v", catalog)
	}
	if len(catalog.Warnings) != 5 {
		t.Fatalf("expected warnings, got %#v", catalog.Warnings)
	}
}

func TestCachedVersionCatalogUsesFabricFallbackWhenEmpty(t *testing.T) {
	service := NewService(t.TempDir())
	catalog, err := service.CachedVersionCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.FabricLoaderSource != "fallback" {
		t.Fatalf("expected fallback source, got %#v", catalog)
	}
	if len(catalog.FabricLoaderVersions) != 1 || catalog.FabricLoaderVersions[0].ID != "latest" {
		t.Fatalf("unexpected fallback versions %#v", catalog.FabricLoaderVersions)
	}
	if catalog.QuiltLoaderSource != "fallback" {
		t.Fatalf("expected Quilt fallback source, got %#v", catalog)
	}
	if len(catalog.QuiltLoaderVersions) != 1 || catalog.QuiltLoaderVersions[0].ID != "latest" {
		t.Fatalf("unexpected Quilt fallback versions %#v", catalog.QuiltLoaderVersions)
	}
	if catalog.ForgeLoaderSource != "fallback" {
		t.Fatalf("expected Forge fallback source, got %#v", catalog)
	}
	if len(catalog.ForgeLoaderVersions) != 1 || catalog.ForgeLoaderVersions[0].ID != "latest" {
		t.Fatalf("unexpected Forge fallback versions %#v", catalog.ForgeLoaderVersions)
	}
	if catalog.NeoForgeLoaderSource != "fallback" {
		t.Fatalf("expected NeoForge fallback source, got %#v", catalog)
	}
	if len(catalog.NeoForgeLoaderVersions) != 1 || catalog.NeoForgeLoaderVersions[0].ID != "latest" {
		t.Fatalf("unexpected NeoForge fallback versions %#v", catalog.NeoForgeLoaderVersions)
	}
}
