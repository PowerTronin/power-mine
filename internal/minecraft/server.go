package minecraft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"power-mine/internal/domain"
)

type LocalServerProgressFunc func(domain.LocalServerProgress)

type ServerLaunchOptions struct {
	JavaPath  string
	Memory    domain.MemorySettings
	ExtraArgs []string
}

func (s *Service) InstallVanillaServer(ctx context.Context, server domain.LocalServer, progress LocalServerProgressFunc) error {
	emit := func(event domain.LocalServerProgress) {
		event.ServerID = server.ID
		if progress != nil {
			progress(event)
		}
	}

	emit(domain.LocalServerProgress{
		Stage:   "metadata",
		Message: "Resolving Minecraft server metadata",
		Percent: 5,
	})

	version, err := s.fetchVersion(ctx, server.MinecraftVersion)
	if err != nil {
		return err
	}
	if version.Downloads.Server.URL == "" {
		return fmt.Errorf("minecraft version %s does not publish a vanilla server jar", server.MinecraftVersion)
	}

	if err := os.MkdirAll(server.ServerDir, 0o755); err != nil {
		return err
	}

	emit(domain.LocalServerProgress{
		Stage:   "download",
		Message: "Downloading vanilla server jar " + server.MinecraftVersion,
		Current: 1,
		Total:   1,
		Percent: 45,
	})
	if err := s.ensureDownload(ctx, downloadItem{
		URL:   version.Downloads.Server.URL,
		Path:  LocalServerJarPath(server),
		SHA1:  version.Downloads.Server.SHA1,
		Size:  version.Downloads.Server.Size,
		Label: "Server jar " + server.MinecraftVersion,
	}); err != nil {
		return err
	}

	emit(domain.LocalServerProgress{
		Stage:   "configure",
		Message: "Writing server configuration",
		Current: 1,
		Total:   1,
		Percent: 90,
	})
	if err := writeLocalServerConfig(server); err != nil {
		return err
	}

	emit(domain.LocalServerProgress{
		Stage:   "complete",
		Message: "Local server is ready",
		Current: 1,
		Total:   1,
		Percent: 100,
		Done:    true,
	})
	return nil
}

func BuildLocalServerLaunchCommand(server domain.LocalServer, options ServerLaunchOptions) (LaunchCommand, error) {
	javaPath := strings.TrimSpace(options.JavaPath)
	if javaPath == "" {
		javaPath = "java"
	}
	serverDir := strings.TrimSpace(server.ServerDir)
	if serverDir == "" {
		return LaunchCommand{}, fmt.Errorf("local server directory is not configured")
	}
	serverJar := LocalServerJarPath(server)
	if _, err := os.Stat(serverJar); err != nil {
		return LaunchCommand{}, fmt.Errorf("server jar is missing: %w", err)
	}

	memory := options.Memory
	if memory.MinMB <= 0 {
		memory.MinMB = server.Memory.MinMB
	}
	if memory.MaxMB <= 0 {
		memory.MaxMB = server.Memory.MaxMB
	}

	args := make([]string, 0, 6+len(options.ExtraArgs))
	if memory.MinMB > 0 {
		args = append(args, fmt.Sprintf("-Xms%dM", memory.MinMB))
	}
	if memory.MaxMB > 0 {
		args = append(args, fmt.Sprintf("-Xmx%dM", memory.MaxMB))
	}
	args = append(args, "-jar", filepath.Base(serverJar), "nogui")
	args = append(args, options.ExtraArgs...)

	return LaunchCommand{
		JavaPath: javaPath,
		Args:     args,
		WorkDir:  serverDir,
	}, nil
}

func LocalServerJarPath(server domain.LocalServer) string {
	return filepath.Join(server.ServerDir, "server.jar")
}

func writeLocalServerConfig(server domain.LocalServer) error {
	if err := os.MkdirAll(server.ServerDir, 0o755); err != nil {
		return err
	}
	eulaValue := "false"
	if server.EulaAccepted {
		eulaValue = "true"
	}
	if err := os.WriteFile(filepath.Join(server.ServerDir, "eula.txt"), []byte("eula="+eulaValue+"\n"), 0o644); err != nil {
		return err
	}
	propertiesPath := filepath.Join(server.ServerDir, "server.properties")
	if _, err := os.Stat(propertiesPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	properties := fmt.Sprintf("server-port=%d\nonline-mode=false\nenable-command-block=false\nmotd=Power Mine local server\n", server.Port)
	return os.WriteFile(propertiesPath, []byte(properties), 0o644)
}
