# TuneLoop Project Reference

## Two Environments

### Development (Dev)

| Component | URL / Port | Working Directory |
|---|---|---|
| PC Frontend (Vite dev) | `http://localhost:5554` | `frontend-pc/` |
| Mobile Frontend (Vite dev) | `http://localhost:5553` | `frontend-mobile/` |
| Backend (PC + API) | `http://localhost:5557` | `backend/` (run `go run main.go`) |
| Backend (Mobile + API) | `http://localhost:5556` | same process |
| IAM | `http://opencode.linxdeep.com:5552` | `/home/coder/beaconiam/` (run `go run cmd/api/main.go`) |
| Database | PostgreSQL `localhost:5432` | `tuneloop_db` |
| Redis | `localhost:6379` | Docker container `jobmaster-redis` |

**How to start dev:**
```bash
# Terminal 1: Backend (from project root)
cd backend && go run main.go

# Terminal 2: PC frontend
cd frontend-pc && npm run dev

# Terminal 3: Mobile frontend
cd frontend-mobile && npm run dev

# IAM is already running as a persistent process on port 5552
```

**Dev IAM clients:**
- PC: `tuneloop_dev_web` / secret: `bzJyPw1DqJ2uM5lUI6bSjdPxtdyyzah7`
- WX: `tuneloop_dev_wechat` / secret: `CQUJAXsFsOkvRa17zhOoHj2DGzcA1d3T`

**Config file:** `.env` (project root)

### Prerelease

| Component | URL / Port | Working Directory |
|---|---|---|
| PC Frontend | `https://web.cadenzayueqi.com` | N/A (served by backend) |
| Mobile Frontend | `https://wx.cadenzayueqi.com` | N/A (served by backend) |
| Backend (PC + API) | `http://localhost:5558` | `prerelease/` |
| Backend (Mobile + API) | `http://localhost:5559` | same process |
| IAM | `https://iam.cadenzayueqi.com` | `/home/coder/beaconiam/prerelease/` |
| Database | same PostgreSQL `localhost:5432` | `tuneloop_db` (shared with dev) |
| Redis | `localhost:6379` | shared |

**How to start prerelease:**
```bash
# Build everything
make prerelease

# Start backend (MUST run from prerelease/ directory)
cd prerelease && ./service/tuneloop --env=.env

# IAM is already running as a persistent process on port 5560
```

**Prerelease IAM clients:**
- PC: `tuneloop_web` / secret: `3GHS49_Bck0fMcQRSSApynEaL8jKSXWv`
- WX: `tuneloop_wechat` / secret: `7yCtXks9kGK5mI9Drmh8xRjcpSV1lPG9`

**Config file:** `prerelease/.env` (see `prerelease/.env.example` for reference)

**Nginx proxy rules:**

| Domain | Proxies To | Config |
|---|---|---|
| `web.cadenzayueqi.com` | `http://127.0.0.1:5558` | `/etc/nginx/conf.d/web.conf` |
| `wx.cadenzayueqi.com` | `http://127.0.0.1:5559` | `/etc/nginx/conf.d/wx.conf` |
| `iam.cadenzayueqi.com` | `http://127.0.0.1:5560` | `/etc/nginx/conf.d/iam.conf` |

All three domains use the SSL certificate for `iam.cadenzayueqi.com`.

---

## Architecture

### Backend

Single Go binary that runs **two HTTP servers** in one process:
- **PC server** (`TUNELOOP_WWW_PORT`, default 5557 dev / 5558 prerelease) - serves PC frontend + API
- **Mobile server** (`TUNELOOP_WX_PORT`, default 5556 dev / 5559 prerelease) - serves mobile frontend + API

Both servers expose **identical API routes** via `setupAPIRoutes()`.

### Frontend Serving

The backend serves frontend static files from paths relative to CWD:
- PC: `../frontend-pc/dist/` (resolved via `getAbsPath`)
- Mobile: `../frontend-mobile/dist/` (resolved via `getAbsPath`)

When running from `prerelease/`, these resolve to `frontend-pc/dist/` and `frontend-mobile/dist` (sibling directories). The Makefile `prebuild-pc`/`prebuild-mobile` targets build into `frontend-{pc,mobile}/dist/` first, then copy to `prerelease/www/` and `prerelease/mobile/` as backup. **The backend serves from `frontend-*/dist/`, not from `prerelease/www/` or `prerelease/mobile/`.**

### IAM (BeaconIAM)

Separate project at `/home/coder/beaconiam/`. Two instances:
- **Dev** (port 5552): runs via `go run cmd/api/main.go`, uses SQLite + PostgreSQL `beaconiam_debug`
- **Prerelease** (port 5560): runs compiled binary from `prerelease/`, uses PostgreSQL `beaconiam_db`, has RSA key pair for RS256 signing

Dev IAM issues HS256 tokens. Prerelease IAM issues RS256 tokens. The backend `ValidateToken` supports both.

### OAuth Flow

