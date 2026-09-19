.PHONY: web-dev mobile-dev mobile-weapp-dev weapp-upload-prod weapp-upload-pre weapp-upload-dev weapp-build weapp-build-pre weapp-build-prod weapp-build-local weapp-cleanup weapp-check web mobile build-frontend build-pc build-mobile kill-port run-backend run run-prod stop install init tunnel-ssh check-tunnel check-uploads mount-sshfs ensure-junction mount-uploads unmount-sshfs unmount-uploads

NODE_MAJOR := $(shell node -v 2>/dev/null | sed 's/v//' | cut -d. -f1)
NVM22 := export PATH="$(NODE22_PATH):$$PATH" &&

weapp-check:
	@$(NVM22) echo "Node $$(node -v) ready" || (echo "ERROR: Node 22 not available via nvm"; exit 1)
	@echo "== weapp style gate (#1831) =="
	@SRC_DIR=frontend-mobile/src; \
	HARD1=$$(grep -rhoE "space-[xy]-[0-9]" $$SRC_DIR --include="*.jsx" 2>/dev/null | wc -l | tr -d ' '); \
	HARD2=$$(grep -rhoE "(w|h|top|bottom|left|right|translate-[xy]|rotate|scale)-(-?[0-9]+/[0-9]+)" $$SRC_DIR --include="*.jsx" 2>/dev/null | wc -l | tr -d ' '); \
	if [ "$$HARD1" != "0" ] || [ "$$HARD2" != "0" ]; then \
		echo "ERROR [hard gate]: space-y/x=$$HARD1, fraction=$$HARD2 (must be 0, baseline cleared in #1829)"; \
		exit 1; \
	fi; \
	NEW_VIOL=$$( { git diff -- $$SRC_DIR; git diff --cached -- $$SRC_DIR; git ls-files --others --exclude-standard -- $$SRC_DIR | xargs -r sed 's/^/+/'; } 2>/dev/null | grep -E "^\+" | grep -E "space-[xy]-[0-9]|[a-z-]+-\[[^]]*\]|(w|h|top|bottom|left|right|translate-[xy]|rotate|scale)-(-?[0-9]+/[0-9]+)|(^| )((active|hover|focus|first|last|sm|md|lg):)|\b(bg|text|border)-(gray|zinc|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white|transparent)-[0-9]+/[0-9]+" | sed 's/^+//' | sort -u); \
	if [ -n "$$NEW_VIOL" ]; then \
		echo "ERROR [incremental block]: new lines contain weapp banned classes (soft gate: variant/opacity/arbitrary, baseline in #1832):"; \
		echo "$$NEW_VIOL" | sed 's/^/    + /'; \
		exit 1; \
	fi; \
	echo "== weapp style gate: PASS (baseline 71/25/55+ allowed, delta 0) =="

kill-port:
	@fuser -k 5556/tcp 2>/dev/null || true
	@fuser -k 5557/tcp 2>/dev/null || true

# SSH Tunnel (from .env config)
include backend/.env
# Frontend mobile local config (TARO_APP_API_BASE_URL, TARO_APP_VERSION)
include frontend-mobile/.env.local
export

check-tunnel:
	@netstat -ano 2>/dev/null | grep ":$(SSH_TUNNEL_LOCAL_PORT)" | grep -q "LISTENING" && \
		echo "[TUNNEL] SSH tunnel is running" || \
		echo "[TUNNEL] SSH tunnel is not running"

tunnel-ssh:
	@netstat -ano 2>/dev/null | grep ":$(SSH_TUNNEL_LOCAL_PORT)" | grep -q "LISTENING" && \
		echo "[TUNNEL] SSH tunnel already running" || \
		(echo "[TUNNEL] Starting SSH tunnel..." && \
		ssh cadenza -L $(SSH_TUNNEL_LOCAL_PORT):localhost:$(SSH_TUNNEL_REMOTE_PORT) \
			-o ServerAliveInterval=60 -N &)

