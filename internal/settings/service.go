package settings

import (
	"path/filepath"

	"power-mine/internal/account"
	"power-mine/internal/domain"
	"power-mine/internal/platform"
	"power-mine/internal/storage"
)

type Service struct {
	path    string
	dataDir string
}

func NewService(dataDir string) *Service {
	return &Service{
		path:    filepath.Join(dataDir, "settings.json"),
		dataDir: dataDir,
	}
}

func Default(dataDir string) domain.Settings {
	return domain.Settings{
		DataDir:  dataDir,
		JavaPath: "java",
		Account:  account.Default(),
		DefaultMemory: domain.MemorySettings{
			MinMB: 1024,
			MaxMB: 4096,
		},
		Network: domain.NetworkSettings{
			RetryCount:       3,
			MetadataTTLHours: 6,
		},
	}
}

func NewDefaultService() (*Service, error) {
	dataDir, err := platform.AppDataDir()
	if err != nil {
		return nil, err
	}
	return NewService(dataDir), nil
}

func (s *Service) Get() (domain.Settings, error) {
	settings, err := storage.ReadJSON(s.path, Default(s.dataDir))
	if err != nil {
		return domain.Settings{}, err
	}
	if settings.DataDir == "" {
		settings.DataDir = s.dataDir
	}
	if settings.JavaPath == "" {
		settings.JavaPath = "java"
	}
	settings.Account = account.Normalize(settings.Account)
	if settings.DefaultMemory.MinMB == 0 {
		settings.DefaultMemory.MinMB = 1024
	}
	if settings.DefaultMemory.MaxMB == 0 {
		settings.DefaultMemory.MaxMB = 4096
	}
	if settings.Network.RetryCount == 0 {
		settings.Network.RetryCount = 3
	}
	if settings.Network.MetadataTTLHours == 0 {
		settings.Network.MetadataTTLHours = 6
	}
	return settings, nil
}

func (s *Service) Save(next domain.Settings) (domain.Settings, error) {
	if next.DataDir == "" {
		next.DataDir = s.dataDir
	}
	if next.DefaultMemory.MinMB <= 0 {
		next.DefaultMemory.MinMB = 1024
	}
	if next.DefaultMemory.MaxMB < next.DefaultMemory.MinMB {
		next.DefaultMemory.MaxMB = next.DefaultMemory.MinMB
	}
	if next.JavaPath == "" {
		next.JavaPath = "java"
	}
	next.Account = account.Normalize(next.Account)
	if err := account.Validate(next.Account); err != nil {
		return domain.Settings{}, err
	}
	if next.Network.RetryCount < 0 {
		next.Network.RetryCount = 0
	}
	if next.Network.MetadataTTLHours <= 0 {
		next.Network.MetadataTTLHours = 6
	}
	return next, storage.WriteJSON(s.path, next)
}
