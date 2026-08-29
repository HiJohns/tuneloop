# Media Storage Architecture

> Unified media storage system. Business media (instrument/order/repair) go through `instrument_media` table; JSONB `photos` fields deprecated. A unified `media_assets` registry tracks every physical file on disk for orphan detection and periodic cleanup.

## Data Model

### `instrument_media` — business media records

```
instrument_media
├── id              UUID (PK)
├── tenant_id       UUID (NOT NULL)
├── org_id          UUID
├── instrument_id   UUID (nullable — FK to instruments)
├── object_type     VARCHAR(30)  — "repair_request", "transit_order", etc.
├── object_id       UUID         — FK to the entity identified by object_type
├── batch_id        UUID (NOT NULL) — groups files from the same operation
├── batch_type      VARCHAR(20)  — "shipping", "receiving", "repair", "repaired", "relaying"
├── file_name       VARCHAR(255)
├── file_type       VARCHAR(10)  — "image", "video", "video_thumb"
├── file_size       BIGINT
├── storage_key     VARCHAR(500) — backend storage key or URL
├── is_display      BOOLEAN      — true=display image, false=process record
├── sort_order      INT
└── created_at      TIMESTAMP
```

### `media_assets` — unified physical file registry

```
media_assets
├── id                  UUID (PK)
├── storage_key         VARCHAR(500) (NOT NULL, UNIQUE) — physical file key (relative to uploads/media)
├── source_type         VARCHAR(30)  — "content_image", "avatar", "id_photo", "instrument_media"
├── source_id           VARCHAR(100) — reference entity (setting_key / user_id / instrument_id …)
├── is_referenced       BOOLEAN      — still referenced by any content?
├── ref_count           INT          — reference count (increment on reuse)
├── file_size           BIGINT
├── file_type           VARCHAR(10)
├── created_at          TIMESTAMP
└── last_referenced_at  TIMESTAMP    — last time a reference was observed (drives orphan grace period)
```

`instrument_media` remains the authoritative source for business media (source_type=`instrument_media`); `media_assets` is a unified index used only for orphan detection and physical-file sweep — it never makes deletion decisions for business media.

### Two-tier linking

| Link Type | Fields | Entities |
|-----------|--------|----------|
| Instrument-linked | `instrument_id` (NOT NULL) | maintenance_tickets, repair_records, damage_assessments |
| Entity-linked | `object_type` + `object_id` (NOT NULL) | repair_requests, transit_orders, appeals |

### Key constraints

- At least one of `instrument_id` or (`object_type` + `object_id`) must be set
- `batch_id` + `tenant_id` uniquely identify an upload batch
- `video_thumb` entries share the same `batch_id` as the source video

## Batch Types

| batch_type | Usage | Status Transition |
|-----------|-------|-------------------|
| `shipping` | Pre-shipment photos | → shipped |
| `forwarding` | Inter-site forwarding | internal transfer |
| `accepting` | Acceptance photos | receiving dock |
| `returning` | Return photos | → returned |
| `relaying` | Transit site unpack/repack | transit_order arrived/repacked |
| `receiving` | Return inspection photos | → returned, → assessed |
| `repair` | Repair process photos | maintenance/repair workflow |
| `repaired` | Repair completion photos | → maintenance, → repaired |
| `face_capture` | 用户自拍核身素材（图片+视频，人工审核用） | 实名核身流程（长期保存，GC 豁免） |

## File Types

| file_type | Description | Thumbnail |
|-----------|------------|-----------|
| `image` | JPEG/PNG/GIF/WebP | `_display.webp` (1040×1400 max) + `_thumb.jpg` (128px, legacy) auto-generated |
| `video` | MP4/WebM/MOV | via `video_thumb` entry in same batch |
| `video_thumb` | Auto-generated video thumbnail | N/A |

## Upload Flow

```
Frontend                           Backend
   │                                  │
    ├── POST /upload (multipart) ──────→  HandleUpload
    │   (file)                          │  validate type/size
    │                                   │  storage.Upload()
    │                                   │  media_registry.RegisterAsset(source_type=content_image)
    │   ←── { file_key, url } ──────────┘
    │
    ├── POST /api/instruments/:id/media ──→  CreateInstrumentMedia
    │   { batch_type, is_display, files }   │  validate batch_type
    │                                        │  create InstrumentMedia records
    │                                        │  media_registry.RegisterAsset(source_type=instrument_media)
    │                                        │  generate thumbnails (images)
    │   ←── { media: [...] } ────────────────┘
```

