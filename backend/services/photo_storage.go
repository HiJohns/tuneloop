package services

import (
	"fmt"
	"os"
	"path/filepath"
)

type PhotoStorageService struct {
	basePath string
}

func NewPhotoStorageService() *PhotoStorageService {
	return &PhotoStorageService{
		basePath: filepath.Join(".", "uploads", "photos"),
	}
}

// UpdateLatest creates/updates the latest symlink for an instrument
func (s *PhotoStorageService) UpdateLatest(tenantID string, instrumentSN string, batchDir string) error {
	if tenantID == "" {
		tenantID = "default"
	}
	if instrumentSN == "" {
		instrumentSN = "unknown_sn"
	}

	photoBaseDir := filepath.Join(s.basePath, tenantID, instrumentSN)
	latestDir := filepath.Join(photoBaseDir, "latest")

	if err := os.RemoveAll(latestDir); err != nil {
		return fmt.Errorf("failed to remove existing latest directory: %w", err)
	}

	srcPath := filepath.Join(photoBaseDir, batchDir)
	if err := os.Symlink(srcPath, latestDir); err != nil {
		return s.copyAsLatest(srcPath, latestDir)
	}

	return nil
}

// copyAsLatest copies files from batch directory to latest (fallback for Windows)
func (s *PhotoStorageService) copyAsLatest(srcDir string, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create latest directory: %w", err)
	}

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dstDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		if info.Name() == "manifest.yaml" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// GetLatestPhotos returns paths to latest photos for an instrument
func (s *PhotoStorageService) GetLatestPhotos(tenantID string, instrumentSN string) ([]string, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if instrumentSN == "" {
		instrumentSN = "unknown_sn"
	}

	latestDir := filepath.Join(s.basePath, tenantID, instrumentSN, "latest")

	if _, err := os.Stat(latestDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no latest photos found")
	}

	var photos []string
	entries, err := os.ReadDir(latestDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read latest directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "manifest.yaml" {
			photoPath := filepath.Join("/uploads/photos", tenantID, instrumentSN, "latest", entry.Name())
			photos = append(photos, photoPath)
		}
	}

	return photos, nil
}
