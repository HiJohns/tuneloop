package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"tuneloop-backend/database"
	"tuneloop-backend/models"
	"tuneloop-backend/services"
)

// #1914 P4: 历史媒体回填 CLI（--migrate-media-oss）。
// 全量扫描本地 uploads/ 经 MediaStorage 幂等上传 OSS；不改 DB、不改 storage_key 语义。

// MediaOSSBackfillOptions 控制回填行为。
type MediaOSSBackfillOptions struct {
	DryRun      bool
	Concurrency int
	ResumeFile  string // 仅处理该失败清单（JSONL）内的 key
	Overwrite   bool   // OSS 已存在但 size 不一致时是否覆盖（默认报错跳过）
}

type mediaOSSStats struct {
	Scanned     int
	Uploaded    int
	Overwritten int
	Skipped     int
	Failed      int
	Orphans     int
	Bytes       int64
}

type mediaOSSCandidate struct {
	Key  string
	Path string
	Size int64
}

type mediaOSSFailure struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	Err  string `json:"error"`
	At   string `json:"at"`
}

type backfillRoot struct {
	Dir    string
	Prefix string
}

// MigrateMediaOSS CLI 入口：连接真实 MediaStorage/DB 后执行回填。
func MigrateMediaOSS(opts MediaOSSBackfillOptions) error {
	ctx := context.Background()
	storage := services.NewMediaStorage()
	roots := []backfillRoot{
		{"./uploads/media", ""},
		{"./uploads/batch", "batch/"},
	}
	stats, failures, missingLocally, err := runMediaBackfill(ctx, storage, opts, roots, loadReferencedMediaKeys())
	if err != nil {
		return err
	}
	if !opts.DryRun && len(failures) > 0 {
		if werr := writeBackfillFailures(failures); werr != nil {
			log.Printf("[MediaOSSBackfill] write failure list failed: %v", werr)
		}
	}
	report := buildBackfillReport(opts, stats, missingLocally)
	fmt.Print(report)
	if merr := os.MkdirAll("tmp", 0755); merr == nil {
		_ = os.WriteFile("tmp/oss_backfill_report.txt", []byte(report), 0644)
	}
	return nil
}

// runMediaBackfill 是回填核心（依赖注入，便于单测）。
// 返回 (stats, failures, media_assets 引用但本地缺失的 key 列表, 致命错误)。
func runMediaBackfill(ctx context.Context, storage services.MediaStorage, opts MediaOSSBackfillOptions, roots []backfillRoot, referenced map[string]bool) (mediaOSSStats, []mediaOSSFailure, []string, error) {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}

	candidates, err := collectCandidates(roots)
	if err != nil {
		return mediaOSSStats{}, nil, nil, err
	}

	if opts.ResumeFile != "" {
		want, rerr := loadBackfillKeys(opts.ResumeFile)
		if rerr != nil {
			return mediaOSSStats{}, nil, nil, fmt.Errorf("load resume file: %w", rerr)
		}
		filtered := candidates[:0]
		for _, c := range candidates {
			if want[c.Key] {
				filtered = append(filtered, c)
			}
		}
		candidates = filtered
	}

	candidateKeys := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		candidateKeys[c.Key] = true
	}
	missingLocally := make([]string, 0)
	for k := range referenced {
		if !candidateKeys[k] {
			missingLocally = append(missingLocally, k)
		}
	}
	sort.Strings(missingLocally)
	orphans := 0
	for _, c := range candidates {
		if !referenced[c.Key] {
			orphans++
		}
	}

	stats := mediaOSSStats{Scanned: len(candidates), Orphans: orphans}
	failures := make([]mediaOSSFailure, 0)
	var mu sync.Mutex

	jobs := make(chan mediaOSSCandidate)
	var wg sync.WaitGroup
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				rec, bytes, ferr := backfillOne(ctx, storage, c, opts)
				mu.Lock()
				stats.Bytes += bytes
				switch rec {
				case "uploaded", "dry-uploaded":
					stats.Uploaded++
				case "overwritten", "dry-overwritten":
					stats.Overwritten++
				case "skipped", "dry-skipped":
					stats.Skipped++
				}
				if ferr != nil {
					stats.Failed++
					failures = append(failures, mediaOSSFailure{Key: c.Key, Size: c.Size, Err: ferr.Error(), At: time.Now().Format(time.RFC3339)})
				}
				mu.Unlock()
			}
		}()
	}
	for _, c := range candidates {
		jobs <- c
	}
	close(jobs)
	wg.Wait()
	sort.Slice(failures, func(i, j int) bool { return failures[i].Key < failures[j].Key })
	return stats, failures, missingLocally, nil
}

