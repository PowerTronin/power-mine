package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

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
)

type nativeWindowConfig struct {
	target     *url.URL
	title      string
	width      int
	height     int
	focusAddr  string
	focusToken string
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
		OnStartup:        focusControl.setContext,
		OnShutdown:       focusControl.shutdown,
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
	addr   string
	token  string
	server *http.Server
	mu     sync.RWMutex
	ctx    context.Context
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
	c.mu.RUnlock()
	if ctx == nil {
		http.Error(w, "native window is not ready", http.StatusServiceUnavailable)
		return
	}
	wailsruntime.WindowShow(ctx)
	wailsruntime.WindowUnminimise(ctx)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) openNativeWindow(key string, targetURL string, title string, width int, height int) error {
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
