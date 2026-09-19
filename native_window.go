package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"power-mine/internal/platform"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	nativeWindowCommand       = "--power-mine-native-window"
	nativeWindowURLEnv        = "POWER_MINE_NATIVE_WINDOW_URL"
	nativeWindowTitleEnv      = "POWER_MINE_NATIVE_WINDOW_TITLE"
	nativeWindowWidthEnv      = "POWER_MINE_NATIVE_WINDOW_WIDTH"
	nativeWindowHeightEnv     = "POWER_MINE_NATIVE_WINDOW_HEIGHT"
	nativeWindowFocusAddrEnv  = "POWER_MINE_NATIVE_WINDOW_FOCUS_ADDR"
	nativeWindowFocusTokenEnv = "POWER_MINE_NATIVE_WINDOW_FOCUS_TOKEN"
	nativeWindowPlacementEnv  = "POWER_MINE_NATIVE_WINDOW_PLACEMENT_FILE"
)

const (
	nativeWindowPlacementApplyDelay       = 300 * time.Millisecond
	nativeWindowPlacementAutosaveInterval = 750 * time.Millisecond
)

type nativeWindowConfig struct {
	target     *url.URL
	title      string
	width      int
	height     int
	focusAddr  string
	focusToken string
	placement  string
}

type nativeWindowProcess struct {
	cmd      *exec.Cmd
	focusURL string
}

func runNativeWindow() int {
	config, err := nativeWindowConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	focusControl := newNativeWindowFocusControl(config.focusAddr, config.focusToken)
	if err := focusControl.start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	windowRuntime := &nativeWindowRuntime{
		focus:     focusControl,
		placement: nativeWindowPlacementStore{path: config.placement, defaultWidth: config.width, defaultHeight: config.height},
		watchdog:  newWindowCloseExitWatchdog(2*time.Second, os.Exit),
	}
	focusControl.setOnFocus(windowRuntime.focusWindow)
	defer focusControl.shutdown(context.Background())

	err = wails.Run(&options.App{
		Title:     config.title,
		Width:     config.width,
		Height:    config.height,
		MinWidth:  640,
		MinHeight: 420,
		AssetServer: &assetserver.Options{
			Handler: newNativeWindowProxyHandler(config.target),
		},
		BackgroundColour: &options.RGBA{R: 5, G: 5, B: 5, A: 1},
		OnStartup:        windowRuntime.startup,
		OnDomReady:       windowRuntime.domReady,
		OnShutdown:       windowRuntime.shutdown,
		OnBeforeClose:    windowRuntime.beforeClose,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err.Error())
		return 1
	}
	return 0
}

func nativeWindowConfigFromEnv() (nativeWindowConfig, error) {
	rawURL := strings.TrimSpace(os.Getenv(nativeWindowURLEnv))
	if rawURL == "" {
		return nativeWindowConfig{}, fmt.Errorf("native window URL is not set")
	}
	target, err := url.Parse(rawURL)
	if err != nil {
		return nativeWindowConfig{}, fmt.Errorf("parse native window URL: %w", err)
	}
	if err := validateNativeWindowTarget(target); err != nil {
		return nativeWindowConfig{}, err
	}

	title := strings.TrimSpace(os.Getenv(nativeWindowTitleEnv))
	if title == "" {
		title = "Power Mine"
	}
	return nativeWindowConfig{
		target:     target,
		title:      title,
		width:      nativeWindowEnvInt(nativeWindowWidthEnv, 1000),
		height:     nativeWindowEnvInt(nativeWindowHeightEnv, 720),
		focusAddr:  strings.TrimSpace(os.Getenv(nativeWindowFocusAddrEnv)),
		focusToken: strings.TrimSpace(os.Getenv(nativeWindowFocusTokenEnv)),
		placement:  strings.TrimSpace(os.Getenv(nativeWindowPlacementEnv)),
	}, nil
}

func nativeWindowEnvInt(name string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 320 {
		return fallback
	}
	return parsed
}

func validateNativeWindowTarget(target *url.URL) error {
	if target == nil {
		return fmt.Errorf("native window target is empty")
	}
	if target.Scheme != "http" {
		return fmt.Errorf("native window target must use local http")
	}
	host := target.Hostname()
	if host == "" {
		return fmt.Errorf("native window target host is empty")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("native window target must be loopback-only")
	}
	return nil
}