1. Frontend redirects user to IAM `/oauth/authorize?client_id=...&redirect_uri=...&response_type=code`
2. IAM authenticates user and redirects back to `redirect_uri?code=...`
3. Frontend sends `code` to backend `POST /api/auth/callback` (PC sends `{ code }`, WX sends `{ code, client_type: 'wx' }`)
4. Backend exchanges code for token with IAM, using the correct `redirect_uri` per client type (`IAM_PC_REDIRECT_URI` or `IAM_WX_REDIRECT_URI`)
5. Backend sets cookies and returns token in JSON response
6. Frontend stores token in `localStorage`

---

## Configuration Loading

### Backend .env Loading Order (CRITICAL)

There are **three** `godotenv.Load()` calls. `godotenv.Load()` does NOT override already-set env vars, so the first load wins:

1. **`database/db.go` init()** - tries `.env`, `../.env`, `../../.env` (stops at first found)
2. **`services/iam.go` init()** - calls `godotenv.Load()` (redundant, vars already set)
3. **`main.go` with `--env` flag** - calls `godotenv.Load(envFile)` (cannot override vars from step 1)

**Implication:** When running `cd prerelease && ./service/tuneloop --env=.env`:
- Step 1 looks for `prerelease/.env` -> `prerelease/../.env` (root `.env`). Which one loads first depends on whether `prerelease/.env` exists (it does), so `prerelease/.env` loads first and wins.
- Step 3 loads `prerelease/.env` again (no-op, already set).

**Warning:** If `prerelease/.env` is missing, step 1 would load root `.env` instead, giving you dev config on the prerelease server. Always ensure `prerelease/.env` exists.

### IAM Service Package-Level Variables

`services/iam.go` captures `BEACONIAM_INTERNAL_URL` and `BEACONIAM_EXTERNAL_URL` at package init time:
```go
var (
    iamInternalURL = os.Getenv("BEACONIAM_INTERNAL_URL")  // captured during init()
    iamExternalURL = os.Getenv("BEACONIAM_EXTERNAL_URL")
)
```
These work correctly because `database/db.go` init() runs before `services/iam.go` init() (Go init order follows import dependency).

### Frontend .env Loading (Vite)

| Mode | Files Loaded | Override Priority |
|---|---|---|
| `vite dev` | `.env` + `.env.development` | Shell env > `.env.development` > `.env` |
| `vite build` (production) | `.env` + `.env.production` | Shell env > `.env.production` > `.env` |
| `vite build --mode prerelease` | `.env` + `.env.prerelease` | Shell env > `.env.prerelease` > `.env` |

---

## Environment Variables Reference

### Backend (.env)

| Variable | Dev Default | Prerelease Value | Purpose |
|---|---|---|---|
| `TUNELOOP_DB` | `tuneloop_db` | `tuneloop_db` | Database name |
| `POSTGRES_USER` | `tuneloop_user` | `tuneloop_user` | DB user |
| `POSTGRES_PASSWORD` | `tune_secret_2026` | `tune_secret_2026` | DB password |
| `TUNELOOP_WWW_PORT` | `5557` | `5558` | PC server port |
| `TUNELOOP_WX_PORT` | `5556` | `5559` | Mobile server port |
| `IAM_PC_CLIENT_ID` | `tuneloop_dev_web` | `tuneloop_web` | IAM OAuth client ID for PC |
| `IAM_PC_CLIENT_SECRET` | `bzJyPw1DqJ2uM5lUI6bSjdPxtdyyzah7` | `3GHS49_Bck0fMcQRSSApynEaL8jKSXWv` | IAM OAuth client secret for PC |
| `IAM_PC_REDIRECT_URI` | `http://opencode.linxdeep.com:5554/callback` | `https://web.cadenzayueqi.com/callback` | OAuth callback URL for PC |
| `IAM_WX_CLIENT_ID` | `tuneloop_dev_wechat` | `tuneloop_wechat` | IAM OAuth client ID for WX |
| `IAM_WX_CLIENT_SECRET` | `CQUJAXsFsOkvRa17zhOoHj2DGzcA1d3T` | `7yCtXks9kGK5mI9Drmh8xRjcpSV1lPG9` | IAM OAuth client secret for WX |
| `IAM_WX_REDIRECT_URI` | `http://opencode.linxdeep.com:5553/callback` | `https://wx.cadenzayueqi.com/callback` | OAuth callback URL for WX |
| `BEACONIAM_EXTERNAL_URL` | `http://opencode.linxdeep.com:5552` | `https://iam.cadenzayueqi.com` | IAM URL exposed to frontend |
| `BEACONIAM_INTERNAL_URL` | `http://localhost:5552` | `http://localhost:5560` | IAM URL for backend-to-IAM calls |
| `IAM_NAMESPACE` | `tuneloop_dev` | `tuneloop` | IAM namespace |
| `IAM_SECRET` | `7jqbyc3uRBOO-rXOm7-AEsGsAkfmBJJ0` | `XGClIrajpcSQA3wpf1ynJ2WDm0d3uhF9` | IAM shared secret |
| `IAM_CLIENT_ID` | *(not set in dev .env)* | `tuneloop-pc` | Backend IAM client ID for token validation |
| `IAM_CLIENT_SECRET` | *(not set in dev .env)* | `Welcome1234` | Backend IAM client secret |

