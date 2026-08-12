.PHONY: web-dev mobile-dev mobile-weapp-dev weapp-upload-prod weapp-upload-pre weapp-build weapp-build-pre weapp-build-prod weapp-cleanup weapp-check web mobile build-frontend build-pc build-mobile kill-port run-backend run run-prod stop install init

NODE_MAJOR := $(shell node -v 2>/dev/null | sed 's/v//' | cut -d. -f1)
NVM22 := . "$$HOME/.nvm/nvm.sh" && nvm use 22 >/dev/null 2>&1 &&

weapp-check:
	@$(NVM22) echo "Node $$(node -v) ready" || (echo "ERROR: Node 22 not available via nvm"; exit 1)

kill-port:
	@fuser -k 5556/tcp 2>/dev/null || true
	@fuser -k 5557/tcp 2>/dev/null || true

build-frontend: build-pc build-mobile

build-pc:
	@echo "Building PC frontend..."
	cd frontend-pc && npm install && npm run build

build-mobile:
	@echo "Building Mobile frontend..."
	cd frontend-mobile && npm install && npm run build

web: build-pc
	@echo "PC frontend build completed."

mobile: build-mobile
	@echo "Mobile frontend build completed."

run-backend: kill-port
	@echo "=========================================="
	@echo "Starting backend services..."
	@echo "Backend API (Mobile): http://localhost:5556"
	@echo "Backend API (PC):     http://localhost:5557"
	@echo "Log file:             backend/backend.log"
	@echo "=========================================="
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

weapp-build: weapp-check
	@rm -rf frontend-mobile/node_modules/.cache
	@rm -rf frontend-mobile/dist-weapp
	@echo "Building WeApp (production apiBaseUrl)..."
	@cd frontend-mobile && TARO_APP_API_BASE_URL=https://wx.cadenzayueqi.com/api npm run build:weapp

weapp-build-pre: weapp-check
	@rm -rf frontend-mobile/node_modules/.cache
	@rm -rf frontend-mobile/dist-weapp
	@echo "Building WeApp (pre-production apiBaseUrl)..."
	@cd frontend-mobile && TARO_APP_API_BASE_URL=https://prewx.cadenzayueqi.com/api npm run build:weapp
	@make weapp-archive-pre VERSION=$(if $(filter command line,$(origin VERSION)),$(VERSION),$(WEAPP_AUTO_VERSION))

weapp-build-prod: weapp-build
	@make weapp-archive-prod VERSION=$(if $(filter command line,$(origin VERSION)),$(VERSION),$(WEAPP_AUTO_VERSION))

# Archive a build (wxss cleanup applied once at archive time — archive is
# the ready-to-upload artifact).
weapp-archive-pre:
	@mkdir -p $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))
	@rm -rf $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cp -r frontend-mobile/dist-weapp $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cd $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp && \
	sed -i 's/\\!//g; s/!important//g; s/\\\//-/g; s/\\//g' app.wxss
	@echo "Archived: $(WEAPP_RELEASE_DIR)/weapp-pre/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp"

weapp-archive-prod:
	@mkdir -p $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))
	@rm -rf $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cp -r frontend-mobile/dist-weapp $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp
	@cd $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp && \
	sed -i 's/\\!//g; s/!important//g; s/\\\//-/g; s/\\//g' app.wxss
	@echo "Archived: $(WEAPP_RELEASE_DIR)/weapp-prod/$(or $(VERSION),$(WEAPP_AUTO_VERSION))/dist-weapp"

# Upload ONLY an archived build — never recompile.
weapp-upload-pre:
	@test -d $(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)/dist-weapp || (echo "ERROR: archive '$(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)' not found — run 'make weapp-build-pre VERSION=$(VERSION)' first"; exit 1)
	@cd frontend-mobile && \
	node_modules/.bin/miniprogram-ci upload \
		--pp ../$(WEAPP_RELEASE_DIR)/weapp-pre/$(VERSION)/dist-weapp \
		--pkp private.wx9f96827856269a6c.key \
		--appid wx9f96827856269a6c \
		--uv $(or $(VERSION),1.0.0-pre) \
		--ud "$(or $(DESC),pre auto deploy)"

weapp-upload-prod:
	@test -d $(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)/dist-weapp || (echo "ERROR: archive '$(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)' not found — run 'make weapp-build-prod VERSION=$(VERSION)' first"; exit 1)
	@cd frontend-mobile && \
	node_modules/.bin/miniprogram-ci upload \
		--pp ../$(WEAPP_RELEASE_DIR)/weapp-prod/$(VERSION)/dist-weapp \
		--pkp private.wxcb44a1be70e356ed.key \
		--appid wxcb44a1be70e356ed \
		--uv $(or $(VERSION),1.0.0) \
		--ud "$(or $(DESC),auto deploy)"

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
	# PC frontend (IAM config from /api/config at runtime)
	$(NVM22) cd frontend-pc && npm run build
	cp -r frontend-pc/dist/* $(RELEASE_BUILD)/tuneloop-pre/www/
	# Mobile frontend (Vite H5, IAM config from /api/config at runtime)
	$(NVM22) cd frontend-mobile && npm run build -- --mode prerelease
	cp -r frontend-mobile/dist/* $(RELEASE_BUILD)/tuneloop-pre/mobile/
	# Backend (version injected via ldflags)
	cd backend && go build -ldflags "-X main.Version=$(VERSION)" -o $(RELEASE_BUILD)/tuneloop-pre/service/tuneloop .
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