func newNativeWindowProxyHandler(target *url.URL) http.Handler {
	proxyTarget := &url.URL{Scheme: target.Scheme, Host: target.Host}
	proxy := httputil.NewSingleHostReverseProxy(proxyTarget)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalPath := req.URL.Path
		originalQuery := req.URL.RawQuery
		director(req)
		req.Host = proxyTarget.Host
		if originalPath == "/" && originalQuery == "" {
			req.URL.Path = target.Path
			req.URL.RawPath = target.RawPath
			req.URL.RawQuery = target.RawQuery
		}
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		http.Error(w, "native window proxy: "+err.Error(), http.StatusBadGateway)
	}
	return proxy
}

type nativeWindowFocusControl struct {
	addr    string
	token   string
	server  *http.Server
	mu      sync.RWMutex
	ctx     context.Context
	onFocus func(context.Context)
}

func newNativeWindowFocusControl(addr string, token string) *nativeWindowFocusControl {
	return &nativeWindowFocusControl{addr: strings.TrimSpace(addr), token: strings.TrimSpace(token)}
}

func (c *nativeWindowFocusControl) start() error {
	if c.addr == "" && c.token == "" {
		return nil
	}
	if c.addr == "" || c.token == "" {
		return fmt.Errorf("native window focus control requires both address and token")
	}
	if err := validateNativeWindowFocusAddr(c.addr); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/focus", c.handleFocus)
	listener, err := net.Listen("tcp", c.addr)
	if err != nil {
		return fmt.Errorf("listen native window focus server: %w", err)
	}
	c.server = &http.Server{Addr: c.addr, Handler: mux}
	go func() {
		if err := c.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "native window focus server stopped: %v\n", err)
		}
	}()
	return nil
}

func (c *nativeWindowFocusControl) setContext(ctx context.Context) {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
}

func (c *nativeWindowFocusControl) setOnFocus(fn func(context.Context)) {
	c.mu.Lock()
	c.onFocus = fn
	c.mu.Unlock()
}

func (c *nativeWindowFocusControl) shutdown(ctx context.Context) {
	if c.server == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = c.server.Shutdown(shutdownCtx)
}

func (c *nativeWindowFocusControl) handleFocus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("token") != c.token {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.mu.RLock()
	ctx := c.ctx
	onFocus := c.onFocus
	c.mu.RUnlock()
	if ctx == nil {
		http.Error(w, "native window is not ready", http.StatusServiceUnavailable)
		return
	}
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	if onFocus != nil {
		onFocus(ctx)
	}
	w.WriteHeader(http.StatusNoContent)
}

type nativeWindowRuntime struct {
	mu             sync.Mutex
	focus          *nativeWindowFocusControl
	placement      nativeWindowPlacementStore
	watchdog       *windowCloseExitWatchdog
	autosaveCancel context.CancelFunc
	stopping       bool
}

func (r *nativeWindowRuntime) startup(ctx context.Context) {
	if r.focus != nil {
		r.focus.setContext(ctx)
	}
}

func (r *nativeWindowRuntime) domReady(ctx context.Context) {
	r.focusWindow(ctx)
	r.startAutosaveAfter(ctx, 2*nativeWindowPlacementApplyDelay)
}

func (r *nativeWindowRuntime) shutdown(ctx context.Context) {
	r.markStopping()
	r.stopAutosave()
	if r.focus != nil {
		r.focus.shutdown(ctx)
	}
}

func (r *nativeWindowRuntime) beforeClose(ctx context.Context) bool {
	r.markStopping()
	r.stopAutosave()
	if r.watchdog == nil {
		return false
	}
	return r.watchdog.beforeClose(ctx)
}

func (r *nativeWindowRuntime) focusWindow(ctx context.Context) {
	r.placement.apply(ctx)
	r.placement.applyDelayed(ctx, nativeWindowPlacementApplyDelay)
}

func (r *nativeWindowRuntime) startAutosaveAfter(ctx context.Context, delay time.Duration) {
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		if r.isStopping() {
			return
		}
		r.setAutosaveCancel(r.placement.startAutosave(ctx, nativeWindowPlacementAutosaveInterval))
	}()
}

