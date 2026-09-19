package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBucket records calls so routing/multipart logic is testable offline.
type fakeBucket struct {
	putKeys   []string
	multipart *fakeMultipart
}

type fakeMultipart struct {
	initiated []string
	parts     []oss.UploadPart
	completed []string
	aborted   []string
}

func (f *fakeBucket) PutObject(key string, r io.Reader, options ...oss.Option) error {
	f.putKeys = append(f.putKeys, key)
	return nil
}

func (f *fakeBucket) GetObject(key string, options ...oss.Option) (io.ReadCloser, error) {
	return nil, errors.New("not needed")
}

func (f *fakeBucket) DeleteObject(key string, options ...oss.Option) error { return nil }
func (f *fakeBucket) ListObjects(options ...oss.Option) (oss.ListObjectsResult, error) {
	return oss.ListObjectsResult{}, nil
}
func (f *fakeBucket) DeleteObjects(keys []string, options ...oss.Option) (oss.DeleteObjectsResult, error) {
	return oss.DeleteObjectsResult{}, nil
}
func (f *fakeBucket) CopyObject(src, dst string, options ...oss.Option) (oss.CopyObjectResult, error) {
	return oss.CopyObjectResult{}, nil
}
func (f *fakeBucket) SignURL(key string, method oss.HTTPMethod, expiredInSec int64, options ...oss.Option) (string, error) {
	return fmt.Sprintf("https://signed.example.com/%s?signature=fake&ttl=%d", key, expiredInSec), nil
}

func (f *fakeBucket) InitiateMultipartUpload(key string, options ...oss.Option) (oss.InitiateMultipartUploadResult, error) {
	if f.multipart == nil {
		f.multipart = &fakeMultipart{}
	}
	f.multipart.initiated = append(f.multipart.initiated, key)
	return oss.InitiateMultipartUploadResult{Key: key}, nil
}

func (f *fakeBucket) UploadPart(imur oss.InitiateMultipartUploadResult, reader io.Reader, partSize int64, partNumber int, options ...oss.Option) (oss.UploadPart, error) {
	f.multipart.parts = append(f.multipart.parts, oss.UploadPart{PartNumber: partNumber})
	return oss.UploadPart{PartNumber: partNumber}, nil
}

func (f *fakeBucket) CompleteMultipartUpload(imur oss.InitiateMultipartUploadResult, parts []oss.UploadPart, options ...oss.Option) (oss.CompleteMultipartUploadResult, error) {
	f.multipart.completed = append(f.multipart.completed, imur.Key)
	return oss.CompleteMultipartUploadResult{}, nil
}

func (f *fakeBucket) AbortMultipartUpload(imur oss.InitiateMultipartUploadResult, options ...oss.Option) error {
	f.multipart.aborted = append(f.multipart.aborted, imur.Key)
	return nil
}

func newTestOSS(private bool) (*OSSStorage, *fakeBucket, *fakeBucket) {
	pub := &fakeBucket{}
	cfg := OSSStorageConfig{Endpoint: "oss-cn-beijing.aliyuncs.com", Bucket: "tuneloop-media", SignedURLTTL: 900}
	s := &OSSStorage{cfg: cfg, bucket: pub}
	priv := (*fakeBucket)(nil)
	if private {
		priv = &fakeBucket{}
		s.privateBucket = priv
		s.cfg.PrivateBucket = "tuneloop-media-sec"
	}
	return s, pub, priv
}

// Public assets use plain PutObject; small size regardless.
func TestUploadSmallEntry(t *testing.T) {
	s, pub, _ := newTestOSS(false)
	r := strings.NewReader("hello")
	require.NoError(t, s.Upload(context.Background(), "id_photos/a.jpg", r, "image/jpeg"))
	require.Equal(t, []string{"id_photos/a.jpg"}, pub.putKeys)
}

// Private keys route to the private bucket.
func TestUploadRoutedToPrivateBucket(t *testing.T) {
	s, pub, priv := newTestOSS(true)
	require.NoError(t, s.Upload(context.Background(), "face_captures/u1/b1/x.jpg", strings.NewReader("p"), "image/jpeg"))
	require.Empty(t, pub.putKeys, "private key must NOT touch public bucket")
	require.Equal(t, []string{"face_captures/u1/b1/x.jpg"}, priv.putKeys)
}

