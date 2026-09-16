package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// OSSStorage implements MediaStorage on Alibaba Cloud OSS (#1914).
//
// Key semantics unchanged: storage keys are identical to LocalStorage keys
// (no DB migration); only the backend and GetURL output differ.
// Sensitive material (face_captures/ prefix) is routed to a private bucket
// and returned via signed URLs.

const (
	privateKeyPrefix        = "face_captures/"
	multipartThresholdBytes = 64 << 20 // >= 64MB goes multipart
	multipartPartSizeBytes  = 8 << 20  // 8MB per part
	defaultSignedURLTTL     = 900      // seconds (15 min)
)

// OSSStorageConfig mirrors docs/oss.md §4.
type OSSStorageConfig struct {
	Endpoint         string
	Region           string
	Bucket           string
	PrivateBucket    string
	CDNPrefix        string
	PrivateCDNPrefix string
	SignedURLTTL     int64
}

// ossBucket is the small seam over *oss.Bucket used by OSSStorage so the
// routing/multipart logic is unit-testable with a fake.
type ossBucket interface {
	PutObject(key string, r io.Reader, options ...oss.Option) error
	GetObject(key string, options ...oss.Option) (io.ReadCloser, error)
	DeleteObject(key string, options ...oss.Option) error
	ListObjects(options ...oss.Option) (oss.ListObjectsResult, error)
	DeleteObjects(keys []string, options ...oss.Option) (oss.DeleteObjectsResult, error)
	CopyObject(src, dst string, options ...oss.Option) (oss.CopyObjectResult, error)
	SignURL(key string, method oss.HTTPMethod, expiredInSec int64, options ...oss.Option) (string, error)
	InitiateMultipartUpload(key string, options ...oss.Option) (oss.InitiateMultipartUploadResult, error)
	UploadPart(imur oss.InitiateMultipartUploadResult, reader io.Reader, partSize int64, partNumber int, options ...oss.Option) (oss.UploadPart, error)
	CompleteMultipartUpload(imur oss.InitiateMultipartUploadResult, parts []oss.UploadPart, options ...oss.Option) (oss.CompleteMultipartUploadResult, error)
	AbortMultipartUpload(imur oss.InitiateMultipartUploadResult, options ...oss.Option) error
}

// compile-time interface conformance
var _ MediaStorage = (*OSSStorage)(nil)

type OSSStorage struct {
	cfg           OSSStorageConfig
	bucket        ossBucket
	privateBucket ossBucket
}

// NewOSSStorage builds the OSS-backed storage from environment config.
// Errors when OSS_ENDPOINT/OSS_BUCKET missing or no credentials available.
func NewOSSStorage() (*OSSStorage, error) {
	cfg := OSSStorageConfig{
		Endpoint:         os.Getenv("OSS_ENDPOINT"),
		Region:           os.Getenv("OSS_REGION"),
		Bucket:           os.Getenv("OSS_BUCKET"),
		PrivateBucket:    os.Getenv("OSS_PRIVATE_BUCKET"),
		CDNPrefix:        strings.TrimRight(os.Getenv("OSS_CDN_PREFIX"), "/"),
		PrivateCDNPrefix: strings.TrimRight(os.Getenv("OSS_PRIVATE_CDN_PREFIX"), "/"),
		SignedURLTTL:     envInt64("OSS_SIGNED_URL_TTL", defaultSignedURLTTL),
	}
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, fmt.Errorf("OSS_ENDPOINT and OSS_BUCKET are required")
	}
	if cfg.SignedURLTTL <= 0 {
		cfg.SignedURLTTL = defaultSignedURLTTL
	}

	provider, err := resolveCredentialsProvider()
	if err != nil {
		return nil, fmt.Errorf("resolve OSS credentials: %w", err)
	}
	// #1914: buckets enforce HTTPS (plain HTTP returns a misleading 403
	// "bucket acl"); the SDK defaults to HTTP when the endpoint carries no
	// scheme — force https.
	client, err := oss.New(normalizeEndpoint(cfg.Endpoint), "", "", oss.SetCredentialsProvider(provider))
	if err != nil {
		return nil, fmt.Errorf("create oss client: %w", err)
	}

	b, err := client.Bucket(cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket %s: %w", cfg.Bucket, err)
	}
	s := &OSSStorage{cfg: cfg, bucket: b}
	if cfg.PrivateBucket != "" && cfg.PrivateBucket != cfg.Bucket {
		pb, err := client.Bucket(cfg.PrivateBucket)
		if err != nil {
			return nil, fmt.Errorf("bucket %s: %w", cfg.PrivateBucket, err)
		}
		s.privateBucket = pb
	}
	return s, nil
}

func (s *OSSStorage) isPrivate(key string) bool {
	return s.privateBucket != nil && strings.HasPrefix(key, privateKeyPrefix)
}

func (s *OSSStorage) target(key string) ossBucket {
	if s.isPrivate(key) {
		return s.privateBucket
	}
	return s.bucket
}