# 探测是否已挂载。判据：Z: 盘存在且能列出远端内容（非空）。
# 不检查 backend/uploads（junction 目标失效时 ls 返回空，无法区分"未挂载"）。
check-uploads:
	@if cmd //c "if exist $(SSHFS_MOUNT_LETTER):\\ (exit 0) else (exit 1)"; then \
		echo "[SSHFS] uploads already mounted ($(SSHFS_MOUNT_LETTER):)"; \
		exit 0; \
	else \
		echo "[SSHFS] uploads not mounted ($(SSHFS_MOUNT_LETTER): absent)"; \
		exit 1; \
	fi

# 拉起 sshfs 挂载（无 ro，允许后端写入）：远端 -> Z: 盘符
# 必须用 SSHFS_SSH_COMMAND（自带 Cygwin ssh，规避 dup() 问题）
mount-sshfs:
	@echo "[SSHFS] Mounting $(SSHFS_REMOTE_PATH) -> $(SSHFS_MOUNT_LETTER): ..."
	@cd backend && "$(SSHFS_EXEC)" -o ssh_command="$(SSHFS_SSH_COMMAND)",IdentityFile=$(SSH_TUNNEL_KEY),StrictHostKeyChecking=no,reconnect,allow_other $(SSHFS_REMOTE_USER)@$(SSH_TUNNEL_HOST):$(SSHFS_REMOTE_PATH) $(SSHFS_MOUNT_LETTER): || \
		(echo "[SSHFS] ERROR: mount failed (sshfs exited non-zero)"; exit 1)
	@echo "[SSHFS] Waiting for mount to become ready..."
	@i=0; while [ $$i -lt 15 ]; do \
		if cmd //c "if exist $(SSHFS_MOUNT_LETTER):\\ (exit 0) else (exit 1)"; then \
			echo "[SSHFS] Mounted OK: $(SSHFS_MOUNT_LETTER):"; \
			exit 0; \
		fi; \
		sleep 1; i=$$((i+1)); \
	done; \
	echo "[SSHFS] ERROR: mount not ready after 15s — cleaning up..."; \
	taskkill //F //IM sshfs.exe 2>/dev/null || true; \
	exit 1

# 确保 backend/uploads 是指向 Z: 的 junction（WinFsp 网络文件系统不能直接挂目录）
# 若缺失/非 junction，则重建。重建使用 PowerShell New-Item（无需管理员）。
ensure-junction:
	@if ! powershell -NoProfile -Command "if (Test-Path '$(CURDIR)/backend/uploads') { exit 0 } else { exit 1 }" 2>/dev/null; then \
		echo "[SSHFS] Creating junction backend/uploads -> $(SSHFS_MOUNT_LETTER): ..."; \
		powershell -NoProfile -Command "New-Item -ItemType Junction -Path '$(CURDIR)/backend/uploads' -Target '$(SSHFS_MOUNT_LETTER):' | Out-Null" || \
			(echo "[SSHFS] ERROR: cannot create junction"; exit 1); \
	fi
	@powershell -NoProfile -Command "if ((Get-Item '$(CURDIR)/backend/uploads' -Force).LinkType -eq 'Junction') { exit 0 } else { exit 1 }" 2>/dev/null || \
		(echo "[SSHFS] ERROR: backend/uploads is not a junction"; exit 1)
	@echo "[SSHFS] junction OK: backend/uploads -> $(SSHFS_MOUNT_LETTER):"

# 挂载就绪（强依赖）：探测 -> 拉起 -> 校验 junction -> 失败则中止（不启动后端）
mount-uploads:
	@if $(MAKE) --no-print-directory check-uploads; then \
		$(MAKE) --no-print-directory ensure-junction; \
	elif $(MAKE) --no-print-directory mount-sshfs; then \
		$(MAKE) --no-print-directory ensure-junction; \
	else \
		echo "[SSHFS] FATAL: uploads mount failed — cleaning up residual sshfs..."; \
		taskkill //F //IM sshfs.exe 2>/dev/null || true; \
		exit 1; \
	fi
	@echo "[SSHFS] uploads mount ready"

