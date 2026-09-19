package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