### Upload-then-submit pattern

1. Upload files → get `file_key` (storage key)
2. Construct payload with `file_key` as the URL reference
3. Backend handler creates `instrument_media` records automatically

## Retention Policy

| Category | Retention | Configurable |
|----------|-----------|-------------|
| Display images (`is_display=true`) | Permanent | No |
| Process record images | 180 days | `system_settings.media_retention_days` |
| Video files | 180 days | (same as process records) |
| **Identity materials (`batch_type=face_capture`, id photos)** | **Permanent（长期保存）** | **No — GC 豁免（生物特征合规数据，禁止按 180 天回收）** |
| Orphan files (unreferenced) | grace period (default 30 days) | `system_settings.media_orphan_grace_days` |

Cleanup is handled by `services/media_cleanup.go` scheduler which runs periodically and deletes eligible `instrument_media` records **along with their physical storage files** (including derived `_display.webp` / `_thumb.jpg` and `video_thumb` entries). A separate orphan GC pass reconciles `media_assets` against the `uploads/media/` directory — files no longer referenced (rich-text image removed, avatar/id_photo replaced, batch-import session expired) are physically deleted after the grace period.

## Storage Directories

| Directory | Purpose | Governed by |
|-----------|---------|-------------|
| `uploads/media/` | Unified media (rich-text content images, avatars, id photos, instrument media) | `media_assets` registry + `media_cleanup.go` GC |
| `uploads/media/face_captures/{userID}/` | 实名核身自拍素材（图片 + 可选视频），人工审核证据，长期保存 | `media_assets` registry（source_type=face_capture，GC 豁免） |
| `uploads/batch/` | Batch-import ZIP extraction temp (`{sessionID}/`) | `media_cleanup.go` GC (expired session dirs) |
| `uploads/photos/` | Legacy outbound photo mechanism (`{tenant}/{sn}/` with manifest.yaml + ZIP archive) | Historical — registered for attribution only, not refactored |


## Image Hierarchy

See `AGENTS.md` → "Instrument Image Hierarchy" for per-field display rules.

### Frontend consumption

- **Cover image** (`cover_image`): dedicated square upload (≤72×72, WebP Q0.8) → all list views (home, staff, orders, cart)
- **Display images** (`media.display`): instrument detail page carousel (uses `_display.webp` variant)
- **Process images** (`media` by batch_type): activity log, repair panel
- **Video**: instrument detail page, thumb URL in `video_thumb` entry
- **Poster**: instrument detail page (max width 1040px, WebP Q0.8)

## Image Processing Variants (v3)

| Variant | Format | Quality | Max Size | Purpose |
|---------|--------|---------|----------|---------|
| `{key}_display.webp` | WebP | 0.8 | 1040×1400px | Display thumbnail (carousel) |
| `{key}_thumb.jpg` | JPEG | 85 | 128×128px | Legacy list thumbnail |
| `cover_{id}.webp` | WebP | 0.8 | 72×72px | Square cover image |
| Poster | WebP | 0.8 | max-width 1040px | Detail page poster |

## Migration Status

| Table | JSONB Field | Migrated | Dual-write |
|-------|------------|----------|------------|
| maintenance_tickets | `repair_photos`, `completion_photos` | ✅ | ✅ |
| repair_records | `photos` | ✅ | ✅ |
| damage_assessments | `photos` | ✅ | ❌ 表已废弃（#1708/#1710/#1711）——验收统一写入 damage_reports，照片统一 instrument_media（receiving 批次）；迁移 `--migrate-damage-photos` 回填存量 |
| repair_requests | `photos` | ✅ | ✅ |
| repair_request_records | `photos` | ✅ (handler N/A) | — |
| transit_orders | `unpack_photos` | ✅ | ✅ |
| repair_transit_orders | `unpack_photos` | ✅ (handler N/A) | — |

All new writes go to `instrument_media`. Old JSONB fields are kept for backward compatibility.
