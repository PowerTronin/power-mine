package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"power-mine/internal/domain"
	"power-mine/internal/javasvc"
	"power-mine/internal/minecraft"
	"power-mine/internal/platform"
)

type headlessResponse struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type headlessLaunchResult struct {
	ProfileID                  string `json:"profileId"`
	Status                     string `json:"status"`
	Message                    string `json:"message"`
	PID                        int    `json:"pid"`
	LogPath                    string `json:"logPath,omitempty"`
	StartedAt                  string `json:"startedAt"`
	PauseOnLostFocusDisabled   bool   `json:"pauseOnLostFocusDisabled"`
	PauseOnLostFocusWasChanged bool   `json:"pauseOnLostFocusWasChanged"`
}

func runHeadless(ctx context.Context, args []string) int {
	if len(args) == 0 {
		writeHeadless(false, "headless", nil, "missing command")
		return 2
	}

	command := args[0]
	dataDir, remaining, err := parseHeadlessDataDir(args[1:])
	if err != nil {
		writeHeadless(false, command, nil, err.Error())
		return 2
	}
	if dataDir == "" {
		dataDir, err = platform.AppDataDir()
		if err != nil {
			writeHeadless(false, command, nil, err.Error())
			return 1
		}
	}

	app := NewApp()
	app.headless = true
	app.initServices(ctx, dataDir)

	var result interface{}
	switch command {
	case "create-profile":
		result, err = runHeadlessCreateProfile(app, remaining)
	case "install-java":
		result, err = runHeadlessInstallJava(app, remaining)
	case "install-profile":
		result, err = runHeadlessInstall(app, remaining, false)
	case "repair-profile":
		result, err = runHeadlessInstall(app, remaining, true)
	case "launch-profile":
		result, err = runHeadlessLaunch(ctx, app, remaining)
	default:
		err = fmt.Errorf("unknown headless command %q", command)
	}
	if err != nil {
		writeHeadless(false, command, nil, err.Error())
		return 1
	}
	writeHeadless(true, command, result, "")
	return 0
}

func parseHeadlessDataDir(args []string) (string, []string, error) {
	dataDir := strings.TrimSpace(os.Getenv("POWER_MINE_DATA_DIR"))
	remaining := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--data-dir" {
			if index+1 >= len(args) {
				return "", nil, fmt.Errorf("--data-dir requires a value")
			}
			dataDir = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(arg, "--data-dir=") {
			dataDir = strings.TrimPrefix(arg, "--data-dir=")
			continue
		}
		remaining = append(remaining, arg)
	}
	return dataDir, remaining, nil
}

