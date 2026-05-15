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
	@echo "=========================================="
	cd backend && go run main.go &

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
PRERELEASE_DIR := /home/coder/prerelease/tuneloop
TIMESTAMP := $(shell date +%Y%m%d-%H%M%S)
FLOW_DIR := /opt/flow

clean-prerelease:
	@echo "Cleaning prerelease directories..."
	rm -rf $(PRERELEASE_DIR)/www $(PRERELEASE_DIR)/mobile $(PRERELEASE_DIR)/service

prebuild-pc: clean-prerelease
	@echo "Building PC frontend for prerelease..."
	@mkdir -p $(PRERELEASE_DIR)/www
	cd frontend-pc && VITE_API_BASE_URL=/api VITE_BEACONIAM_EXTERNAL_URL=https://iam.cadenzayueqi.com VITE_IAM_PC_CLIENT_ID=tuneloop_web VITE_IAM_PC_REDIRECT_URI=https://web.cadenzayueqi.com/callback npm run build
	@cp -r frontend-pc/dist/* $(PRERELEASE_DIR)/www/

prebuild-mobile:
	@echo "Building Mobile frontend for prerelease..."
	@mkdir -p $(PRERELEASE_DIR)/mobile
	cd frontend-mobile && npm run build -- --mode prerelease
	@cp -r frontend-mobile/dist/* $(PRERELEASE_DIR)/mobile/

prebuild-backend:
	@echo "Building backend for prerelease..."
	@mkdir -p $(PRERELEASE_DIR)/service $(PRERELEASE_DIR)/database
	cd backend && go build -o $(PRERELEASE_DIR)/service/tuneloop .
	@echo "Copying database migrations..."
	@cp -r backend/database/migrations $(PRERELEASE_DIR)/database/

prebuild-env:
	@echo "Copying environment config..."
	@cp .env.example $(PRERELEASE_DIR)/.env

prerelease: clean-prerelease prebuild-backend prebuild-pc prebuild-mobile prebuild-env
	@echo "=========================================="
	@echo "预发布构建完成($(PRERELEASE_DIR))"
	@echo "PC:   web.cadenzayueqi.com"
	@echo "WX:   wx.cadenzayueqi.com"
	@echo "=========================================="

# Flow release: build both tuneloop + beaconiam into a timestamped zip
FLOW_BUILD := /tmp/flow_build_$(TIMESTAMP)

release: clean-prerelease
	@echo "=========================================="
	@echo "Flow release: $(TIMESTAMP)"
	@echo "=========================================="
	rm -rf $(FLOW_BUILD)
	mkdir -p $(FLOW_BUILD)/tuneloop/www $(FLOW_BUILD)/tuneloop/mobile \
	         $(FLOW_BUILD)/tuneloop/service $(FLOW_BUILD)/tuneloop/database \
	         $(FLOW_BUILD)/beaconiam/www $(FLOW_BUILD)/beaconiam/service
	# tuneloop PC frontend
	cd frontend-pc && VITE_API_BASE_URL=/api VITE_BEACONIAM_EXTERNAL_URL=https://iam.cadenzayueqi.com VITE_IAM_PC_CLIENT_ID=tuneloop_web VITE_IAM_PC_REDIRECT_URI=https://web.cadenzayueqi.com/callback npm run build
	cp -r frontend-pc/dist/* $(FLOW_BUILD)/tuneloop/www/
	# tuneloop Mobile frontend
	cd frontend-mobile && npm run build -- --mode prerelease
	cp -r frontend-mobile/dist/* $(FLOW_BUILD)/tuneloop/mobile/
	# tuneloop backend
	cd backend && go build -o $(FLOW_BUILD)/tuneloop/service/tuneloop .
	cp -r backend/database/migrations $(FLOW_BUILD)/tuneloop/database/
	# BeaconIAM backend
	cd ../beaconiam && go build -o $(FLOW_BUILD)/beaconiam/service/beaconiam ./cmd/api
	# BeaconIAM frontend
	cd ../beaconiam/ui && npm run build
	cp -r ../beaconiam/ui/dist/* $(FLOW_BUILD)/beaconiam/www/
	# Package
	mkdir -p $(FLOW_DIR)
	cd $(FLOW_BUILD) && zip -r $(FLOW_DIR)/$(TIMESTAMP).zip .
	rm -rf $(FLOW_BUILD)
	@echo "=========================================="
	@echo "Flow release package: $(FLOW_DIR)/$(TIMESTAMP).zip"
	@echo "=========================================="

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
