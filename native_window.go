package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const (
	nativeWindowCommand   = "--power-mine-native-window"
	nativeWindowURLEnv    = "POWER_MINE_NATIVE_WINDOW_URL"
	nativeWindowTitleEnv  = "POWER_MINE_NATIVE_WINDOW_TITLE"
	nativeWindowWidthEnv  = "POWER_MINE_NATIVE_WINDOW_WIDTH"
	nativeWindowHeightEnv = "POWER_MINE_NATIVE_WINDOW_HEIGHT"
)

type nativeWindowConfig struct {
	target *url.URL
	title  string
	width  int
	height int
}

func runNativeWindow() int {
	config, err := nativeWindowConfigFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

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
		target: target,
		title:  title,
		width:  nativeWindowEnvInt(nativeWindowWidthEnv, 1000),
		height: nativeWindowEnvInt(nativeWindowHeightEnv, 720),
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

	a.nativeWindowMu.Lock()
	defer a.nativeWindowMu.Unlock()
	if a.nativeWindows == nil {
		a.nativeWindows = make(map[string]*exec.Cmd)
	}
	if running := a.nativeWindows[key]; nativeWindowProcessRunning(running) {
		return nil
	}
	delete(a.nativeWindows, key)

	cmd := exec.Command(executable, nativeWindowCommand)
	cmd.Env = append(os.Environ(),
		nativeWindowURLEnv+"="+targetURL,
		nativeWindowTitleEnv+"="+title,
		nativeWindowWidthEnv+"="+strconv.Itoa(width),
		nativeWindowHeightEnv+"="+strconv.Itoa(height),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("open native window: %w", err)
	}
	a.nativeWindows[key] = cmd
	go a.waitNativeWindow(key, cmd)
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

func nativeWindowProcessRunning(cmd *exec.Cmd) bool {
	return cmd != nil && cmd.Process != nil && cmd.ProcessState == nil
}

func (a *App) waitNativeWindow(key string, cmd *exec.Cmd) {
	_ = cmd.Wait()
	a.nativeWindowMu.Lock()
	defer a.nativeWindowMu.Unlock()
	if a.nativeWindows[key] == cmd {
		delete(a.nativeWindows, key)
	}
}

func (a *App) stopNativeWindowProcesses() {
	a.nativeWindowMu.Lock()
	commands := make([]*exec.Cmd, 0, len(a.nativeWindows))
	for key, cmd := range a.nativeWindows {
		if nativeWindowProcessRunning(cmd) {
			commands = append(commands, cmd)
		}
		delete(a.nativeWindows, key)
	}
	a.nativeWindowMu.Unlock()

	for _, cmd := range commands {
		_ = cmd.Process.Kill()
	}
}