# 卸载 SSHFS 挂载：taskkill sshfs.exe（WinFsp 挂载随进程结束消失）+ net use 兜底（兼容 UNC/svc 映射）
# 注：必须用 taskkill（Windows 原生），pkill 对 sshfs.exe 不可靠（Cygwin 进程匹配问题）。
# 同时清理 5556/5557 端口的旧进程（fuser -k）避免端口占用残留。
unmount-sshfs:
	@echo "[SSHFS] Unmounting uploads..."
	@taskkill //F //IM sshfs.exe 2>/dev/null || true
	@net use $(SSHFS_MOUNT_LETTER): /delete 2>/dev/null || true
	@fuser -k 5556/tcp 2>/dev/null || true
	@fuser -k 5557/tcp 2>/dev/null || true
	@sleep 1
	@if cmd //c "if exist $(SSHFS_MOUNT_LETTER):\\ (exit 0) else (exit 1)"; then \
		echo "[SSHFS] WARNING: mount still present — check for leftover sshfs process"; \
	else \
		echo "[SSHFS] Unmounted OK"; \
	fi

unmount-uploads: unmount-sshfs

build-frontend: build-pc build-mobile

build-pc:
	@echo "Building PC frontend..."
	cd frontend-pc && npm install && npm run build

build-mobile:
	@echo "Building Mobile frontend..."
	cd frontend-mobile && npm install && VITE_APP_VERSION=$(FRONTEND_VERSION) npm run build

web: build-pc
	@echo "PC frontend build completed."

mobile: build-mobile
	@echo "Mobile frontend build completed."

run-backend: kill-port
	@echo "=========================================="
	@echo "Starting SSH tunnel + SSHFS + backend..."
	@echo "Backend API (Mobile): http://localhost:5556"
	@echo "Backend API (PC):     http://localhost:5557"
	@echo "SSH Tunnel:           localhost:5432 → cadenza → remote DB"
	@echo "SSHFS:                remote uploads → $(SSHFS_LOCAL_MOUNT) (via $(SSHFS_MOUNT_LETTER):)"
	@echo "Log file:             backend/backend.log"
	@echo "=========================================="
	@netstat -ano 2>/dev/null | grep ":$(SSH_TUNNEL_LOCAL_PORT)" | grep -q "LISTENING" && \
		echo "[TUNNEL] SSH tunnel already running" || \
		(echo "[TUNNEL] Starting SSH tunnel..." && \
		ssh cadenza -L $(SSH_TUNNEL_LOCAL_PORT):localhost:$(SSH_TUNNEL_REMOTE_PORT) \
			-o ServerAliveInterval=60 -N &)
	@sleep 5
	@$(MAKE) --no-print-directory mount-uploads || \
		(echo "[SSHFS] FATAL: uploads mount failed — aborting backend startup (uploads is a hard dependency)"; exit 1)
	cd backend && go run main.go 2>&1 | tee backend.log

web-dev:
	@echo "Starting PC frontend development server..."
	@echo "PC Frontend: http://localhost:5554 (with source map)"
	@cd frontend-pc && npm run dev

mobile-dev:
	@echo "Starting Mobile frontend development server..."
	@echo "Mobile Frontend: http://localhost:5553"
	@cd frontend-mobile && npm run dev

mobile-weapp-dev: weapp-check
	@echo "Starting Taro weapp build (watch)..."
	@echo "Open WeChat Developer Tool -> import dist-weapp/"
	@cd frontend-mobile && npm run dev:weapp

# ============================================================
# Weapp release flow (#1619)
# Builds are archived to releases/weapp-{pre,prod}/<VERSION>/.
# Uploads ONLY consume archived builds — never recompile.
# Rollback = upload an older archive. Archives kept >= 180 days.
# ============================================================
WEAPP_RELEASE_DIR := releases
WEAPP_AUTO_VERSION := $(shell date -u +%Y%m%d-%H%M%S)_$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# Frontend package version shown in app UI: 1.0.<git short hash> (#1692)
FRONTEND_VERSION := 1.0.$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# Backend build tag: git short hash injected via ldflags (GET /api/config build)
GIT_SHORT := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)

