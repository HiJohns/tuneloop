.PHONY: web-dev mobile-dev web mobile build-frontend build-pc build-mobile kill-port run-backend run run-prod stop install init

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
	cd backend && go run main.go 2>&1 | tee backend.log &

web-dev:
	@echo "Starting PC frontend development server..."
	@echo "PC Frontend: http://localhost:5554 (with source map)"
	@cd frontend-pc && npm run dev

mobile-dev:
	@echo "Starting Mobile frontend development server..."
	@echo "Mobile Frontend: http://localhost:5553"
	@cd frontend-mobile && npm run dev

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
RELEASE_DIR := /opt/flow
PKG_NAME := tuneloop_$(TIMESTAMP)_$(GIT_HASH)
RELEASE_BUILD := /tmp/release_build_$(TIMESTAMP)

clean-prerelease:
	@echo "Cleaning build cache..."
	rm -rf $(RELEASE_BUILD)

release: clean-prerelease
	@echo "=========================================="
	@echo "Release: $(PKG_NAME)"
	@echo "=========================================="
	mkdir -p $(RELEASE_BUILD)/tuneloop/www $(RELEASE_BUILD)/tuneloop/mobile \
	         $(RELEASE_BUILD)/tuneloop/service $(RELEASE_BUILD)/tuneloop/database
	# PC frontend
	cd frontend-pc && VITE_API_BASE_URL=/api VITE_BEACONIAM_EXTERNAL_URL=https://iam.cadenzayueqi.com VITE_IAM_PC_CLIENT_ID=tuneloop_web VITE_IAM_PC_REDIRECT_URI=https://web.cadenzayueqi.com/callback npm run build
	cp -r frontend-pc/dist/* $(RELEASE_BUILD)/tuneloop/www/
	# Mobile frontend
	cd frontend-mobile && npm run build -- --mode prerelease
	cp -r frontend-mobile/dist/* $(RELEASE_BUILD)/tuneloop/mobile/
	# Backend
	cd backend && go build -o $(RELEASE_BUILD)/tuneloop/service/tuneloop .
	cp -r backend/database/migrations $(RELEASE_BUILD)/tuneloop/database/
	# Package
	mkdir -p $(RELEASE_DIR)
	cd $(RELEASE_BUILD) && zip -r $(RELEASE_DIR)/$(PKG_NAME).zip .
	rm -rf $(RELEASE_BUILD)
	@echo "=========================================="
	@echo "Package: $(RELEASE_DIR)/$(PKG_NAME).zip"
	@echo "=========================================="

# Backward-compatible alias
prerelease: release

# Debug build
.PHONY: debug
RELEASE_DIR := /home/coder/release/tuneloop
debug:
	@echo "Building debug server..."
	@mkdir -p $(RELEASE_DIR)/service $(RELEASE_DIR)/database
	cd backend && go build -gcflags="all=-N -l" -o $(RELEASE_DIR)/service/tuneloop .
	@echo "Copying database migrations..."
	@cp -r backend/database/migrations $(RELEASE_DIR)/database/
	@cp .env.example $(RELEASE_DIR)/.env
	@echo "Debug build complete: $(RELEASE_DIR)/service/tuneloop"
