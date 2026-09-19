package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestNativeWindowProxyMapsRootToTargetPage(t *testing.T) {
	var mu sync.Mutex
	gotURL := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURL = r.URL.String()
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html><body>logs</body></html>")
	}))
	defer backend.Close()

	target, err := url.Parse(backend.URL + "/logs?token=abc")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	recorder := httptest.NewRecorder()
	newNativeWindowProxyHandler(target).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://wails.localhost/", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "logs") {
		t.Fatalf("unexpected body: %q", recorder.Body.String())
	}
	mu.Lock()
	defer mu.Unlock()
	if gotURL != "/logs?token=abc" {
		t.Fatalf("unexpected proxied URL: %s", gotURL)
	}
}

func TestNativeWindowProxyPreservesAPIPath(t *testing.T) {
	var mu sync.Mutex
	gotURL := ""
	gotMethod := ""
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotURL = r.URL.String()
		gotMethod = r.Method
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"sent"}`)
	}))
	defer backend.Close()

	target, err := url.Parse(backend.URL + "/server-terminal/server-1?token=abc")
	if err != nil {
		t.Fatalf("parse target: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://wails.localhost/api/server/server-1/command?token=abc", strings.NewReader(`{"command":"say hi"}`))
	newNativeWindowProxyHandler(target).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotURL != "/api/server/server-1/command?token=abc" {
		t.Fatalf("unexpected proxied URL: %s", gotURL)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected method: %s", gotMethod)
	}
}

func TestValidateNativeWindowTargetRequiresLoopbackHTTP(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{name: "loopback", rawURL: "http://127.0.0.1:1234/logs", wantErr: false},
		{name: "localhost", rawURL: "http://localhost:1234/logs", wantErr: false},
		{name: "https", rawURL: "https://127.0.0.1:1234/logs", wantErr: true},
		{name: "external", rawURL: "http://example.com/logs", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := url.Parse(tc.rawURL)
			if err != nil {
				t.Fatalf("parse target: %v", err)
			}
			err = validateNativeWindowTarget(target)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNativeWindowConfigReadsFocusEndpoint(t *testing.T) {
	t.Setenv(nativeWindowURLEnv, "http://127.0.0.1:1234/logs")
	t.Setenv(nativeWindowTitleEnv, "Logs")
	t.Setenv(nativeWindowWidthEnv, "900")
	t.Setenv(nativeWindowHeightEnv, "700")
	t.Setenv(nativeWindowFocusAddrEnv, "127.0.0.1:4321")
	t.Setenv(nativeWindowFocusTokenEnv, "token")
	t.Setenv(nativeWindowPlacementEnv, "/tmp/power-mine-placement.json")

	config, err := nativeWindowConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.target.String() != "http://127.0.0.1:1234/logs" {
		t.Fatalf("unexpected target: %s", config.target.String())
	}
	if config.title != "Logs" || config.width != 900 || config.height != 700 {
		t.Fatalf("unexpected window config: %#v", config)
	}
	if config.focusAddr != "127.0.0.1:4321" || config.focusToken != "token" {
		t.Fatalf("unexpected focus config: %#v", config)
	}
	if config.placement != "/tmp/power-mine-placement.json" {
		t.Fatalf("unexpected placement config: %#v", config)
	}
}

func TestValidateNativeWindowFocusAddrRequiresLoopbackIP(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr bool
	}{
		{name: "loopback", addr: "127.0.0.1:1234", wantErr: false},
		{name: "ipv6 loopback", addr: "[::1]:1234", wantErr: false},
		{name: "localhost", addr: "localhost:1234", wantErr: true},
		{name: "external", addr: "192.168.1.10:1234", wantErr: true},
		{name: "missing port", addr: "127.0.0.1", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNativeWindowFocusAddr(tc.addr)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestNativeWindowPlacementFilenameSanitizesKeys(t *testing.T) {
	cases := map[string]string{
		"logs":                "logs",
		"server-terminal:abc": "server-terminal-abc",
		"  Server Terminal  ": "server-terminal",
		"../../bad path.json": "bad-pathjson",
		"":                    "native-window",
	}
	for input, want := range cases {
		if got := nativeWindowPlacementFilename(input); got != want {
			t.Fatalf("nativeWindowPlacementFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNativeWindowPlacementRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placement.json")
	want := nativeWindowPlacement{X: 11, Y: 22, Width: 900, Height: 700}
	if err := writeNativeWindowPlacement(path, want); err != nil {
		t.Fatalf("write placement: %v", err)
	}
	got, ok := readNativeWindowPlacement(path)
	if !ok {
		t.Fatal("expected placement to be readable")
	}
	if got != want {
		t.Fatalf("unexpected placement: %#v", got)
	}
}

func TestNativeWindowPlacementRejectsTinySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placement.json")
	if err := writeNativeWindowPlacement(path, nativeWindowPlacement{Width: 100, Height: 100}); err != nil {
		t.Fatalf("write invalid placement: %v", err)
	}
	if _, ok := readNativeWindowPlacement(path); ok {
		t.Fatal("expected invalid placement to be ignored")
	}
}

func TestNativeWindowPlacementStoreSkipsClosingFallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placement.json")
	store := nativeWindowPlacementStore{path: path, defaultWidth: 1100, defaultHeight: 760}
	want := nativeWindowPlacement{X: 320, Y: 160, Width: 900, Height: 620}
	if err := writeNativeWindowPlacement(path, want); err != nil {
		t.Fatalf("write placement: %v", err)
	}

	if err := store.write(nativeWindowPlacement{X: 0, Y: 35, Width: 1100, Height: 760}); err != nil {
		t.Fatalf("write fallback placement: %v", err)
	}
	got, ok := readNativeWindowPlacement(path)
	if !ok {
		t.Fatal("expected placement to remain readable")
	}
	if got != want {
		t.Fatalf("unexpected placement: %#v", got)
	}
}

func TestNativeWindowPlacementStoreWritesRealDefaultPlacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "placement.json")
	store := nativeWindowPlacementStore{path: path, defaultWidth: 1100, defaultHeight: 760}
	want := nativeWindowPlacement{X: 420, Y: 222, Width: 1100, Height: 760}
	if err := store.write(want); err != nil {
		t.Fatalf("write placement: %v", err)
	}
	got, ok := readNativeWindowPlacement(path)
	if !ok {
		t.Fatal("expected placement to be readable")
	}
	if got != want {
		t.Fatalf("unexpected placement: %#v", got)
	}
}

func TestNativeWindowBeforeCloseStopsAutosave(t *testing.T) {
	canceled := make(chan struct{})
	runtime := &nativeWindowRuntime{
		autosaveCancel: func() { close(canceled) },
	}

	if prevent := runtime.beforeClose(context.Background()); prevent {
		t.Fatal("expected close to continue")
	}
	select {
	case <-canceled:
	default:
		t.Fatal("expected autosave to be stopped")
	}
	if !runtime.isStopping() {
		t.Fatal("expected runtime to be marked stopping")
	}
	if runtime.autosaveCancel != nil {
		t.Fatal("expected autosave cancel to be cleared")
	}
}
