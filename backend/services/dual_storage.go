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
)

// DualStorage writes to OSS (primary) + LocalStorage (cold backup) during the
// OSS transition (#1914 P3). Reads delegate to the primary (OSS) side.
//
// Failure policy: OSS write failure aborts by default (explicit error, never
// silent — red line); OSS_FAIL_SOFT=true degrades to a logged warning plus an
// append-only failure list (tmp/oss_write_failures.log). Local backup failures
// only warn (local is cold backup, not the source of truth).
type DualStorage struct {
	primary  MediaStorage
	local    MediaStorage
	failSoft bool
	failMu   sync.Mutex
}

// NewDualStorage composes the OSS primary with the local cold backup.
func NewDualStorage(primary, local MediaStorage) *DualStorage {
	return &DualStorage{
		primary:  primary,
		local:    local,
		failSoft: isTruthyEnv("OSS_FAIL_SOFT"),
	}
}

func isTruthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// spoolToTemp buffers the reader so both backends consume independent streams
// (readers are single-shot; large videos must not be re-read from the socket).
func spoolToTemp(reader io.Reader) (*os.File, error) {
	if err := os.MkdirAll("tmp", 0755); err == nil {
		if f, err := os.CreateTemp("tmp", "dual-upload-*"); err == nil {
			if _, cerr := io.Copy(f, reader); cerr != nil {
				f.Close()
				os.Remove(f.Name())
				return nil, fmt.Errorf("spool upload: %w", cerr)
			}
			return f, nil
		}
	}
	// Fallback to system temp when the project tmp dir is unavailable.
	f, err := os.CreateTemp("", "dual-upload-*")
	if err != nil {
		return nil, fmt.Errorf("create spool: %w", err)
	}
	if _, cerr := io.Copy(f, reader); cerr != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, fmt.Errorf("spool upload: %w", cerr)
	}
	return f, nil
}

func (s *DualStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	f, err := spoolToTemp(reader)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()

	// OSS first: default abort must not leave a local half-state.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind spool: %w", err)
	}
	if err := s.primary.Upload(ctx, key, f, contentType); err != nil {
		if !s.failSoft {
			return fmt.Errorf("oss upload %s: %w", key, err)
		}
		s.recordFailure(key, err)
		log.Printf("[DualStorage] OSS upload failed (soft) key=%s: %v", key, err)
	}

	// Local cold backup: failure only warns.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind spool: %w", err)
	}
	if err := s.local.Upload(ctx, key, f, contentType); err != nil {
		log.Printf("[DualStorage] local backup write failed key=%s: %v", key, err)
	}
	return nil
}

func (s *DualStorage) recordFailure(key string, cause error) {
	s.failMu.Lock()
	defer s.failMu.Unlock()
	if err := os.MkdirAll("tmp", 0755); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join("tmp", "oss_write_failures.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%s\n", time.Now().Format(time.RFC3339), key, cause)
}

// GetURL delegates to the read side (OSS in dual mode; see #1993).
func (s *DualStorage) GetURL(ctx context.Context, key string) (string, error) {
	return s.primary.GetURL(ctx, key)
}

// Stat reports the primary (OSS) object state — the authoritative read side.
func (s *DualStorage) Stat(ctx context.Context, key string) (int64, bool, error) {
	return s.primary.Stat(ctx, key)
}

func (s *DualStorage) Delete(ctx context.Context, key string) error {
	return s.both(key, func(m MediaStorage) error { return m.Delete(ctx, key) })
}

func (s *DualStorage) DeletePrefix(ctx context.Context, prefix string) error {
	return s.both(prefix, func(m MediaStorage) error { return m.DeletePrefix(ctx, prefix) })
}

func (s *DualStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	return s.both(srcKey, func(m MediaStorage) error { return m.Copy(ctx, srcKey, dstKey) })
}

func (s *DualStorage) Rename(ctx context.Context, srcKey, dstKey string) error {
	return s.both(srcKey, func(m MediaStorage) error { return m.Rename(ctx, srcKey, dstKey) })
}

// both applies op to primary then local; failures are aggregated (not aborted)
// so one side's transient error never masks the other.
func (s *DualStorage) both(label string, op func(MediaStorage) error) error {
	var errs []string
	if err := op(s.primary); err != nil {
		errs = append(errs, fmt.Sprintf("primary: %v", err))
	}
	if err := op(s.local); err != nil {
		errs = append(errs, fmt.Sprintf("local: %v", err))
	}
	if len(errs) > 0 {
		return fmt.Errorf("dual op %s failed: %s", label, strings.Join(errs, "; "))
	}
	return nil
}
