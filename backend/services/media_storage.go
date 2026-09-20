package services

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type MediaStorage interface {
	Upload(ctx context.Context, key string, reader io.Reader, contentType string) error
	GetURL(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
	DeletePrefix(ctx context.Context, prefix string) error
	Copy(ctx context.Context, srcKey string, dstKey string) error
	Rename(ctx context.Context, srcKey string, dstKey string) error
	// Stat returns the object size in bytes and whether it exists
	// (#1914 P4: idempotent backfill size comparison).
	Stat(ctx context.Context, key string) (int64, bool, error)
	// List enumerates objects under prefix (empty = all), backend-agnostic
	// (#1914 P6: gc-media orphan sweep over OSS).
	List(ctx context.Context, prefix string) ([]MediaObject, error)
}

// MediaObject is a backend-agnostic listing entry (LocalStorage file or OSS object).
type MediaObject struct {
	Key          string
	Size         int64
	LastModified time.Time
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

// List walks the local media tree under prefix (empty = whole tree).
func (s *LocalStorage) List(ctx context.Context, prefix string) ([]MediaObject, error) {
	base := s.fullPath(prefix)
	var out []MediaObject
	err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(s.basePath, path)
		if rerr != nil {
			return rerr
		}
		out = append(out, MediaObject{
			Key:          filepath.ToSlash(rel),
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", prefix, err)
	}
	return out, nil
}

// Stat returns the file size and existence for a local storage key.
func (s *LocalStorage) Stat(ctx context.Context, key string) (int64, bool, error) {
	fi, err := os.Stat(s.fullPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("stat %s: %w", key, err)
	}
	return fi.Size(), true, nil
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

// NewMediaStorage selects the storage backend from environment (#1914):
//
//	STORAGE_MODE=local (default) → LocalStorage (现状)
//	STORAGE_MODE=dual            → DualStorage(OSS primary + local cold backup)
//	STORAGE_MODE=oss             → OSSStorage only
//
// dual/oss require OSS_ENDPOINT+OSS_BUCKET AND resolvable credentials;
// otherwise fall back to LocalStorage with a startup WARN (keeps local dev runnable).
var (
	ossStorageOnce   sync.Once
	ossStorageCached *OSSStorage
	ossStorageErr    error
)

// ossStorageFromEnv returns the cached OSS backend (built once).
func ossStorageFromEnv() (*OSSStorage, error) {
	ossStorageOnce.Do(func() {
		ossStorageCached, ossStorageErr = NewOSSStorage()
	})
	return ossStorageCached, ossStorageErr
}

func NewMediaStorage() MediaStorage {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_MODE")))
	if mode == "" {
		mode = "local"
	}
	if mode != "dual" && mode != "oss" {
		return NewLocalStorage()
	}
	if os.Getenv("OSS_ENDPOINT") == "" || os.Getenv("OSS_BUCKET") == "" {
		log.Printf("[MediaStorage] STORAGE_MODE=%s but OSS_ENDPOINT/OSS_BUCKET missing — falling back to LocalStorage", mode)
		return NewLocalStorage()
	}
	st, err := ossStorageFromEnv()
	if err != nil {
		log.Printf("[MediaStorage] OSS init failed (%v) — falling back to LocalStorage", err)
		return NewLocalStorage()
	}
	if mode == "oss" {
		return st
	}
	return NewDualStorage(st, NewLocalStorage())
}

// ResetMediaStorageCache clears the cached OSS instance (test-only helper).
func ResetMediaStorageCache() {
	ossStorageOnce = sync.Once{}
	ossStorageCached = nil
	ossStorageErr = nil
}

func MediaStorageFromContext(c *gin.Context) MediaStorage {
	if v, ok := c.Get("media_storage"); ok {
		if s, ok := v.(MediaStorage); ok {
			return s
		}
	}
	return NewMediaStorage()
}