func (r *nativeWindowRuntime) markStopping() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
}

func (r *nativeWindowRuntime) isStopping() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopping
}

func (r *nativeWindowRuntime) setAutosaveCancel(cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		if cancel != nil {
			cancel()
		}
		return
	}
	if r.autosaveCancel != nil {
		r.autosaveCancel()
	}
	r.autosaveCancel = cancel
}

func (r *nativeWindowRuntime) stopAutosave() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.autosaveCancel == nil {
		return
	}
	r.autosaveCancel()
	r.autosaveCancel = nil
}

type nativeWindowPlacementStore struct {
	path          string
	defaultWidth  int
	defaultHeight int
}

type nativeWindowPlacement struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (s nativeWindowPlacementStore) apply(ctx context.Context) {
	placement, ok := readNativeWindowPlacement(s.path)
	if !ok {
		return
	}
	wailsruntime.WindowSetSize(ctx, placement.Width, placement.Height)
	wailsruntime.WindowSetPosition(ctx, placement.X, placement.Y)
}

func (s nativeWindowPlacementStore) applyDelayed(ctx context.Context, delay time.Duration) {
	if delay <= 0 {
		s.apply(ctx)
		return
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		<-timer.C
		s.apply(ctx)
	}()
}

func (s nativeWindowPlacementStore) startAutosave(ctx context.Context, interval time.Duration) context.CancelFunc {
	if strings.TrimSpace(s.path) == "" || ctx == nil || interval <= 0 {
		return nil
	}
	autosaveCtx, cancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var last nativeWindowPlacement
		var hasLast bool
		for {
			select {
			case <-autosaveCtx.Done():
				return
			case <-ticker.C:
				placement, ok := s.current(ctx)
				if !ok || hasLast && placement == last {
					continue
				}
				if err := s.write(placement); err != nil {
					continue
				}
				last = placement
				hasLast = true
			}
		}
	}()
	return cancel
}

func (s nativeWindowPlacementStore) current(ctx context.Context) (nativeWindowPlacement, bool) {
	if strings.TrimSpace(s.path) == "" || ctx == nil {
		return nativeWindowPlacement{}, false
	}
	if !wailsruntime.WindowIsNormal(ctx) {
		return nativeWindowPlacement{}, false
	}
	width, height := wailsruntime.WindowGetSize(ctx)
	x, y := wailsruntime.WindowGetPosition(ctx)
	placement := nativeWindowPlacement{X: x, Y: y, Width: width, Height: height}
	if !placement.valid() {
		return nativeWindowPlacement{}, false
	}
	return placement, true
}

func (s nativeWindowPlacementStore) write(placement nativeWindowPlacement) error {
	if existing, ok := readNativeWindowPlacement(s.path); ok && existing != placement && s.looksLikeClosingFallback(placement) {
		return nil
	}
	return writeNativeWindowPlacement(s.path, placement)
}

func (s nativeWindowPlacementStore) looksLikeClosingFallback(placement nativeWindowPlacement) bool {
	if s.defaultWidth <= 0 || s.defaultHeight <= 0 {
		return false
	}
	return placement.Width == s.defaultWidth && placement.Height == s.defaultHeight && placement.X >= 0 && placement.X <= 16 && placement.Y >= 0 && placement.Y <= 80
}

