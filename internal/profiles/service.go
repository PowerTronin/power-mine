package profiles

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"power-mine/internal/domain"
	"power-mine/internal/platform"
	"power-mine/internal/storage"
)

var (
	ErrProfileNotFound = errors.New("profile not found")
	ErrInvalidProfile  = errors.New("invalid profile")
)

type fileData struct {
	SelectedProfileID string           `json:"selectedProfileId"`
	Profiles          []domain.Profile `json:"profiles"`
}

type Service struct {
	path    string
	dataDir string
}

func NewService(dataDir string) *Service {
	return &Service{
		path:    filepath.Join(dataDir, "profiles.json"),
		dataDir: dataDir,
	}
}

func (s *Service) List() (domain.ProfileList, error) {
	data, err := s.read()
	if err != nil {
		return domain.ProfileList{}, err
	}
	return domain.ProfileList{
		SelectedProfileID: data.SelectedProfileID,
		Profiles:          data.Profiles,
	}, nil
}

func (s *Service) Get(id string) (domain.Profile, error) {
	data, err := s.read()
	if err != nil {
		return domain.Profile{}, err
	}
	for _, profile := range data.Profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return domain.Profile{}, ErrProfileNotFound
}

func (s *Service) Create(input domain.ProfileInput, defaults domain.MemorySettings) (domain.Profile, error) {
	if input.Memory.MinMB == 0 && input.Memory.MaxMB == 0 {
		input.Memory = defaults
	}
	if err := validateInput(input); err != nil {
		return domain.Profile{}, err
	}

	data, err := s.read()
	if err != nil {
		return domain.Profile{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newID()
	if err != nil {
		return domain.Profile{}, err
	}

	gameDir := strings.TrimSpace(input.GameDir)
	if gameDir == "" {
		gameDir = platform.ProfileGameDir(s.dataDir, id)
	}

	profile := domain.Profile{
		ID:               id,
		Name:             strings.TrimSpace(input.Name),
		MinecraftVersion: strings.TrimSpace(input.MinecraftVersion),
		Loader:           normalizeLoader(input.Loader),
		GameDir:          gameDir,
		Memory:           normalizeMemory(input.Memory),
		Install: domain.InstallState{
			Status: "not-installed",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	data.Profiles = append(data.Profiles, profile)
	if data.SelectedProfileID == "" {
		data.SelectedProfileID = id
	}
	if err := s.write(data); err != nil {
		return domain.Profile{}, err
	}
	return profile, nil
}

func (s *Service) Update(id string, input domain.ProfileInput, defaults domain.MemorySettings) (domain.Profile, error) {
	if input.Memory.MinMB == 0 && input.Memory.MaxMB == 0 {
		input.Memory = defaults
	}
	if err := validateInput(input); err != nil {
		return domain.Profile{}, err
	}

	data, err := s.read()
	if err != nil {
		return domain.Profile{}, err
	}

	for index := range data.Profiles {
		if data.Profiles[index].ID != id {
			continue
		}

		data.Profiles[index].Name = strings.TrimSpace(input.Name)
		data.Profiles[index].MinecraftVersion = strings.TrimSpace(input.MinecraftVersion)
		data.Profiles[index].Loader = normalizeLoader(input.Loader)
		data.Profiles[index].GameDir = strings.TrimSpace(input.GameDir)
		if data.Profiles[index].GameDir == "" {
			data.Profiles[index].GameDir = platform.ProfileGameDir(s.dataDir, id)
		}
		data.Profiles[index].Memory = normalizeMemory(input.Memory)
		data.Profiles[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339)

		if err := s.write(data); err != nil {
			return domain.Profile{}, err
		}
		return data.Profiles[index], nil
	}

	return domain.Profile{}, ErrProfileNotFound
}

func (s *Service) Delete(id string) error {
	data, err := s.read()
	if err != nil {
		return err
	}

	next := data.Profiles[:0]
	found := false
	for _, profile := range data.Profiles {
		if profile.ID == id {
			found = true
			continue
		}
		next = append(next, profile)
	}
	if !found {
		return ErrProfileNotFound
	}

	data.Profiles = next
	if data.SelectedProfileID == id {
		data.SelectedProfileID = ""
		if len(data.Profiles) > 0 {
			data.SelectedProfileID = data.Profiles[0].ID
		}
	}
	return s.write(data)
}

func (s *Service) Select(id string) (domain.ProfileList, error) {
	data, err := s.read()
	if err != nil {
		return domain.ProfileList{}, err
	}
	for _, profile := range data.Profiles {
		if profile.ID == id {
			data.SelectedProfileID = id
			if err := s.write(data); err != nil {
				return domain.ProfileList{}, err
			}
			return domain.ProfileList{
				SelectedProfileID: data.SelectedProfileID,
				Profiles:          data.Profiles,
			}, nil
		}
	}
	return domain.ProfileList{}, ErrProfileNotFound
}

func (s *Service) SetInstallState(id string, install domain.InstallState) (domain.Profile, error) {
	data, err := s.read()
	if err != nil {
		return domain.Profile{}, err
	}

	for index := range data.Profiles {
		if data.Profiles[index].ID != id {
			continue
		}
		data.Profiles[index].Install = install
		data.Profiles[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.write(data); err != nil {
			return domain.Profile{}, err
		}
		return data.Profiles[index], nil
	}

	return domain.Profile{}, ErrProfileNotFound
}

func (s *Service) TouchInstallState(id string, update func(domain.InstallState) domain.InstallState) (domain.Profile, error) {
	data, err := s.read()
	if err != nil {
		return domain.Profile{}, err
	}

	for index := range data.Profiles {
		if data.Profiles[index].ID != id {
			continue
		}
		data.Profiles[index].Install = update(data.Profiles[index].Install)
		data.Profiles[index].UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.write(data); err != nil {
			return domain.Profile{}, err
		}
		return data.Profiles[index], nil
	}

	return domain.Profile{}, ErrProfileNotFound
}

func (s *Service) read() (fileData, error) {
	return storage.ReadJSON(s.path, fileData{Profiles: []domain.Profile{}})
}

func (s *Service) write(data fileData) error {
	if data.Profiles == nil {
		data.Profiles = []domain.Profile{}
	}
	return storage.WriteJSON(s.path, data)
}

func validateInput(input domain.ProfileInput) error {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidProfile)
	}
	if len(name) > 80 {
		return fmt.Errorf("%w: name is too long", ErrInvalidProfile)
	}
	if strings.TrimSpace(input.MinecraftVersion) == "" {
		return fmt.Errorf("%w: minecraft version is required", ErrInvalidProfile)
	}

	loader := normalizeLoader(input.Loader)
	switch loader.Type {
	case domain.LoaderVanilla, domain.LoaderFabric, domain.LoaderQuilt, domain.LoaderForge, domain.LoaderNeoForge:
	default:
		return fmt.Errorf("%w: unsupported loader %q", ErrInvalidProfile, loader.Type)
	}

	memory := normalizeMemory(input.Memory)
	if memory.MinMB <= 0 || memory.MaxMB <= 0 {
		return fmt.Errorf("%w: memory must be positive", ErrInvalidProfile)
	}
	if memory.MaxMB < memory.MinMB {
		return fmt.Errorf("%w: max memory must be greater than or equal to min memory", ErrInvalidProfile)
	}
	return nil
}

func normalizeLoader(loader domain.LoaderConfig) domain.LoaderConfig {
	loader.Type = domain.LoaderType(strings.ToLower(strings.TrimSpace(string(loader.Type))))
	loader.Version = strings.TrimSpace(loader.Version)
	if loader.Type == "" {
		loader.Type = domain.LoaderVanilla
	}
	return loader
}

func normalizeMemory(memory domain.MemorySettings) domain.MemorySettings {
	return domain.MemorySettings{
		MinMB: memory.MinMB,
		MaxMB: memory.MaxMB,
	}
}

func newID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
