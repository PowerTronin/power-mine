package servers

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"power-mine/internal/domain"
	"power-mine/internal/platform"
	"power-mine/internal/storage"
)

var (
	ErrServerNotFound = errors.New("local server not found")
	ErrInvalidServer  = errors.New("invalid local server")
)

const maxServerNameLength = 64

type fileData struct {
	Servers []domain.LocalServer `json:"servers"`
}

type Service struct {
	mu      sync.Mutex
	path    string
	dataDir string
}

func NewService(dataDir string) *Service {
	return &Service{
		path:    filepath.Join(dataDir, "local-servers.json"),
		dataDir: dataDir,
	}
}

func (s *Service) List() (domain.LocalServerList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return domain.LocalServerList{}, err
	}
	return domain.LocalServerList{Servers: data.Servers}, nil
}

func (s *Service) Get(id string) (domain.LocalServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return domain.LocalServer{}, err
	}
	for _, server := range data.Servers {
		if server.ID == id {
			return server, nil
		}
	}
	return domain.LocalServer{}, ErrServerNotFound
}

func (s *Service) Create(input domain.LocalServerInput, defaults domain.MemorySettings) (domain.LocalServer, error) {
	if input.Memory.MinMB == 0 && input.Memory.MaxMB == 0 {
		input.Memory = defaults
	}
	if input.Port == 0 {
		input.Port = 25565
	}
	if err := validateInput(input); err != nil {
		return domain.LocalServer{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return domain.LocalServer{}, err
	}

	id, err := newID()
	if err != nil {
		return domain.LocalServer{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	serverDir := strings.TrimSpace(input.ServerDir)
	if serverDir == "" {
		serverDir = s.defaultServerDir(data, input.Name, "")
	}

	server := domain.LocalServer{
		ID:               id,
		Name:             strings.TrimSpace(input.Name),
		MinecraftVersion: strings.TrimSpace(input.MinecraftVersion),
		ServerDir:        serverDir,
		Memory:           normalizeMemory(input.Memory),
		Port:             input.Port,
		EulaAccepted:     input.EulaAccepted,
		Install: domain.InstallState{
			Status: "not-installed",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data.Servers = append(data.Servers, server)
	if err := s.write(data); err != nil {
		return domain.LocalServer{}, err
	}
	return server, nil
}

func (s *Service) SetInstallState(id string, install domain.InstallState) (domain.LocalServer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := s.read()
	if err != nil {
		return domain.LocalServer{}, err
	}
	for index := range data.Servers {
		if data.Servers[index].ID != id {
			continue
		}
		data.Servers[index].Install = install
		data.Servers[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.write(data); err != nil {
			return domain.LocalServer{}, err
		}
		return data.Servers[index], nil
	}
	return domain.LocalServer{}, ErrServerNotFound
}

func (s *Service) read() (fileData, error) {
	return storage.ReadJSON(s.path, fileData{Servers: []domain.LocalServer{}})
}

func (s *Service) write(data fileData) error {
	if data.Servers == nil {
		data.Servers = []domain.LocalServer{}
	}
	return storage.WriteJSON(s.path, data)
}

func (s *Service) defaultServerDir(data fileData, serverName string, excludeServerID string) string {
	baseName := serverNameSlug(serverName)
	for suffix := 0; ; suffix++ {
		serverDir := platform.LocalServerDir(s.dataDir, serverNameWithSuffix(baseName, suffix))
		if serverDirUsedByAnotherServer(data, serverDir, excludeServerID) {
			continue
		}
		if !serverDirOwnedByServer(data, serverDir, excludeServerID) && pathExists(serverDir) {
			continue
		}
		return serverDir
	}
}

func serverDirUsedByAnotherServer(data fileData, serverDir string, excludeServerID string) bool {
	for _, server := range data.Servers {
		if server.ID == excludeServerID {
			continue
		}
		if samePath(server.ServerDir, serverDir) {
			return true
		}
	}
	return false
}

func serverDirOwnedByServer(data fileData, serverDir string, serverID string) bool {
	if serverID == "" {
		return false
	}
	for _, server := range data.Servers {
		if server.ID == serverID && samePath(server.ServerDir, serverDir) {
			return true
		}
	}
	return false
}

func samePath(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validateInput(input domain.LocalServerInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidServer)
	}
	if len(name) > 80 {
		return fmt.Errorf("%w: name is too long", ErrInvalidServer)
	}
	if strings.TrimSpace(input.MinecraftVersion) == "" {
		return fmt.Errorf("%w: minecraft version is required", ErrInvalidServer)
	}
	if input.Port < 1 || input.Port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidServer)
	}
	memory := normalizeMemory(input.Memory)
	if memory.MinMB <= 0 || memory.MaxMB <= 0 {
		return fmt.Errorf("%w: memory must be positive", ErrInvalidServer)
	}
	if memory.MaxMB < memory.MinMB {
		return fmt.Errorf("%w: max memory must be greater than or equal to min memory", ErrInvalidServer)
	}
	return nil
}

func normalizeMemory(memory domain.MemorySettings) domain.MemorySettings {
	return domain.MemorySettings{
		MinMB: memory.MinMB,
		MaxMB: memory.MaxMB,
	}
}

func serverNameSlug(name string) string {
	name = strings.TrimSpace(name)
	var builder strings.Builder
	lastDash := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
			lastDash = false
			continue
		}
		if builder.Len() > 0 && !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(builder.String(), "- .")
	if slug == "" {
		slug = "server"
	}
	return trimServerName(slug, maxServerNameLength)
}

func serverNameWithSuffix(baseName string, suffix int) string {
	if suffix == 0 {
		return baseName
	}
	suffixText := fmt.Sprintf("-%d", suffix+1)
	return trimServerName(baseName, maxServerNameLength-len(suffixText)) + suffixText
}

func trimServerName(name string, maxRunes int) string {
	if maxRunes <= 0 {
		return "server"
	}
	runes := []rune(name)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	trimmed := strings.Trim(string(runes), "- .")
	if trimmed == "" {
		return "server"
	}
	return trimmed
}

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