func readNativeWindowPlacement(path string) (nativeWindowPlacement, bool) {
	if strings.TrimSpace(path) == "" {
		return nativeWindowPlacement{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nativeWindowPlacement{}, false
	}
	var placement nativeWindowPlacement
	if err := json.Unmarshal(data, &placement); err != nil || !placement.valid() {
		return nativeWindowPlacement{}, false
	}
	return placement, true
}

func writeNativeWindowPlacement(path string, placement nativeWindowPlacement) error {
	if strings.TrimSpace(path) == "" || !placement.valid() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(placement, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (p nativeWindowPlacement) valid() bool {
	return p.Width >= 640 && p.Height >= 420 && p.Width <= 10000 && p.Height <= 10000
}

func (a *App) openNativeWindow(key string, placementKey string, targetURL string, title string, width int, height int) error {
	target, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("parse native window target: %w", err)
	}
	if err := validateNativeWindowTarget(target); err != nil {
		return err
	}
	executable, err := launcherExecutablePath()
	if err != nil {
		return err
	}
	focusAddr, err := reserveNativeWindowFocusAddr()
	if err != nil {
		return err
	}
	focusToken, err := randomToken()
	if err != nil {
		return err
	}
	placementPath, err := nativeWindowPlacementPath(placementKey)
	if err != nil {
		return err
	}
	focusURL := "http://" + focusAddr + "/focus?token=" + url.QueryEscape(focusToken)

	a.nativeWindowMu.Lock()
	defer a.nativeWindowMu.Unlock()
	if a.nativeWindows == nil {
		a.nativeWindows = make(map[string]*nativeWindowProcess)
	}
	if running := a.nativeWindows[key]; nativeWindowProcessRunning(running) {
		if err := focusNativeWindow(running); err != nil {
			fmt.Fprintf(os.Stderr, "focus native window: %v\n", err)
		}
		return nil
	}
	delete(a.nativeWindows, key)

	cmd := exec.Command(executable, nativeWindowCommand)
	cmd.Env = append(os.Environ(),
		nativeWindowURLEnv+"="+targetURL,
		nativeWindowTitleEnv+"="+title,
		nativeWindowWidthEnv+"="+strconv.Itoa(width),
		nativeWindowHeightEnv+"="+strconv.Itoa(height),
		nativeWindowFocusAddrEnv+"="+focusAddr,
		nativeWindowFocusTokenEnv+"="+focusToken,
		nativeWindowPlacementEnv+"="+placementPath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open native window: %w", err)
	}
	process := &nativeWindowProcess{cmd: cmd, focusURL: focusURL}
	a.nativeWindows[key] = process
	go a.waitNativeWindow(key, process)
	return nil
}

func nativeWindowPlacementPath(placementKey string) (string, error) {
	dataDir, err := platform.AppDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "native-windows", nativeWindowPlacementFilename(placementKey)+".json"), nil
}

func nativeWindowPlacementFilename(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	var builder strings.Builder
	lastDash := false
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ':' || r == ' ':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	filename := strings.Trim(builder.String(), "-")
	if filename == "" {
		return "native-window"
	}
	return filename
}

func reserveNativeWindowFocusAddr() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("reserve native window focus address: %w", err)
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

func validateNativeWindowFocusAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse native window focus address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("native window focus address must be loopback-only")
	}
	return nil
}

func focusNativeWindow(process *nativeWindowProcess) error {
	if process == nil || strings.TrimSpace(process.focusURL) == "" {
		return fmt.Errorf("native window focus endpoint is unavailable")
	}
	client := http.Client{Timeout: 750 * time.Millisecond}
	response, err := client.Get(process.focusURL)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("focus endpoint returned %s", response.Status)
	}
	return nil
}

func launcherExecutablePath() (string, error) {
	if appImage := strings.TrimSpace(os.Getenv("APPIMAGE")); appImage != "" {
		if _, err := os.Stat(appImage); err == nil {
			return appImage, nil
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve launcher executable: %w", err)
	}
	return executable, nil
}

func nativeWindowProcessRunning(process *nativeWindowProcess) bool {
	return process != nil && process.cmd != nil && process.cmd.Process != nil && process.cmd.ProcessState == nil
}

func (a *App) waitNativeWindow(key string, process *nativeWindowProcess) {
	_ = process.cmd.Wait()
	a.nativeWindowMu.Lock()
	defer a.nativeWindowMu.Unlock()
	if a.nativeWindows[key] == process {
		delete(a.nativeWindows, key)
	}
}

func (a *App) stopNativeWindowProcesses() {
	a.nativeWindowMu.Lock()
	commands := make([]*exec.Cmd, 0, len(a.nativeWindows))
	for key, process := range a.nativeWindows {
		if nativeWindowProcessRunning(process) {
			commands = append(commands, process.cmd)
		}
		delete(a.nativeWindows, key)
	}
	a.nativeWindowMu.Unlock()

	for _, cmd := range commands {
		_ = cmd.Process.Kill()
	}
}