### PC Frontend (.env)

| Variable | Dev | Production | Prerelease (Makefile inline) |
|---|---|---|---|
| `VITE_API_BASE_URL` | *(not set, falls back to `/api`)* | *(not set, falls back to `/api`)* | `/api` |
| `VITE_BEACONIAM_EXTERNAL_URL` | `http://opencode.linxdeep.com:5552` | `https://iam.cadenzayueqi.com` | `https://iam.cadenzayueqi.com` |
| `VITE_IAM_PC_CLIENT_ID` | `tuneloop_dev_web` | `tuneloop_web` | `tuneloop_web` |
| `VITE_IAM_PC_REDIRECT_URI` | `http://localhost:5554/callback` | `https://tuneloop.example.com/callback` | `https://web.cadenzayueqi.com/callback` |

### Mobile Frontend (.env)

| Variable | Dev (`.env`) | Prerelease (`.env.prerelease`) |
|---|---|---|
| `VITE_API_BASE_URL` | *(not set, falls back to `/api`)* | `/api` |
| `VITE_BEACONIAM_EXTERNAL_URL` | `http://opencode.linxdeep.com:5552` | `https://iam.cadenzayueqi.com` |
| `VITE_IAM_WX_CLIENT_ID` | `tuneloop_dev_wechat` | `tuneloop_wechat` |
| `VITE_IAM_WX_REDIRECT_URI` | `http://localhost:5556/callback` | `https://wx.cadenzayueqi.com/callback` |

---

## Build Commands

| Command | What It Does |
|---|---|
| `make prerelease` | Build backend binary + PC frontend + mobile frontend for prerelease |
| `make prebuild-pc` | Build PC frontend with inline env var overrides (production mode) |
| `make prebuild-mobile` | Build mobile frontend with `--mode prerelease` (uses `.env.prerelease`) |
| `make prebuild-backend` | Compile `go build` binary to `prerelease/service/tuneloop` |
| `make build-frontend` | Build both frontends for dev/production |
| `make install` | Install all dependencies |
| `make init` | Install + run database migrations |

---

## Key Code Locations

| Concern | File | Lines |
|---|---|---|
| Backend entry point | `backend/main.go` | `main()` at 318 |
| .env loading (init) | `backend/database/db.go` | `init()` at 31 |
| .env loading (redundant) | `backend/services/iam.go` | `init()` at 20 |
| .env loading (--env flag) | `backend/main.go` | 325-336 |
| IAM service init | `backend/services/iam.go` | package vars at 24-27, `NewIAMService()` at 80 |
| Token validation (RS256+HS256) | `backend/services/iam.go` | `ValidateToken()` |
| Valid JWT issuers | `backend/middleware/iam.go` | `validIssuers` at 37-42 |
| OAuth callback | `backend/handlers/auth.go` | `Callback()` at 74 |
| Cookie domain logic | `backend/handlers/auth.go` | 134-142 |
| PC frontend OAuth | `frontend-pc/src/components/ProtectedRoute.jsx` | `redirectToLogin()` |
| PC frontend token storage | `frontend-pc/src/services/api.js` | `getToken()`, `setToken()` |
| Mobile frontend OAuth | `frontend-mobile/src/App.jsx` | `OAuthCallback` at 43 |
| Mobile frontend API | `frontend-mobile/src/services/api.js` | `request()`, `getToken()` |
| PC menu + permissions | `frontend-pc/src/App.jsx` | `MainLayout()` |
| API config endpoint | `backend/main.go` | `/api/config` at 74-117 |
| Port configuration | `backend/main.go` | 365-366 |

---

## Known Issues

1. **`--env` flag cannot override init-time vars** - `godotenv.Load()` in main() cannot override vars already set by init(). Should use `godotenv.Overload()` if override behavior is desired.

2. **Prerelease frontend served from `frontend-*/dist/`** - The Makefile copies builds to `prerelease/www/` and `prerelease/mobile/` but the backend serves from `../frontend-pc/dist` and `../frontend-mobile/dist` (relative to CWD). Running `npm run build` in either frontend after `make prerelease` would overwrite the prerelease build being served.

3. **Shared SSL certificate** - `web.cadenzayueqi.com` and `wx.cadenzayueqi.com` use the `iam.cadenzayueqi.com` SSL certificate. Will cause browser warnings if the cert doesn't have SANs for all three domains.

4. **`.env.production` has wrong redirect URI** - `frontend-pc/.env.production` has `VITE_IAM_PC_REDIRECT_URI=https://tuneloop.example.com/callback` (placeholder domain). The Makefile overrides this inline for prerelease builds.

5. **Triple godotenv.Load()** - Redundant loads in `services/iam.go` init() and potentially in `main.go`.

6. **Cookie domain only for specific domains** - `auth.go` only sets cookie domain for `cadenzayueqi.com` and `linxdeep.com`. Other domains get host-only cookies.

7. **State validation disabled** - OAuth state validation is disabled due to cross-domain cookie issues. Needs server-side state storage (Redis) for production.