weapp-build: weapp-check
	@rm -rf frontend-mobile/node_modules/.cache
	@rm -rf frontend-mobile/dist-weapp
	@echo "Building WeApp (production apiBaseUrl)..."
	@cd frontend-mobile && TARO_APP_API_BASE_URL=https://wx.cadenzayueqi.com/api TARO_APP_VERSION=$(FRONTEND_VERSION) npm run build:weapp

weapp-build-pre: weapp-check
	@rm -rf frontend-mobile/node_modules/.cache
	@rm -rf frontend-mobile/dist-weapp
	@echo "Building WeApp (pre-production apiBaseUrl)..."
	@cd frontend-mobile && TARO_APP_API_BASE_URL=https://prewx.cadenzayueqi.com/api TARO_APP_VERSION=$(FRONTEND_VERSION) npm run build:weapp
	@make weapp-archive-pre VERSION=$(if $(filter command line,$(origin VERSION)),$(VERSION),$(WEAPP_AUTO_VERSION))

weapp-build-local: weapp-check
	@rm -rf frontend-mobile/node_modules/.cache
	@rm -rf frontend-mobile/dist-weapp
	@echo "Building WeApp (local apiBaseUrl=$(TARO_APP_API_BASE_URL))..."
	@cd frontend-mobile && TARO_APP_API_BASE_URL=$(TARO_APP_API_BASE_URL) TARO_APP_VERSION=$(TARO_APP_VERSION) npm run build:weapp

weapp-build-prod: weapp-build
	@make weapp-archive-prod VERSION=$(if $(filter command line,$(origin VERSION)),$(VERSION),$(WEAPP_AUTO_VERSION))

# Archive a build (wxss cleanup applied once at archive time — archive is
# the ready-to-upload artifact).
weapp-archive-pre:
	@mkdir -p $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))
	@rm -rf $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cp -r frontend-mobile/dist-weapp $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cd $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp && \
	sed -i 's/\\!//g; s/!important//g' app.wxss
	@echo "Archived: $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp"

weapp-archive-prod:
	@mkdir -p $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))
	@rm -rf $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cp -r frontend-mobile/dist-weapp $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cd $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp && \
	sed -i 's/\\!//g; s/!important//g' app.wxss
	@echo "Archived: $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp"

# Upload ONLY an archived build — never recompile.
# 单 appid（wxcb44a1be70e356ed）策略（#1694）：不再有独立的预生产小程序。
# 开发版（测试）：weapp-build-pre 构建（prewx apiBaseUrl）+ weapp-upload-dev
#   上传为开发版（微信版本号 APP_VERSION，默认 1.0.0-dev）→ 后台设体验版分发。
# 发布版（正式）：weapp-build-prod 构建（wx apiBaseUrl）+ weapp-upload-prod
#   上传为正式版（APP_VERSION 语义化 1.0.x，必须高于线上/审核中版本）→ 提交审核。
# weapp-upload-pre 为历史命令名别名（#1704）：.PHONY 残留导致 "Nothing to be
# done"——语义=开发版上传（单 appid 策略），保留兼容旧脚本。
weapp-upload-pre: weapp-upload-dev

weapp-upload-dev:
	@test -d $(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)/dist-weapp || (echo "ERROR: archive '$(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)' not found — run 'make weapp-build-pre VERSION=$(VERSION)' first"; exit 1)
	@cd frontend-mobile && \
	node_modules/.bin/miniprogram-ci upload \
		--pp ../$(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)/dist-weapp \
		--pkp D:/Work/AI/rent/certs/private.wxcb44a1be70e356ed.key \
		--appid wxcb44a1be70e356ed \
		--uv $(or $(APP_VERSION),1.0.0-dev) \
		--ud "$(or $(DESC),dev build)"