// Large seekable payloads go through multipart with size-appropriate parts.
func TestUploadLargeUsesMultipart(t *testing.T) {
	s, pub, _ := newTestOSS(false)
	size := int64(multipartThresholdBytes) + 3 // 64MB + 3
	big := &staticSeeker{data: make([]byte, size), size: size}
	require.NoError(t, s.Upload(context.Background(), "videos/big.mp4", big, "video/mp4"))
	require.Empty(t, pub.putKeys, "large payload must not use PutObject")
	require.Equal(t, 1, len(pub.multipart.initiated))
	require.Equal(t, 1, len(pub.multipart.completed))
	require.Empty(t, pub.multipart.aborted)
	// 64MB + 3 = 9 parts of 8MB.
	require.Equal(t, 9, len(pub.multipart.parts))
}

// Non-seekable readers fall back to PutObject.
func TestUploadNonSeekerFallsBack(t *testing.T) {
	s, pub, _ := newTestOSS(false)
	require.NoError(t, s.Upload(context.Background(), "x.bin", &noSeekReader{data: make([]byte, 100)}, "application/octet-stream"))
	require.Equal(t, []string{"x.bin"}, pub.putKeys)
}

// GetURL: public via CDN prefix when configured, bucket endpoint otherwise;
// private via signed URL on the private bucket.
func TestGetURLComposition(t *testing.T) {
	s, _, _ := newTestOSS(false)
	u, err := s.GetURL(context.Background(), "media/x.jpg")
	require.NoError(t, err)
	assert.Equal(t, "https://tuneloop-media.oss-cn-beijing.aliyuncs.com/media/x.jpg", u)

	s.cfg.CDNPrefix = "https://img.cadenzayueqi.com"
	u, err = s.GetURL(context.Background(), "media/x.jpg")
	require.NoError(t, err)
	assert.Equal(t, "https://img.cadenzayueqi.com/media/x.jpg", u)

	// private requires a real bucket for signing — verify routing decision
	// (isPrivate) instead of the network call.
	s2, _, _ := newTestOSS(true)
	assert.True(t, s2.isPrivate("face_captures/u/v.jpg"))
	assert.False(t, s2.isPrivate("media/x.jpg"))

	// #1993: private key → signed URL (TTL from config, never the public path).
	su, err := s2.GetURL(context.Background(), "face_captures/u/v.jpg")
	require.NoError(t, err)
	assert.Contains(t, su, "face_captures/u/v.jpg")
	assert.Contains(t, su, "signature=fake")
	assert.Contains(t, su, "ttl=900")
	assert.NotContains(t, su, "tuneloop-media.oss", "私有不得返回公开 bucket URL")
}

// LocalStorage path untouched — GetURL keeps the /uploads/media prefix.
func TestLocalStorageGetURL(t *testing.T) {
	ls := NewLocalStorage()
	u, err := ls.GetURL(context.Background(), "a/b.jpg")
	require.NoError(t, err)
	assert.Equal(t, "/uploads/media/a/b.jpg", u)
}

// staticSeeker implements io.ReadSeeker returning fixed-size content.
type staticSeeker struct {
	data []byte
	size int64
	offs int64
}

func (s *staticSeeker) Read(p []byte) (int, error) {
	if s.offs >= s.size {
		return 0, io.EOF
	}
	n := len(p)
	if int64(n) > s.size-s.offs {
		n = int(s.size - s.offs)
	}
	for i := 0; i < n; i++ {
		p[i] = 0
	}
	s.offs += int64(n)
	return n, nil
}

func (s *staticSeeker) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		s.offs = offset
	case io.SeekCurrent:
		s.offs += offset
	case io.SeekEnd:
		s.offs = s.size + offset
	}
	if s.offs < 0 {
		s.offs = 0
	}
	return s.offs, nil
}

type noSeekReader struct {
	data []byte
}

func (r *noSeekReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

var _ = oss.HTTPGet