func runHeadlessCreateProfile(app *App, args []string) (domain.Profile, error) {
	flags := flag.NewFlagSet("create-profile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "profile name")
	minecraftVersion := flags.String("minecraft-version", "1.20.1", "minecraft version")
	loaderType := flags.String("loader", string(domain.LoaderFabric), "loader type")
	loaderVersion := flags.String("loader-version", "latest", "loader version")
	gameDir := flags.String("game-dir", "", "profile game directory")
	minMemory := flags.Int("min-memory", 1024, "minimum memory in MB")
	maxMemory := flags.Int("max-memory", 4096, "maximum memory in MB")
	install := flags.Bool("install", false, "install the profile after creating it")
	if err := flags.Parse(args); err != nil {
		return domain.Profile{}, err
	}
	if strings.TrimSpace(*name) == "" && flags.NArg() > 0 {
		*name = flags.Arg(0)
	}
	if strings.TrimSpace(*name) == "" {
		return domain.Profile{}, fmt.Errorf("profile name is required")
	}

	profile, err := app.CreateProfile(domain.ProfileInput{
		Name:             *name,
		MinecraftVersion: *minecraftVersion,
		Loader: domain.LoaderConfig{
			Type:    domain.LoaderType(*loaderType),
			Version: *loaderVersion,
		},
		GameDir: *gameDir,
		Memory: domain.MemorySettings{
			MinMB: *minMemory,
			MaxMB: *maxMemory,
		},
	})
	if err != nil {
		return domain.Profile{}, err
	}
	if *install {
		return app.installProfile(profile.ID, false)
	}
	return profile, nil
}

func runHeadlessInstallJava(app *App, args []string) (domain.JavaStatus, error) {
	flags := flag.NewFlagSet("install-java", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	version := flags.Int("version", 21, "Java major version")
	if err := flags.Parse(args); err != nil {
		return domain.JavaStatus{}, err
	}
	if flags.NArg() > 0 {
		parsed, err := strconv.Atoi(flags.Arg(0))
		if err != nil {
			return domain.JavaStatus{}, fmt.Errorf("invalid Java version %q", flags.Arg(0))
		}
		*version = parsed
	}
	if *version <= 0 {
		return domain.JavaStatus{}, fmt.Errorf("Java version must be positive")
	}
	return app.InstallJava(*version)
}

func runHeadlessInstall(app *App, args []string, repair bool) (domain.Profile, error) {
	flags := flag.NewFlagSet("install-profile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profileID := flags.String("profile-id", "", "profile id")
	if err := flags.Parse(args); err != nil {
		return domain.Profile{}, err
	}
	if *profileID == "" && flags.NArg() > 0 {
		*profileID = flags.Arg(0)
	}
	if strings.TrimSpace(*profileID) == "" {
		return domain.Profile{}, fmt.Errorf("profile id is required")
	}
	return app.installProfile(*profileID, repair)
}

func runHeadlessLaunch(ctx context.Context, app *App, args []string) (headlessLaunchResult, error) {
	flags := flag.NewFlagSet("launch-profile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profileID := flags.String("profile-id", "", "profile id")
	quickPlaySingleplayer := flags.String("quick-play-singleplayer", "", "singleplayer world name to open with Minecraft Quick Play")
	keepPauseOnLostFocus := flags.Bool("keep-pause-on-lost-focus", false, "do not force pauseOnLostFocus:false before launch")
	if err := flags.Parse(args); err != nil {
		return headlessLaunchResult{}, err
	}
	if *profileID == "" && flags.NArg() > 0 {
		*profileID = flags.Arg(0)
	}
	if strings.TrimSpace(*profileID) == "" {
		return headlessLaunchResult{}, fmt.Errorf("profile id is required")
	}
	if err := app.ensureReady(); err != nil {
		return headlessLaunchResult{}, err
	}

	profile, err := app.profileService.Get(*profileID)
	if err != nil {
		return headlessLaunchResult{}, err
	}
	if profile.Install.Status != "installed" {
		return headlessLaunchResult{}, fmt.Errorf("profile is not installed")
	}

	currentSettings, err := app.settingsService.Get()
	if err != nil {
		return headlessLaunchResult{}, err
	}
	javaPath, requiredJava, err := app.javaPathForProfile(profile, currentSettings.JavaPath)
	if err != nil {
		return headlessLaunchResult{}, err
	}
	javaStatus := app.javaService.Validate(ctx, javaPath)
	if !javaStatus.OK {
		return headlessLaunchResult{}, errors.New(javaStatus.Message)
	}
	if requiredJava > 0 && !javasvc.CompatibleMajor(javasvc.MajorVersion(javaStatus.Version), requiredJava) {
		return headlessLaunchResult{}, fmt.Errorf("minecraft %s requires Java %d", profile.MinecraftVersion, requiredJava)
	}

	pauseOnLostFocusChanged := false
	if !*keepPauseOnLostFocus {
		changed, err := minecraft.SetPauseOnLostFocus(profile.GameDir, false)
		if err != nil {
			return headlessLaunchResult{}, fmt.Errorf("disable pauseOnLostFocus: %w", err)
		}
		pauseOnLostFocusChanged = changed
	}

	commandSpec, err := app.minecraftService.BuildLaunchCommand(ctx, profile, minecraft.LaunchOptions{
		JavaPath:      javaPath,
		Memory:        profile.Memory,
		Account:       currentSettings.Account,
		ExtraGameArgs: quickPlayArgs(*quickPlaySingleplayer),
	})
	if err != nil {
		return headlessLaunchResult{}, err
	}

	logPath, logFile, err := headlessLaunchLog(profile)
	if err != nil {
		return headlessLaunchResult{}, err
	}
	defer logFile.Close()

	command := exec.Command(commandSpec.JavaPath, commandSpec.Args...)
	command.Dir = commandSpec.WorkDir
	command.Env = headlessLaunchEnv(os.Environ())
	command.Stdout = logFile
	command.Stderr = logFile
	command.Stdin = nil
	detachCommand(command)
	if err := command.Start(); err != nil {
		return headlessLaunchResult{}, err
	}

	return headlessLaunchResult{
		ProfileID:                  profile.ID,
		Status:                     "running",
		Message:                    "Minecraft process started",
		PID:                        command.Process.Pid,
		LogPath:                    logPath,
		StartedAt:                  time.Now().UTC().Format(time.RFC3339),
		PauseOnLostFocusDisabled:   !*keepPauseOnLostFocus,
		PauseOnLostFocusWasChanged: pauseOnLostFocusChanged,
	}, nil
}

func quickPlayArgs(singleplayer string) []string {
	singleplayer = strings.TrimSpace(singleplayer)
	if singleplayer == "" {
		return nil
	}
	return []string{"--quickPlaySingleplayer", singleplayer}
}

func headlessLaunchLog(profile domain.Profile) (string, *os.File, error) {
	logsDir := filepath.Join(profile.GameDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		return "", nil, err
	}
	logPath := filepath.Join(logsDir, "power-mine-headless-"+time.Now().UTC().Format("20060102-150405")+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", nil, err
	}
	return logPath, logFile, nil
}

func headlessLaunchEnv(base []string) []string {
	env := envMap(base)
	if strings.TrimSpace(env["XDG_RUNTIME_DIR"]) == "" {
		runtimeDir := filepath.Join("/run/user", fmt.Sprint(os.Getuid()))
		if info, err := os.Stat(runtimeDir); err == nil && info.IsDir() {
			env["XDG_RUNTIME_DIR"] = runtimeDir
		}
	}
	if strings.TrimSpace(env["DISPLAY"]) == "" {
		if display := inferXDisplay(); display != "" {
			env["DISPLAY"] = display
		}
	}
	if strings.TrimSpace(env["XAUTHORITY"]) == "" && strings.TrimSpace(env["XDG_RUNTIME_DIR"]) != "" {
		for _, candidate := range []string{
			filepath.Join(env["XDG_RUNTIME_DIR"], "gdm", "Xauthority"),
			filepath.Join(env["XDG_RUNTIME_DIR"], "Xauthority"),
		} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				env["XAUTHORITY"] = candidate
				break
			}
		}
	}
	return flattenEnv(env)
}

func envMap(values []string) map[string]string {
	env := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if ok && key != "" {
			env[key] = val
		}
	}
	return env
}

func flattenEnv(env map[string]string) []string {
	values := make([]string, 0, len(env))
	for key, value := range env {
		values = append(values, key+"="+value)
	}
	return values
}

func inferXDisplay() string {
	matches, err := filepath.Glob("/tmp/.X11-unix/X*")
	if err != nil {
		return ""
	}
	for _, match := range matches {
		name := filepath.Base(match)
		display := strings.TrimPrefix(name, "X")
		if display != "" && display != name {
			return ":" + display
		}
	}
	return ""
}

func writeHeadless(ok bool, command string, result interface{}, message string) {
	response := headlessResponse{
		OK:      ok,
		Command: command,
		Result:  result,
	}
	if !ok {
		response.Error = message
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(response)
}
