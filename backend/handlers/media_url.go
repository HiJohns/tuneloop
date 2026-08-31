package handlers

import (
	"context"
	"strings"

	"tuneloop-backend/services"
)

const mediaURLPrefix = "/uploads/media/"

// resolveMediaURL 将存储 key（或历史 URL 值）转为可访问 URL（单前缀）。
// 统一入口：user_management.go / user_staff.go 共用，杜绝双前缀 404。
// normalizeMediaKey 见 public.go（循环去前缀，兼容双前缀脏数据）。
func resolveMediaURL(ctx context.Context, key *string) string {
	if key == nil || *key == "" {
		return ""
	}
	k := *key
	if strings.HasPrefix(k, "http://") || strings.HasPrefix(k, "https://") {
		return k
	}
	if strings.HasPrefix(k, mediaURLPrefix) {
		return mediaURLPrefix + normalizeMediaKey(k)
	}
	storage := services.NewMediaStorage()
	url, err := storage.GetURL(ctx, k)
	if err != nil || url == "" {
		return mediaURLPrefix + k
	}
	return url
}