func collectCandidates(roots []backfillRoot) ([]mediaOSSCandidate, error) {
	var candidates []mediaOSSCandidate
	for _, r := range roots {
		if _, err := os.Stat(r.Dir); os.IsNotExist(err) {
			continue
		}
		if err := filepath.Walk(r.Dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rel, rerr := filepath.Rel(r.Dir, path)
			if rerr != nil {
				return rerr
			}
			candidates = append(candidates, mediaOSSCandidate{
				Key:  r.Prefix + filepath.ToSlash(rel),
				Path: path,
				Size: info.Size(),
			})
			return nil
		}); err != nil {
			return nil, fmt.Errorf("scan %s: %w", r.Dir, err)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Key < candidates[j].Key })
	return candidates, nil
}

func backfillOne(ctx context.Context, storage services.MediaStorage, c mediaOSSCandidate, opts MediaOSSBackfillOptions) (string, int64, error) {
	ossSize, exists, err := storage.Stat(ctx, c.Key)
	if err != nil {
		return "", 0, err
	}

	if exists && ossSize == c.Size {
		if opts.DryRun {
			return "dry-skipped", 0, nil
		}
		return "skipped", 0, nil
	}
	if exists && ossSize != c.Size && !opts.Overwrite {
		return "", 0, fmt.Errorf("size mismatch (local=%d oss=%d); use --oss-overwrite to replace", c.Size, ossSize)
	}
	action := "uploaded"
	if exists {
		action = "overwritten"
	}
	if opts.DryRun {
		return "dry-" + action, 0, nil
	}

	f, err := os.Open(c.Path)
	if err != nil {
		return "", 0, fmt.Errorf("open %s: %w", c.Path, err)
	}
	defer f.Close()

	if err := storage.Upload(ctx, c.Key, f, contentTypeForKey(c.Key)); err != nil {
		return "", 0, fmt.Errorf("upload: %w", err)
	}
	// 上传后校验 size（ETag 因分片不稳定，不作主判据）。
	got, ok, err := storage.Stat(ctx, c.Key)
	if err != nil {
		return "", 0, fmt.Errorf("verify stat: %w", err)
	}
	if !ok || got != c.Size {
		return "", 0, fmt.Errorf("verify failed: expected %d, got exists=%v size=%d", c.Size, ok, got)
	}
	return action, c.Size, nil
}

func contentTypeForKey(key string) string {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(key))); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

// loadReferencedMediaKeys 读取 media_assets 中被引用的 storage_key 集合。
func loadReferencedMediaKeys() map[string]bool {
	out := map[string]bool{}
	db := database.GetDB()
	if db == nil {
		return out
	}
	var keys []string
	if err := db.Model(&models.MediaAsset{}).Where("is_referenced = ?", true).Pluck("storage_key", &keys).Error; err != nil {
		log.Printf("[MediaOSSBackfill] load media_assets failed: %v", err)
		return out
	}
	for _, k := range keys {
		if k != "" {
			out[k] = true
		}
	}
	return out
}

func loadBackfillKeys(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	want := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec mediaOSSFailure
		if err := json.Unmarshal([]byte(line), &rec); err == nil && rec.Key != "" {
			want[rec.Key] = true
			continue
		}
		// 兼容纯 key 行。
		want[line] = true
	}
	return want, sc.Err()
}

func writeBackfillFailures(failures []mediaOSSFailure) error {
	if err := os.MkdirAll("tmp", 0755); err != nil {
		return err
	}
	f, err := os.OpenFile("tmp/oss_backfill_failures.jsonl", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, rec := range failures {
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

func buildBackfillReport(opts MediaOSSBackfillOptions, s mediaOSSStats, missingLocally []string) string {
	mode := "REAL"
	if opts.DryRun {
		mode = "DRY-RUN"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "== media OSS backfill (%s) ==\n", mode)
	fmt.Fprintf(&b, "scanned=%d uploaded=%d overwritten=%d skipped=%d failed=%d orphans=%d bytes=%d\n",
		s.Scanned, s.Uploaded, s.Overwritten, s.Skipped, s.Failed, s.Orphans, s.Bytes)
	fmt.Fprintf(&b, "media_assets referenced keys missing locally: %d\n", len(missingLocally))
	for _, k := range missingLocally {
		fmt.Fprintf(&b, "  MISSING_LOCAL %s\n", k)
	}
	if !opts.DryRun && s.Failed > 0 {
		fmt.Fprintf(&b, "failures written to tmp/oss_backfill_failures.jsonl (resume: --oss-resume tmp/oss_backfill_failures.jsonl)\n")
	}
	return b.String()
}
