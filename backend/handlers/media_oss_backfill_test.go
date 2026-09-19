package handlers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeMediaStorage 内存实现 MediaStorage（#1914 P4 回填单测）。
type fakeMediaStorage struct {
	mu         sync.Mutex
	objects    map[string][]byte
	uploadCnt  int
	failUpload map[string]error
}

func newFakeMediaStorage() *fakeMediaStorage {
	return &fakeMediaStorage{objects: map[string][]byte{}, failUpload: map[string]error{}}
}

func (f *fakeMediaStorage) Upload(_ context.Context, key string, r io.Reader, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failUpload[key]; err != nil {
		return err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.objects[key] = b
	f.uploadCnt++
	return nil
}

func (f *fakeMediaStorage) Stat(_ context.Context, key string) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return 0, false, nil
	}
	return int64(len(b)), true, nil
}

func (f *fakeMediaStorage) GetURL(_ context.Context, key string) (string, error) {
	return "/fake/" + key, nil
}
func (f *fakeMediaStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}
func (f *fakeMediaStorage) DeletePrefix(_ context.Context, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.objects {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(f.objects, k)
		}
	}
	return nil
}
func (f *fakeMediaStorage) Copy(_ context.Context, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.objects[src]; ok {
		cp := make([]byte, len(b))
		copy(cp, b)
		f.objects[dst] = cp
	}
	return nil
}
func (f *fakeMediaStorage) Rename(ctx context.Context, src, dst string) error {
	if err := f.Copy(ctx, src, dst); err != nil {
		return err
	}
	return f.Delete(ctx, src)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func TestMediaOSSBackfill_IdempotentAndVariants(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "instruments", "a.jpg"), "AAA")
	writeFile(t, filepath.Join(root, "instruments", "a_display.webp"), "AAAAA")
	writeFile(t, filepath.Join(root, "instruments", "a_thumb.jpg"), "AA")

	storage := newFakeMediaStorage()
	roots := []backfillRoot{{Dir: root}}
	opts := MediaOSSBackfillOptions{Concurrency: 2}

	stats, failures, _, err := runMediaBackfill(context.Background(), storage, opts, roots, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 3, stats.Uploaded)
	require.Equal(t, 3, stats.Scanned)
	require.Empty(t, failures)
	require.Equal(t, 3, storage.uploadCnt)

	// 二次执行：size 一致 → 全部 skip，0 新上传（幂等）。
	stats2, failures2, _, err := runMediaBackfill(context.Background(), storage, opts, roots, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 3, stats2.Skipped)
	require.Equal(t, 0, stats2.Uploaded)
	require.Empty(t, failures2)
	require.Equal(t, 3, storage.uploadCnt, "第二次不应产生上传")
}

func TestMediaOSSBackfill_SizeMismatchAndOverwrite(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "b.jpg"), "BBBB")
	storage := newFakeMediaStorage()
	storage.objects["b.jpg"] = []byte("B") // 已存在但 size 不一致
	roots := []backfillRoot{{Dir: root}}

	// 默认：报错跳过（不覆盖）。
	stats, failures, _, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1}, roots, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, stats.Failed)
	require.Len(t, failures, 1)
	require.Contains(t, failures[0].Err, "size mismatch")

	// --oss-overwrite：覆盖。
	stats2, failures2, _, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1, Overwrite: true}, roots, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, stats2.Overwritten)
	require.Empty(t, failures2)
	require.Equal(t, "BBBB", string(storage.objects["b.jpg"]))
}

func TestMediaOSSBackfill_ResumeOnlyListedKeys(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "x.jpg"), "X")
	writeFile(t, filepath.Join(root, "y.jpg"), "Y")
	resume := filepath.Join(t.TempDir(), "failures.jsonl")
	require.NoError(t, os.WriteFile(resume, []byte(`{"key":"y.jpg","size":1,"error":"boom"}`+"\n"), 0644))

	storage := newFakeMediaStorage()
	stats, _, _, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1, ResumeFile: resume}, []backfillRoot{{Dir: root}}, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, stats.Scanned)
	require.Equal(t, 1, stats.Uploaded)
	_, hasX := storage.objects["x.jpg"]
	_, hasY := storage.objects["y.jpg"]
	require.False(t, hasX, "resume 清单外的 x.jpg 不应上传")
	require.True(t, hasY)
}

func TestMediaOSSBackfill_ReferencedMissingLocallyAndOrphans(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "kept.jpg"), "K")
	writeFile(t, filepath.Join(root, "orphan.jpg"), "O")
	referenced := map[string]bool{"kept.jpg": true, "ghost.jpg": true} // ghost 无本地文件

	storage := newFakeMediaStorage()
	stats, _, missing, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1}, []backfillRoot{{Dir: root}}, referenced)
	require.NoError(t, err)
	require.Equal(t, 1, stats.Orphans, "orphan.jpg 未被引用")
	require.Equal(t, []string{"ghost.jpg"}, missing, "引用但本地缺失")
}

func TestMediaOSSBackfill_DryRunNoWrites(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "z.jpg"), "ZZ")
	storage := newFakeMediaStorage()
	stats, _, _, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1, DryRun: true}, []backfillRoot{{Dir: root}}, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, stats.Uploaded, "dry-run 报告 would-upload")
	require.Equal(t, 0, storage.uploadCnt, "dry-run 不写")
}

func TestMediaOSSBackfill_UploadFailureRecorded(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "fail.jpg"), "F")
	storage := newFakeMediaStorage()
	storage.failUpload["fail.jpg"] = fmt.Errorf("network down")

	stats, failures, _, err := runMediaBackfill(context.Background(), storage, MediaOSSBackfillOptions{Concurrency: 1}, []backfillRoot{{Dir: root}}, map[string]bool{})
	require.NoError(t, err)
	require.Equal(t, 1, stats.Failed)
	require.Len(t, failures, 1)
	require.Contains(t, failures[0].Err, "network down")
}