weapp-upload-prod:
	@test -d $(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)/dist-weapp || (echo "ERROR: archive '$(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)' not found — run 'make weapp-build-prod VERSION=$(VERSION)' first"; exit 1)
	@cd frontend-mobile && \
	node_modules/.bin/miniprogram-ci upload \
		--pp ../$(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)/dist-weapp \
		--pkp D:/Work/AI/rent/certs/private.wxcb44a1be70e356ed.key \
		--appid wxcb44a1be70e356ed \
		--uv $(or $(APP_VERSION),1.0.0) \
		--ud "$(or $(DESC),release)"

# Delete archives older than 180 days. DRY=1 prints what would be removed.
weapp-cleanup:
	@echo "Scanning $(WEAPP_RELEASE_DIR)/weapp-*/ for archives older than 180 days..."
	@find $(WEAPP_RELEASE_DIR)/weapp-pre $(WEAPP_RELEASE_DIR)/weapp-prod -mindepth 1 -maxdepth 1 -type d -mtime +180 2>/dev/null | while read d; do \
		if [ "$(DRY)" = "1" ]; then echo "  [dry-run] would remove: $$d"; \
		else echo "  removing: $$d"; rm -rf "$$d"; fi; \
	done
	@echo "weapp-cleanup done."

run: run-backend

run-prod: build-frontend run-backend

stop:
	@echo "Stopping all services..."
	@$(MAKE) --no-print-directory unmount-sshfs
	@netstat -ano 2>/dev/null | grep ":$(SSH_TUNNEL_LOCAL_PORT)" | grep "LISTENING" | \
		awk '{print $$5}' | sort -u | xargs -I{} taskkill //F //PID {} 2>/dev/null || true
	@pkill -f "go run main.go" || true
	@pkill -f "tee backend.log" || true
	@pkill -f "npm run dev" || true

install:
	@echo "Installing backend dependencies..."
	cd backend && go mod tidy
	@echo "Installing PC frontend dependencies..."
	cd frontend-pc && npm install
	@echo "Installing mobile frontend dependencies..."
	cd frontend-mobile && npm install

init: install
	@echo "Running database migrations..."
	cd backend && go run cmd/migrate/main.go

# Prerelease targets
.PHONY: prerelease clean-prerelease prebuild-pc prebuild-mobile prebuild-backend release
TIMESTAMP := $(shell date +%Y%m%d-%H%M%S)
GIT_HASH := $(shell git rev-parse --short HEAD)
VERSION := $(shell cat VERSION 2>/dev/null || echo "dev")
RELEASE_DIR := /opt/flow
PKG_NAME := tuneloop-pre_$(TIMESTAMP)_$(GIT_HASH)
RELEASE_BUILD := /tmp/release_build_$(TIMESTAMP)

clean-prerelease:
	@echo "Cleaning build cache..."
	rm -rf $(RELEASE_BUILD)

