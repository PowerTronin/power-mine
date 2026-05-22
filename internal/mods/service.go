package mods

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"power-mine/internal/domain"
	"power-mine/internal/storage"
)

var ErrInvalidMod = errors.New("invalid mod")

type Service struct{}

const modrinthManifestFileName = ".power-mine-modrinth.json"

type modrinthManifest struct {
	Installations []modrinthInstallRecord `json:"installations"`
}

type modrinthInstallRecord struct {
	ProjectID     string                         `json:"projectId"`
	ProjectTitle  string                         `json:"projectTitle"`
	VersionID     string                         `json:"versionId"`
	VersionName   string                         `json:"versionName"`
	VersionNumber string                         `json:"versionNumber"`
	FileName      string                         `json:"fileName"`
	Files         []domain.ModrinthInstalledFile `json:"files"`
	InstalledAt   string                         `json:"installedAt"`
}

type fabricMetadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type quiltMetadata struct {
	QuiltLoader struct {
		ID       string `json:"id"`
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"quilt_loader"`
}

func NewService() *Service {
	return &Service{}
}

func (s *Service) List(profile domain.Profile) (domain.ModList, error) {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModList{}, err
	}

	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return domain.ModList{}, err
	}

	modFiles := make([]domain.ModFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isModFileName(entry.Name()) {
			continue
		}

		modFile, err := s.readModFile(modsDir, entry.Name())
		if err != nil {
			return domain.ModList{}, err
		}
		modFiles = append(modFiles, modFile)
	}

	sort.Slice(modFiles, func(i, j int) bool {
		return strings.ToLower(modFiles[i].DisplayName) < strings.ToLower(modFiles[j].DisplayName)
	})

	return domain.ModList{
		ProfileID: profile.ID,
		ModsDir:   modsDir,
		Mods:      modFiles,
	}, nil
}

func (s *Service) Import(profile domain.Profile, sourcePath string) (domain.ModFile, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return domain.ModFile{}, fmt.Errorf("%w: source path is required", ErrInvalidMod)
	}
	if strings.ToLower(filepath.Ext(sourcePath)) != ".jar" {
		return domain.ModFile{}, fmt.Errorf("%w: only .jar files can be imported", ErrInvalidMod)
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return domain.ModFile{}, err
	}
	if sourceInfo.IsDir() {
		return domain.ModFile{}, fmt.Errorf("%w: source is a directory", ErrInvalidMod)
	}

	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModFile{}, err
	}

	fileName := filepath.Base(sourcePath)
	targetPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return domain.ModFile{}, err
	}
	if fileExists(targetPath) || fileExists(targetPath+".disabled") {
		return domain.ModFile{}, fmt.Errorf("%w: %s already exists", ErrInvalidMod, fileName)
	}

	if err := copyFile(sourcePath, targetPath); err != nil {
		return domain.ModFile{}, err
	}
	return s.readModFile(modsDir, fileName)
}

func (s *Service) Install(profile domain.Profile, fileName string, reader io.Reader) (domain.ModFile, error) {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if strings.ToLower(filepath.Ext(fileName)) != ".jar" {
		return domain.ModFile{}, fmt.Errorf("%w: only .jar files can be installed", ErrInvalidMod)
	}

	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModFile{}, err
	}
	targetPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return domain.ModFile{}, err
	}
	if fileExists(targetPath) || fileExists(targetPath+".disabled") {
		return domain.ModFile{}, fmt.Errorf("%w: %s already exists", ErrInvalidMod, fileName)
	}
	if err := writeFile(reader, targetPath); err != nil {
		return domain.ModFile{}, err
	}
	return s.readModFile(modsDir, fileName)
}

func (s *Service) Replace(profile domain.Profile, fileName string, reader io.Reader) (domain.ModFile, error) {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModFile{}, err
	}
	targetPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return domain.ModFile{}, err
	}
	if !fileExists(targetPath) {
		return domain.ModFile{}, os.ErrNotExist
	}

	tempFile, err := os.CreateTemp(modsDir, ".power-mine-mod-*.tmp")
	if err != nil {
		return domain.ModFile{}, err
	}
	tempPath := tempFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := io.Copy(tempFile, reader); err != nil {
		_ = tempFile.Close()
		return domain.ModFile{}, err
	}
	if err := tempFile.Chmod(0o644); err != nil {
		_ = tempFile.Close()
		return domain.ModFile{}, err
	}
	if err := tempFile.Close(); err != nil {
		return domain.ModFile{}, err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return domain.ModFile{}, err
	}
	removeTemp = false
	return s.readModFile(modsDir, fileName)
}

func (s *Service) Existing(profile domain.Profile, fileName string) (domain.ModFile, bool, error) {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if strings.ToLower(filepath.Ext(fileName)) != ".jar" {
		return domain.ModFile{}, false, fmt.Errorf("%w: only .jar files can be checked", ErrInvalidMod)
	}

	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModFile{}, false, err
	}

	for _, candidate := range []string{fileName, fileName + ".disabled"} {
		modPath, err := s.modPath(modsDir, candidate)
		if err != nil {
			return domain.ModFile{}, false, err
		}
		if fileExists(modPath) {
			modFile, err := s.readModFile(modsDir, candidate)
			return modFile, true, err
		}
	}

	return domain.ModFile{}, false, nil
}

func (s *Service) SetEnabled(profile domain.Profile, fileName string, enabled bool) (domain.ModFile, error) {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return domain.ModFile{}, err
	}

	currentPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return domain.ModFile{}, err
	}
	if !fileExists(currentPath) {
		return domain.ModFile{}, os.ErrNotExist
	}

	currentEnabled := isEnabledModFileName(fileName)
	if currentEnabled == enabled {
		return s.readModFile(modsDir, fileName)
	}

	nextName := fileName + ".disabled"
	if enabled {
		nextName = trimDisabledSuffix(fileName)
	}
	nextPath, err := s.modPath(modsDir, nextName)
	if err != nil {
		return domain.ModFile{}, err
	}
	if fileExists(nextPath) {
		return domain.ModFile{}, fmt.Errorf("%w: %s already exists", ErrInvalidMod, nextName)
	}
	if err := os.Rename(currentPath, nextPath); err != nil {
		return domain.ModFile{}, err
	}
	return s.readModFile(modsDir, nextName)
}

func (s *Service) Delete(profile domain.Profile, fileName string) error {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return err
	}
	modPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return err
	}
	return os.Remove(modPath)
}

func (s *Service) RecordModrinthInstall(profile domain.Profile, result domain.ModrinthInstallResult) error {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return err
	}

	record := modrinthInstallRecord{
		ProjectID:     result.ProjectID,
		ProjectTitle:  result.ProjectTitle,
		VersionID:     result.VersionID,
		VersionName:   result.VersionName,
		VersionNumber: result.VersionNumber,
		FileName:      result.FileName,
		Files:         append([]domain.ModrinthInstalledFile{}, result.InstalledFiles...),
		InstalledAt:   time.Now().UTC().Format(time.RFC3339),
	}

	replaced := false
	for index := range manifest.Installations {
		if manifest.Installations[index].ProjectID == record.ProjectID {
			manifest.Installations[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		manifest.Installations = append(manifest.Installations, record)
	}
	return s.writeModrinthManifest(profile, manifest)
}

func (s *Service) ModrinthInstalledProjectIDs(profile domain.Profile) ([]string, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(manifest.Installations))
	for _, record := range manifest.Installations {
		if strings.TrimSpace(record.ProjectID) != "" {
			ids = append(ids, record.ProjectID)
		}
	}
	return ids, nil
}

func (s *Service) ModrinthInstalledProjects(profile domain.Profile) ([]domain.ModrinthInstalledProject, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return nil, err
	}
	projects := make([]domain.ModrinthInstalledProject, 0, len(manifest.Installations))
	for _, record := range manifest.Installations {
		projects = append(projects, modrinthInstalledProject(profile.ID, record))
	}
	return projects, nil
}

func (s *Service) ModrinthInstalledProject(profile domain.Profile, projectID string) (domain.ModrinthInstalledProject, bool, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return domain.ModrinthInstalledProject{}, false, err
	}
	record, found := findModrinthInstallRecord(manifest, projectID)
	if !found {
		return domain.ModrinthInstalledProject{}, false, nil
	}
	return modrinthInstalledProject(profile.ID, record), true, nil
}

func (s *Service) PlanModrinthDelete(profile domain.Profile, projectID string) (domain.ModrinthDeletePlan, bool, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return domain.ModrinthDeletePlan{}, false, err
	}

	record, found := findModrinthInstallRecord(manifest, projectID)
	if !found {
		return domain.ModrinthDeletePlan{}, false, nil
	}

	files, skipped, err := s.modrinthDeleteFiles(profile, manifest, record)
	if err != nil {
		return domain.ModrinthDeletePlan{}, false, err
	}
	return domain.ModrinthDeletePlan{
		ProfileID:    profile.ID,
		ProjectID:    record.ProjectID,
		ProjectTitle: record.ProjectTitle,
		Files:        files,
		SkippedFiles: skipped,
		Tracked:      true,
	}, true, nil
}

func (s *Service) PruneReplacedModrinthFiles(profile domain.Profile, projectID string, newFiles []domain.ModrinthInstalledFile) ([]domain.ModrinthDeleteFile, []domain.ModrinthDeleteFile, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return nil, nil, err
	}
	record, found := findModrinthInstallRecord(manifest, projectID)
	if !found {
		return nil, nil, nil
	}

	newFileKeys := map[string]bool{}
	for _, file := range newFiles {
		newFileKeys[modrinthFileKey(file.FileName)] = true
	}
	references := modrinthFileReferences(manifest, record.ProjectID)
	deleted := make([]domain.ModrinthDeleteFile, 0)
	skipped := make([]domain.ModrinthDeleteFile, 0)
	for _, installed := range record.Files {
		deleteFile := domain.ModrinthDeleteFile{
			ProjectID:      installed.ProjectID,
			ProjectTitle:   record.ProjectTitle,
			VersionID:      installed.VersionID,
			VersionName:    installed.VersionName,
			VersionNumber:  installed.VersionNumber,
			FileName:       installed.FileName,
			DisplayName:    installed.DisplayName,
			DependencyType: installed.DependencyType,
		}
		if installed.DependencyType != "" {
			deleteFile.ProjectTitle = installed.DisplayName
		}
		key := modrinthFileKey(installed.FileName)
		if newFileKeys[key] {
			deleteFile.Reason = "kept by updated install"
			skipped = append(skipped, deleteFile)
			continue
		}
		if installed.AlreadyPresent {
			deleteFile.Reason = "already existed before this install"
			skipped = append(skipped, deleteFile)
			continue
		}
		if titles := references[key]; len(titles) > 0 {
			deleteFile.Reason = "used by " + strings.Join(titles, ", ")
			skipped = append(skipped, deleteFile)
			continue
		}
		existing, ok, err := s.Existing(profile, installed.FileName)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			deleteFile.Reason = "file is already missing"
			skipped = append(skipped, deleteFile)
			continue
		}
		deleteFile.FileName = existing.FileName
		if existing.DisplayName != "" {
			deleteFile.DisplayName = existing.DisplayName
		}
		if err := s.Delete(profile, existing.FileName); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				deleteFile.Reason = "file is already missing"
				skipped = append(skipped, deleteFile)
				continue
			}
			return nil, nil, err
		}
		deleted = append(deleted, deleteFile)
	}
	return deleted, skipped, nil
}

func (s *Service) DeleteModrinthInstall(profile domain.Profile, projectID string) (domain.ModrinthDeleteResult, bool, error) {
	return s.DeleteModrinthInstallFiles(profile, projectID, nil)
}

func (s *Service) DeleteModrinthInstallFiles(profile domain.Profile, projectID string, selectedFileNames []string) (domain.ModrinthDeleteResult, bool, error) {
	manifest, err := s.readModrinthManifest(profile)
	if err != nil {
		return domain.ModrinthDeleteResult{}, false, err
	}

	record, found := findModrinthInstallRecord(manifest, projectID)
	if !found {
		return domain.ModrinthDeleteResult{}, false, nil
	}

	files, skipped, err := s.modrinthDeleteFiles(profile, manifest, record)
	if err != nil {
		return domain.ModrinthDeleteResult{}, false, err
	}
	files, skipped = selectModrinthDeleteFiles(files, skipped, selectedFileNames)
	deleted := make([]domain.ModrinthDeleteFile, 0, len(files))
	for _, file := range files {
		if err := s.Delete(profile, file.FileName); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				file.Reason = "file is already missing"
				skipped = append(skipped, file)
				continue
			}
			return domain.ModrinthDeleteResult{}, false, err
		}
		deleted = append(deleted, file)
	}

	if shouldRemoveModrinthRecord(record, deleted) {
		manifest.Installations = removeModrinthInstallRecord(manifest.Installations, record.ProjectID)
	} else {
		manifest.Installations = updateModrinthInstallRecordFiles(manifest.Installations, record.ProjectID, deleted)
	}
	if err := s.writeModrinthManifest(profile, manifest); err != nil {
		return domain.ModrinthDeleteResult{}, false, err
	}

	modList, err := s.List(profile)
	if err != nil {
		return domain.ModrinthDeleteResult{}, false, err
	}
	return domain.ModrinthDeleteResult{
		ProfileID:    profile.ID,
		ProjectID:    record.ProjectID,
		ProjectTitle: record.ProjectTitle,
		DeletedFiles: deleted,
		SkippedFiles: skipped,
		ModList:      modList,
	}, true, nil
}

func (s *Service) DeleteModrinthFiles(profile domain.Profile, plan domain.ModrinthDeletePlan) (domain.ModrinthDeleteResult, error) {
	return s.DeleteSelectedModrinthFiles(profile, plan, nil)
}

func (s *Service) DeleteSelectedModrinthFiles(profile domain.Profile, plan domain.ModrinthDeletePlan, selectedFileNames []string) (domain.ModrinthDeleteResult, error) {
	files, skipped := selectModrinthDeleteFiles(plan.Files, plan.SkippedFiles, selectedFileNames)
	deleted := make([]domain.ModrinthDeleteFile, 0, len(plan.Files))
	for _, file := range files {
		if err := s.Delete(profile, file.FileName); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				file.Reason = "file is already missing"
				skipped = append(skipped, file)
				continue
			}
			return domain.ModrinthDeleteResult{}, err
		}
		deleted = append(deleted, file)
	}

	modList, err := s.List(profile)
	if err != nil {
		return domain.ModrinthDeleteResult{}, err
	}
	return domain.ModrinthDeleteResult{
		ProfileID:    profile.ID,
		ProjectID:    plan.ProjectID,
		ProjectTitle: plan.ProjectTitle,
		DeletedFiles: deleted,
		SkippedFiles: skipped,
		ModList:      modList,
	}, nil
}

func (s *Service) EnsureDir(profile domain.Profile) (string, error) {
	if strings.TrimSpace(profile.GameDir) == "" {
		return "", fmt.Errorf("%w: profile game directory is empty", ErrInvalidMod)
	}
	modsDir := filepath.Join(profile.GameDir, "mods")
	if err := os.MkdirAll(modsDir, 0o755); err != nil {
		return "", err
	}
	return modsDir, nil
}

func (s *Service) readModFile(modsDir string, fileName string) (domain.ModFile, error) {
	modPath, err := s.modPath(modsDir, fileName)
	if err != nil {
		return domain.ModFile{}, err
	}
	info, err := os.Stat(modPath)
	if err != nil {
		return domain.ModFile{}, err
	}
	if info.IsDir() {
		return domain.ModFile{}, fmt.Errorf("%w: %s is a directory", ErrInvalidMod, fileName)
	}

	enabled := isEnabledModFileName(fileName)
	displayName := fallbackDisplayName(fileName)
	if metadataName, ok := readMetadataDisplayName(modPath); ok {
		displayName = metadataName
	}
	hash, err := fileSHA1(modPath)
	if err != nil {
		return domain.ModFile{}, err
	}
	return domain.ModFile{
		FileName:    fileName,
		DisplayName: displayName,
		Path:        modPath,
		Enabled:     enabled,
		Size:        info.Size(),
		UpdatedAt:   info.ModTime().UTC().Format(time.RFC3339),
		SHA1:        hash,
	}, nil
}

func fileSHA1(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *Service) modPath(modsDir string, fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("%w: file name is required", ErrInvalidMod)
	}
	if strings.ContainsAny(fileName, `/\`) || filepath.Clean(fileName) != fileName || filepath.Base(fileName) != fileName {
		return "", fmt.Errorf("%w: invalid file name", ErrInvalidMod)
	}
	if !isModFileName(fileName) {
		return "", fmt.Errorf("%w: unsupported file name %q", ErrInvalidMod, fileName)
	}
	return filepath.Join(modsDir, fileName), nil
}

func isModFileName(fileName string) bool {
	return isEnabledModFileName(fileName) || isDisabledModFileName(fileName)
}

func isEnabledModFileName(fileName string) bool {
	return strings.ToLower(filepath.Ext(fileName)) == ".jar"
}

func isDisabledModFileName(fileName string) bool {
	return strings.HasSuffix(strings.ToLower(fileName), ".jar.disabled")
}

func trimDisabledSuffix(fileName string) string {
	if !isDisabledModFileName(fileName) {
		return fileName
	}
	return fileName[:len(fileName)-len(".disabled")]
}

func fallbackDisplayName(fileName string) string {
	enabledName := trimDisabledSuffix(fileName)
	return strings.TrimSuffix(enabledName, filepath.Ext(enabledName))
}

func (s *Service) readModrinthManifest(profile domain.Profile) (modrinthManifest, error) {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return modrinthManifest{}, err
	}
	return storage.ReadJSON(filepath.Join(modsDir, modrinthManifestFileName), modrinthManifest{})
}

func (s *Service) writeModrinthManifest(profile domain.Profile, manifest modrinthManifest) error {
	modsDir, err := s.EnsureDir(profile)
	if err != nil {
		return err
	}
	sort.Slice(manifest.Installations, func(i, j int) bool {
		return strings.ToLower(manifest.Installations[i].ProjectTitle) < strings.ToLower(manifest.Installations[j].ProjectTitle)
	})
	return storage.WriteJSON(filepath.Join(modsDir, modrinthManifestFileName), manifest)
}

func (s *Service) modrinthDeleteFiles(profile domain.Profile, manifest modrinthManifest, record modrinthInstallRecord) ([]domain.ModrinthDeleteFile, []domain.ModrinthDeleteFile, error) {
	references := modrinthFileReferences(manifest, record.ProjectID)
	files := make([]domain.ModrinthDeleteFile, 0, len(record.Files))
	skipped := make([]domain.ModrinthDeleteFile, 0)

	for _, installed := range record.Files {
		deleteFile := domain.ModrinthDeleteFile{
			ProjectID:      installed.ProjectID,
			ProjectTitle:   record.ProjectTitle,
			VersionID:      installed.VersionID,
			VersionName:    installed.VersionName,
			VersionNumber:  installed.VersionNumber,
			FileName:       installed.FileName,
			DisplayName:    installed.DisplayName,
			DependencyType: installed.DependencyType,
		}
		if installed.DependencyType != "" {
			deleteFile.ProjectTitle = installed.DisplayName
		}
		if installed.AlreadyPresent {
			deleteFile.Reason = "already existed before this install"
			skipped = append(skipped, deleteFile)
			continue
		}
		if titles := references[modrinthFileKey(installed.FileName)]; len(titles) > 0 {
			deleteFile.Reason = "used by " + strings.Join(titles, ", ")
			skipped = append(skipped, deleteFile)
			continue
		}
		existing, ok, err := s.Existing(profile, installed.FileName)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			deleteFile.Reason = "file is already missing"
			skipped = append(skipped, deleteFile)
			continue
		}
		deleteFile.FileName = existing.FileName
		if existing.DisplayName != "" {
			deleteFile.DisplayName = existing.DisplayName
		}
		files = append(files, deleteFile)
	}

	return files, skipped, nil
}

func findModrinthInstallRecord(manifest modrinthManifest, projectID string) (modrinthInstallRecord, bool) {
	projectID = strings.TrimSpace(projectID)
	for _, record := range manifest.Installations {
		if record.ProjectID == projectID {
			return record, true
		}
	}
	return modrinthInstallRecord{}, false
}

func modrinthInstalledProject(profileID string, record modrinthInstallRecord) domain.ModrinthInstalledProject {
	return domain.ModrinthInstalledProject{
		ProfileID:     profileID,
		ProjectID:     record.ProjectID,
		ProjectTitle:  record.ProjectTitle,
		VersionID:     record.VersionID,
		VersionName:   record.VersionName,
		VersionNumber: record.VersionNumber,
		FileName:      record.FileName,
		Files:         append([]domain.ModrinthInstalledFile{}, record.Files...),
		InstalledAt:   record.InstalledAt,
	}
}

func removeModrinthInstallRecord(records []modrinthInstallRecord, projectID string) []modrinthInstallRecord {
	next := records[:0]
	for _, record := range records {
		if record.ProjectID == projectID {
			continue
		}
		next = append(next, record)
	}
	return next
}

func updateModrinthInstallRecordFiles(records []modrinthInstallRecord, projectID string, deleted []domain.ModrinthDeleteFile) []modrinthInstallRecord {
	deletedKeys := map[string]bool{}
	for _, file := range deleted {
		deletedKeys[modrinthFileKey(file.FileName)] = true
	}
	for recordIndex := range records {
		if records[recordIndex].ProjectID != projectID {
			continue
		}
		files := records[recordIndex].Files[:0]
		for _, file := range records[recordIndex].Files {
			if deletedKeys[modrinthFileKey(file.FileName)] {
				continue
			}
			files = append(files, file)
		}
		records[recordIndex].Files = files
		break
	}
	return records
}

func shouldRemoveModrinthRecord(record modrinthInstallRecord, deleted []domain.ModrinthDeleteFile) bool {
	mainKey := modrinthFileKey(record.FileName)
	for _, file := range deleted {
		if modrinthFileKey(file.FileName) == mainKey {
			return true
		}
	}
	return false
}

func selectModrinthDeleteFiles(files []domain.ModrinthDeleteFile, skipped []domain.ModrinthDeleteFile, selectedFileNames []string) ([]domain.ModrinthDeleteFile, []domain.ModrinthDeleteFile) {
	if len(selectedFileNames) == 0 {
		return append([]domain.ModrinthDeleteFile{}, files...), append([]domain.ModrinthDeleteFile{}, skipped...)
	}
	selected := map[string]bool{}
	for _, fileName := range selectedFileNames {
		selected[modrinthFileKey(fileName)] = true
	}
	nextFiles := make([]domain.ModrinthDeleteFile, 0, len(files))
	nextSkipped := append([]domain.ModrinthDeleteFile{}, skipped...)
	for _, file := range files {
		if selected[modrinthFileKey(file.FileName)] {
			nextFiles = append(nextFiles, file)
			continue
		}
		file.Reason = "not selected"
		nextSkipped = append(nextSkipped, file)
	}
	return nextFiles, nextSkipped
}

func modrinthFileReferences(manifest modrinthManifest, projectID string) map[string][]string {
	references := map[string][]string{}
	for _, record := range manifest.Installations {
		if record.ProjectID == projectID {
			continue
		}
		title := record.ProjectTitle
		if title == "" {
			title = record.ProjectID
		}
		for _, file := range record.Files {
			key := modrinthFileKey(file.FileName)
			if key == "" {
				continue
			}
			if !stringInSlice(references[key], title) {
				references[key] = append(references[key], title)
			}
		}
	}
	return references
}

func modrinthFileKey(fileName string) string {
	fileName = strings.ToLower(strings.TrimSpace(fileName))
	fileName = strings.TrimSuffix(fileName, ".disabled")
	return fileName
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func readMetadataDisplayName(modPath string) (string, bool) {
	reader, err := zip.OpenReader(modPath)
	if err != nil {
		return "", false
	}
	defer reader.Close()

	if name, ok := readFabricDisplayName(reader.File); ok {
		return name, true
	}
	if name, ok := readQuiltDisplayName(reader.File); ok {
		return name, true
	}
	if name, ok := readTOMLDisplayName(reader.File, "META-INF/mods.toml"); ok {
		return name, true
	}
	if name, ok := readTOMLDisplayName(reader.File, "META-INF/neoforge.mods.toml"); ok {
		return name, true
	}
	return "", false
}

func readFabricDisplayName(files []*zip.File) (string, bool) {
	data, ok := readZipFile(files, "fabric.mod.json")
	if !ok {
		return "", false
	}
	var metadata fabricMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", false
	}
	return firstUsefulName(metadata.Name, metadata.ID)
}

func readQuiltDisplayName(files []*zip.File) (string, bool) {
	data, ok := readZipFile(files, "quilt.mod.json")
	if !ok {
		return "", false
	}
	var metadata quiltMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return "", false
	}
	return firstUsefulName(metadata.QuiltLoader.Metadata.Name, metadata.QuiltLoader.ID)
}

func readTOMLDisplayName(files []*zip.File, name string) (string, bool) {
	data, ok := readZipFile(files, name)
	if !ok {
		return "", false
	}
	return firstUsefulName(tomlValue(string(data), "displayName"), tomlValue(string(data), "modId"))
}

func readZipFile(files []*zip.File, name string) ([]byte, bool) {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, false
		}
		defer reader.Close()

		data, err := io.ReadAll(io.LimitReader(reader, 1024*1024))
		return data, err == nil
	}
	return nil, false
}

func tomlValue(data string, key string) string {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(name) != key {
			continue
		}
		return cleanMetadataName(value)
	}
	return ""
}

func firstUsefulName(values ...string) (string, bool) {
	for _, value := range values {
		value = cleanMetadataName(value)
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func cleanMetadataName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	if value == "" || strings.HasPrefix(value, "${") {
		return ""
	}
	return value
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(sourcePath string, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	return writeFile(source, targetPath)
}

func writeFile(reader io.Reader, targetPath string) error {
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	if _, err := io.Copy(target, reader); err != nil {
		_ = target.Close()
		_ = os.Remove(targetPath)
		return err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return nil
}
