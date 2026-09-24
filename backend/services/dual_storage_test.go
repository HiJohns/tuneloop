package services

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBackend 内存 MediaStorage（双写单测）。
type fakeBackend struct {
	mu      sync.Mutex
	objects map[string][]byte
	failPut bool
}

func newFakeBackend() *fakeBackend { return &fakeBackend{objects: map[string][]byte{}} }

func (f *fakeBackend) Upload(_ context.Context, key string, r io.Reader, _ string) error {
	if f.failPut {
		return errors.New("put failed")
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = b
	return nil
}
func (f *fakeBackend) GetURL(_ context.Context, key string) (string, error) {
	return "/fake/" + key, nil
}
func (f *fakeBackend) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}
func (f *fakeBackend) DeletePrefix(_ context.Context, prefix string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			delete(f.objects, k)
		}
	}
	return nil
}
func (f *fakeBackend) Copy(_ context.Context, src, dst string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.objects[src]; ok {
		cp := make([]byte, len(b))
		copy(cp, b)
		f.objects[dst] = cp
	}
	return nil
}
func (f *fakeBackend) Rename(ctx context.Context, src, dst string) error {
	if err := f.Copy(ctx, src, dst); err != nil {
		return err
	}
	return f.Delete(ctx, src)
}
func (f *fakeBackend) List(_ context.Context, prefix string) ([]MediaObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []MediaObject
	for k, b := range f.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			out = append(out, MediaObject{Key: k, Size: int64(len(b))})
		}
	}
	return out, nil
}
func (f *fakeBackend) Stat(_ context.Context, key string) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[key]
	if !ok {
		return 0, false, nil
	}
	return int64(len(b)), true, nil
}

func chdirTemp(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestDualStorage_UploadWritesBoth(t *testing.T) {
	primary, local := newFakeBackend(), newFakeBackend()
	s := NewDualStorage(primary, local)

	require.NoError(t, s.Upload(context.Background(), "a.jpg", strings.NewReader("hello"), "image/jpeg"))
	require.Equal(t, "hello", string(primary.objects["a.jpg"]), "OSS 写入")
	require.Equal(t, "hello", string(local.objects["a.jpg"]), "本地冷备写入")
}

func TestDualStorage_OSSFailBlocksByDefault(t *testing.T) {
	primary, local := newFakeBackend(), newFakeBackend()
	primary.failPut = true
	s := NewDualStorage(primary, local) // OSS_FAIL_SOFT unset → 默认阻断

	err := s.Upload(context.Background(), "b.jpg", strings.NewReader("x"), "image/jpeg")
	require.Error(t, err)
	require.Contains(t, err.Error(), "oss upload")
	_, hasLocal := local.objects["b.jpg"]
	require.False(t, hasLocal, "OSS 失败时本地不留半态")
}

func TestDualStorage_OSSFailSoftWritesLocalAndLogs(t *testing.T) {
	chdirTemp(t)
	t.Setenv("OSS_FAIL_SOFT", "true")
	primary, local := newFakeBackend(), newFakeBackend()
	primary.failPut = true
	s := NewDualStorage(primary, local)

	require.NoError(t, s.Upload(context.Background(), "c.jpg", strings.NewReader("y"), "image/jpeg"))
	require.Equal(t, "y", string(local.objects["c.jpg"]), "soft 模式仍写本地")
	logBytes, err := os.ReadFile(filepath.Join("tmp", "oss_write_failures.log"))
	require.NoError(t, err)
	require.Contains(t, string(logBytes), "c.jpg", "失败清单留痕")
}

func TestDualStorage_DeleteAndPrefixBoth(t *testing.T) {
	primary, local := newFakeBackend(), newFakeBackend()
	primary.objects["k/1.jpg"] = []byte("1")
	local.objects["k/1.jpg"] = []byte("1")
	primary.objects["k/2.jpg"] = []byte("2")
	local.objects["k/2.jpg"] = []byte("2")
	s := NewDualStorage(primary, local)

	require.NoError(t, s.DeletePrefix(context.Background(), "k/"))
	require.Empty(t, primary.objects)
	require.Empty(t, local.objects, "两份都删")
}

func TestDualStorage_StatDelegatesPrimary(t *testing.T) {
	primary, local := newFakeBackend(), newFakeBackend()
	primary.objects["d.jpg"] = []byte("12345")
	s := NewDualStorage(primary, local)
	size, ok, err := s.Stat(context.Background(), "d.jpg")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(5), size)
}

func TestNewMediaStorage_ModeSelection(t *testing.T) {
	ResetMediaStorageCache()
	t.Cleanup(ResetMediaStorageCache)

	// 默认（未设置）→ LocalStorage
	t.Setenv("STORAGE_MODE", "")
	_, isLocal := NewMediaStorage().(*LocalStorage)
	require.True(t, isLocal, "缺省 local")

	// 未知值 → LocalStorage
	t.Setenv("STORAGE_MODE", "bogus")
	_, isLocal = NewMediaStorage().(*LocalStorage)
	require.True(t, isLocal)

	// STORAGE_MODE=oss 但缺 OSS 配置 → 回退 LocalStorage
	t.Setenv("STORAGE_MODE", "oss")
	t.Setenv("OSS_ENDPOINT", "")
	t.Setenv("OSS_BUCKET", "")
	_, isLocal = NewMediaStorage().(*LocalStorage)
	require.True(t, isLocal, "缺配置回退本地")

	// STORAGE_MODE=dual 但缺 OSS 配置 → 回退 LocalStorage
	t.Setenv("STORAGE_MODE", "dual")
	_, isLocal = NewMediaStorage().(*LocalStorage)
	require.True(t, isLocal, "dual 缺配置回退本地")
}

// closingBackend 模拟阿里云 OSS Go SDK 行为：对入参 io.ReadCloser 在上传结束后
// 执行 Close（PutObject/UploadPart 的 defer rc.Close()）。#2061 历史缺陷：
// DualStorage 直接把 spool 的 *os.File 交给 primary，被 SDK 关闭后本地冷备
// 重读失败（rewind spool: file already closed），dual 模式所有上传报错。
type closingBackend struct{ *fakeBackend }

func (b *closingBackend) Upload(_ context.Context, key string, r io.Reader, _ string) error {
	data, err := io.ReadAll(r)
	if rc, ok := r.(io.Closer); ok {
		_ = rc.Close() // 模拟 SDK：上传后关闭入参
	}
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[key] = data
	return nil
}

// TestDualStorage_2061_PrimaryClosesReader 回归：primary 关闭入参 reader 时，
// DualStorage.Upload 仍须成功且本地冷备写入完整内容（修复前此处返回 rewind spool 错误）。
func TestDualStorage_2061_PrimaryClosesReader(t *testing.T) {
	primary := &closingBackend{fakeBackend: newFakeBackend()}
	local := newFakeBackend()
	s := NewDualStorage(primary, local)

	require.NoError(t, s.Upload(context.Background(), "close.jpg", strings.NewReader("payload-2061"), "image/jpeg"))
	require.Equal(t, "payload-2061", string(primary.objects["close.jpg"]), "OSS 侧写入")
	require.Equal(t, "payload-2061", string(local.objects["close.jpg"]), "本地冷备写入（旧实现因 spool 被关闭而失败）")
}