release: clean-prerelease
	@echo "=========================================="
	@echo "Release v$(VERSION): $(PKG_NAME)"
	@echo "IMPORTANT: All releases go to PRE-PROD first!"
	@echo "  Verify on pre-prod, then promote to prod via release.sh"
	@echo "=========================================="
	mkdir -p $(RELEASE_BUILD)/tuneloop-pre/www $(RELEASE_BUILD)/tuneloop-pre/mobile \
	         $(RELEASE_BUILD)/tuneloop-pre/service $(RELEASE_BUILD)/tuneloop-pre/database
	# PC frontend (IAM config from /api/config at runtime; version = FRONTEND_VERSION with git short hash)
	$(NVM22) cd frontend-pc && VITE_APP_VERSION=$(FRONTEND_VERSION) npm run build
	cp -r frontend-pc/dist/* $(RELEASE_BUILD)/tuneloop-pre/www/
	# Mobile frontend (Vite H5, IAM config from /api/config at runtime)
	$(NVM22) cd frontend-mobile && VITE_APP_VERSION=$(FRONTEND_VERSION) npm run build -- --mode prerelease
	cp -r frontend-mobile/dist/* $(RELEASE_BUILD)/tuneloop-pre/mobile/
	# Backend (version + build hash injected via ldflags)
	cd backend && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-X main.Version=$(VERSION) -X main.Build=$(GIT_SHORT)" -o $(RELEASE_BUILD)/tuneloop-pre/service/tuneloop .
	cp -r backend/database/migrations $(RELEASE_BUILD)/tuneloop-pre/database/
	# Migration scripts
	cp scripts/migrate.sh $(RELEASE_BUILD)/tuneloop-pre/service/
	# Package
	mkdir -p $(RELEASE_DIR)
	cd $(RELEASE_BUILD) && zip -r $(RELEASE_DIR)/$(PKG_NAME).zip .
	rm -rf $(RELEASE_BUILD)
	@echo "=========================================="
	@echo "Package: $(RELEASE_DIR)/$(PKG_NAME).zip"
	@echo "=========================================="
	@echo "1. Deploy to PRE-PROD (automatic after upload):"
	@echo "   ssh cadenza ~/download.sh $(PKG_NAME).zip   (download.sh deploys via deploy.sh)"
	@echo ""
	@echo "2. Verify on https://preweb.cadenzayueqi.com & https://prewx.cadenzayueqi.com"
	@echo ""
	@echo "3. Promote to PRODUCTION:"
	@echo "   ssh cadenza '/opt/flow/release.sh $(PKG_NAME).zip'"
	@echo "=========================================="
	@echo "=========================================="
	@echo "Uploading $(PKG_NAME).zip via Seafile (work-time safe)..."
	@echo "=========================================="
	# 1. Copy release package as test.zip (fixed name keeps the Seafile
	#    share link stable so cadenza download.sh always fetches the newest)
	cp $(RELEASE_DIR)/$(PKG_NAME).zip ~/test.zip
	@echo "Prepared ~/test.zip (== $(PKG_NAME).zip)"
	# 2. Upload test.zip to Seafile (replaces existing file, share link unchanged)
	source /d/Work/AI/rent/certs/seafile.key && \
	bash ~/scripts/upload_to_seafile.sh ~/test.zip /debug/uem-core/5.2/test
	@echo "Upload complete -> Seafile /debug/uem-core/5.2/test/test.zip"
	# 3. Trigger prerelease deployment on cadenza (downloads + deploy.sh)
	ssh cadenza "~/download.sh $(PKG_NAME).zip"
	@echo "Deploy triggered on cadenza (download.sh $(PKG_NAME).zip)"

# Backward-compatible alias
prerelease: release

# Debug build
.PHONY: debug
DEBUG_DIR := /home/coder/release/tuneloop
debug:
	@echo "Building debug server..."
	@mkdir -p $(DEBUG_DIR)/service $(DEBUG_DIR)/database
	cd backend && go build -gcflags="all=-N -l" -o $(DEBUG_DIR)/service/tuneloop .
	@echo "Copying database migrations..."
	@cp -r backend/database/migrations $(DEBUG_DIR)/database/
	@cp .env.example $(DEBUG_DIR)/.env
	@echo "Debug build complete: $(DEBUG_DIR)/service/tuneloop"

# Version management (SemVer: major.minor.build)
.PHONY: version bump-major bump-minor bump-build

version:
	@echo "Current version: $(shell cat VERSION 2>/dev/null || echo "VERSION file not found")"

bump-build:
	@if [ ! -f VERSION ]; then echo "ERROR: VERSION file not found"; exit 1; fi
	@awk -F. '{printf "%s.%s.%d\n", $$1, $$2, $$3+1}' VERSION > VERSION.tmp && mv VERSION.tmp VERSION
	@echo "Bumped to $(shell cat VERSION)"

bump-minor:
	@if [ ! -f VERSION ]; then echo "ERROR: VERSION file not found"; exit 1; fi
	@awk -F. '{printf "%s.%d.0\n", $$1, $$2+1}' VERSION > VERSION.tmp && mv VERSION.tmp VERSION
	@echo "Bumped to $(shell cat VERSION)"

bump-major:
	@if [ ! -f VERSION ]; then echo "ERROR: VERSION file not found"; exit 1; fi
	@awk -F. '{printf "%d.0.0\n", $$1+1}' VERSION > VERSION.tmp && mv VERSION.tmp VERSION
	@echo "Bumped to $(shell cat VERSION)"
