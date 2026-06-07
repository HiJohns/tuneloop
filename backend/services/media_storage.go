package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type MediaStorage interface {
	Upload(ctx context.Context, key string, reader io.Reader, contentType string) error
	GetURL(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	Copy(ctx context.Context, srcKey string, dstKey string) error
	Rename(ctx context.Context, srcKey string, dstKey string) error
}

type LocalStorage struct {
	basePath string
}

func NewLocalStorage() *LocalStorage {
	return &LocalStorage{
		basePath: "./uploads/media",
	}
}

func (s *LocalStorage) fullPath(key string) string {
	return filepath.Join(s.basePath, key)
}

func (s *LocalStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	fullPath := s.fullPath(key)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, reader); err != nil {
		return fmt.Errorf("failed to write file %s: %w", fullPath, err)
	}
	return nil
}

func (s *LocalStorage) Copy(ctx context.Context, srcKey string, dstKey string) error {
	srcPath := s.fullPath(srcKey)
	dstPath := s.fullPath(dstKey)
	dir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create dst: %w", err)
	}
	defer dst.Close()
	_, err = io.Copy(dst, src)
	return err
}

func (s *LocalStorage) Rename(ctx context.Context, srcKey string, dstKey string) error {
	if err := s.Copy(ctx, srcKey, dstKey); err != nil {
		return err
	}
	return s.Delete(ctx, srcKey)
}

func (s *LocalStorage) GetURL(ctx context.Context, key string) (string, error) {
	return fmt.Sprintf("/uploads/media/%s", key), nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if err := os.Remove(s.fullPath(key)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file %s: %w", key, err)
	}
	return nil
}

func (s *LocalStorage) DeletePrefix(ctx context.Context, prefix string) error {
	dir := filepath.Join(s.basePath, prefix)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !info.IsDir() {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to delete %s: %w", path, err)
			}
		}
		return nil
	})
}

func NewMediaStorage() MediaStorage {
	if os.Getenv("OSS_ENDPOINT") != "" && os.Getenv("OSS_BUCKET") != "" {
		return NewOSSStorage()
	}
	return NewLocalStorage()
}

type OSSStorage struct{}

func NewOSSStorage() *OSSStorage {
	return &OSSStorage{}
}

func (s *OSSStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	return fmt.Errorf("OSS storage not implemented yet")
}

func (s *OSSStorage) GetURL(ctx context.Context, key string) (string, error) {
	return "", fmt.Errorf("OSS storage not implemented yet")
}

func (s *OSSStorage) Delete(ctx context.Context, key string) error {
	return fmt.Errorf("OSS storage not implemented yet")
}

func (s *OSSStorage) DeletePrefix(ctx context.Context, prefix string) error {
	return fmt.Errorf("OSS storage not implemented yet")
}

func (s *OSSStorage) Copy(ctx context.Context, srcKey string, dstKey string) error {
	return fmt.Errorf("OSS storage not implemented yet")
}

func (s *OSSStorage) Rename(ctx context.Context, srcKey string, dstKey string) error {
	return fmt.Errorf("OSS storage not implemented yet")
}

func MediaStorageFromContext(c *gin.Context) MediaStorage {
	if v, ok := c.Get("media_storage"); ok {
		if s, ok := v.(MediaStorage); ok {
			return s
		}
	}
	return NewMediaStorage()
}
