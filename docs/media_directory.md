# Media Storage Architecture

> Unified media storage system. All image/video files go through `instrument_media` table; JSONB `photos` fields deprecated.

## Data Model

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

## File Types

| file_type | Description | Thumbnail |
|-----------|------------|-----------|
| `image` | JPEG/PNG/GIF/WebP | `_thumb.jpg` auto-generated |
| `video` | MP4/WebM/MOV | via `video_thumb` entry in same batch |
| `video_thumb` | Auto-generated video thumbnail | N/A |

## Upload Flow

```
Frontend                           Backend
   │                                  │
   ├── POST /upload (multipart) ──────→  HandleUpload
   │   (file)                          │  validate type/size
   │                                   │  storage.Upload()
   │   ←── { file_key, url } ──────────┘
   │
   ├── POST /api/instruments/:id/media ──→  CreateInstrumentMedia
   │   { batch_type, is_display, files }   │  validate batch_type
   │                                        │  create InstrumentMedia records
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

Cleanup is handled by `services/media_cleanup.go` scheduler which runs periodically and deletes eligible `instrument_media` records along with their storage files.

## Image Hierarchy

See `AGENTS.md` → "Instrument Image Hierarchy" for per-field display rules.

### Frontend consumption

- **Cover image** (`cover_image`): first `is_display=true` image → instrument list cards, order cards
- **Display images** (`media.display`): instrument detail page carousel
- **Process images** (`media` by batch_type): activity log, repair panel
- **Video**: instrument detail page, thumb URL in `video_thumb` entry

## Migration Status

| Table | JSONB Field | Migrated | Dual-write |
|-------|------------|----------|------------|
| maintenance_tickets | `repair_photos`, `completion_photos` | ✅ | ✅ |
| repair_records | `photos` | ✅ | ✅ |
| damage_assessments | `photos` | ✅ | ✅ |
| repair_requests | `photos` | ✅ | ✅ |
| repair_request_records | `photos` | ✅ (handler N/A) | — |
| transit_orders | `unpack_photos` | ✅ | ✅ |
| repair_transit_orders | `unpack_photos` | ✅ (handler N/A) | — |

All new writes go to `instrument_media`. Old JSONB fields are kept for backward compatibility.