// Upload stores content at key. Large payloads (>= multipartThresholdBytes)
// are uploaded in parts so big videos never hit PutObject timeouts.
func (s *OSSStorage) Upload(ctx context.Context, key string, reader io.Reader, contentType string) error {
	if ctx == nil {
		return fmt.Errorf("nil context")
	}

	// Best-effort size probe — multipart.File / *os.File / *bytes.Reader all
	// implement io.Seeker; unknown-size readers fall back to PutObject.
	size := int64(-1)
	if seeker, ok := reader.(io.Seeker); ok {
		if cur, err := seeker.Seek(0, io.SeekCurrent); err == nil {
			if end, err := seeker.Seek(0, io.SeekEnd); err == nil {
				size = end - cur
				if _, err := seeker.Seek(cur, io.SeekStart); err != nil {
					return fmt.Errorf("rewind reader: %w", err)
				}
			}
		}
	}

	if size >= multipartThresholdBytes {
		return s.uploadMultipart(key, reader, size, contentType)
	}
	return s.target(key).PutObject(key, reader, oss.ContentType(contentType))
}

// uploadMultipart uploads a known-size reader in 8MB parts (multipart
// threshold 64MB). On any part failure the in-flight upload is aborted.
func (s *OSSStorage) uploadMultipart(key string, reader io.Reader, size int64, contentType string) error {
	b := s.target(key)

	imur, err := b.InitiateMultipartUpload(key, oss.ContentType(contentType))
	if err != nil {
		return fmt.Errorf("initiate multipart upload %s: %w", key, err)
	}

	const partBytes = int64(multipartPartSizeBytes)
	parts := make([]oss.UploadPart, 0, (size+partBytes-1)/partBytes)

	var remaining = size
	partNumber := 1
	for remaining > 0 {
		partSize := partBytes
		if remaining < partBytes {
			partSize = remaining
		}
		part, err := b.UploadPart(imur, reader, partSize, partNumber, oss.ContentType(contentType))
		if err != nil {
			_ = b.AbortMultipartUpload(imur)
			return fmt.Errorf("upload part %d of %s: %w", partNumber, key, err)
		}
		parts = append(parts, part)
		remaining -= partSize
		partNumber++
	}

	if _, err := b.CompleteMultipartUpload(imur, parts, oss.ContentType(contentType)); err != nil {
		_ = b.AbortMultipartUpload(imur)
		return fmt.Errorf("complete multipart upload %s: %w", key, err)
	}
	return nil
}

// GetURL returns a CDN/public URL for business assets and a signed URL for
// private assets (face_captures).
func (s *OSSStorage) GetURL(ctx context.Context, key string) (string, error) {
	if s.isPrivate(key) {
		url, err := s.privateBucket.SignURL(key, oss.HTTPGet, s.cfg.SignedURLTTL)
		if err != nil {
			return "", fmt.Errorf("sign url for %s: %w", key, err)
		}
		return url, nil
	}
	return s.publicURL(key), nil
}

func (s *OSSStorage) publicURL(key string) string {
	if s.cfg.CDNPrefix != "" {
		return s.cfg.CDNPrefix + "/" + key
	}
	// https://<bucket>.<endpoint>/<key> — external endpoint only (docs/oss.md).
	ep := strings.TrimPrefix(s.cfg.Endpoint, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return "https://" + s.cfg.Bucket + "." + ep + "/" + key
}

func (s *OSSStorage) Delete(ctx context.Context, key string) error {
	if err := s.target(key).DeleteObject(key); err != nil {
		return fmt.Errorf("delete %s: %w", key, err)
	}
	return nil
}

func (s *OSSStorage) DeletePrefix(ctx context.Context, prefix string) error {
	b := s.target(prefix)
	marker := ""
	for {
		opts := []oss.Option{oss.Prefix(prefix), oss.MaxKeys(1000)}
		if marker != "" {
			opts = append(opts, oss.Marker(marker))
		}
		res, err := b.ListObjects(opts...)
		if err != nil {
			return fmt.Errorf("list %q: %w", prefix, err)
		}
		if len(res.Objects) > 0 {
			keys := make([]string, 0, len(res.Objects))
			for _, o := range res.Objects {
				keys = append(keys, o.Key)
			}
			if _, err := b.DeleteObjects(keys); err != nil {
				return fmt.Errorf("delete batch under %q: %w", prefix, err)
			}
		}
		if !res.IsTruncated {
			return nil
		}
		if res.NextMarker != "" {
			marker = res.NextMarker
			continue
		}
		if len(res.Objects) > 0 {
			marker = res.Objects[len(res.Objects)-1].Key
			continue
		}
		return fmt.Errorf("list %q truncated without objects or marker", prefix)
	}
}

func (s *OSSStorage) Copy(ctx context.Context, srcKey, dstKey string) error {
	if s.isPrivate(srcKey) != s.isPrivate(dstKey) {
		return fmt.Errorf("cross-bucket copy not supported: %s -> %s", srcKey, dstKey)
	}
	if _, err := s.target(srcKey).CopyObject(srcKey, dstKey); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", srcKey, dstKey, err)
	}
	return nil
}

func (s *OSSStorage) Rename(ctx context.Context, srcKey, dstKey string) error {
	if err := s.Copy(ctx, srcKey, dstKey); err != nil {
		return err
	}
	return s.Delete(ctx, srcKey)
}

func envInt64(name string, def int64) int64 {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
			return n
		}
	}
	return def
}

// normalizeEndpoint forces an https scheme (SDK defaults to plain HTTP when
// no scheme is present, and the buckets reject plain HTTP with a misleading
// "bucket acl" 403 — #1914 smoke finding).
func normalizeEndpoint(endpoint string) string {
	ep := strings.TrimSpace(endpoint)
	if ep == "" {
		return ep
	}
	if strings.Contains(ep, "://") {
		return ep
	}
	return "https://" + ep
}
