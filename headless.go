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
	ProfileID string `json:"profileId"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	PID       int    `json:"pid"`
	LogPath   string `json:"logPath,omitempty"`
	StartedAt string `json:"startedAt"`
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

	commandSpec, err := app.minecraftService.BuildLaunchCommand(ctx, profile, minecraft.LaunchOptions{
		JavaPath: javaPath,
		Memory:   profile.Memory,
		Account:  currentSettings.Account,
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
	command.Stdout = logFile
	command.Stderr = logFile
	command.Stdin = nil
	if err := command.Start(); err != nil {
		return headlessLaunchResult{}, err
	}

	return headlessLaunchResult{
		ProfileID: profile.ID,
		Status:    "running",
		Message:   "Minecraft process started",
		PID:       command.Process.Pid,
		LogPath:   logPath,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
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
